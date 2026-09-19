---
created_on: 2026-09-19 10:22
last_modified: 2026-09-19 10:30
status: current
---

# Implementation Plan & Autonomous Agent Instructions: Agent Status

This document contains step-by-step implementation instructions for an autonomous coding agent to build and verify `agent-status`.

All development must be strictly grounded in:
- `docs/prd.md` (System Architecture & Specification)
- `docs/user-spec.md` (User Wishes & Scenarios)
- `docs/lego-bricks.md` (Verified Go Library & Package Catalog)
- `CONTEXT.md` (Domain Model & Ubiquitous Language)

---

## 1. Operating Rules for the Executing Agent

1. **Subagent Delegation**:
   - You **may and should** spawn focused subagents for independent, parallel work (e.g. generating boilerplate Go packages, building isolated UI components, running verification suites).
   - Coordinate subagents so they work in separate packages/directories without colliding on the Git index.
2. **Red/Green TDD & Verification**:
   - Every feature must have automated tests written first or alongside implementation.
   - For all newly added tests, verify their sensitivity by temporarily disabling the implementation change and confirming the test fails.
3. **Pure-Go / Cgo-Free Invariant**:
   - Use `github.com/ncruces/go-sqlite3` for SQLite (never introduce Cgo requirements).
   - Use `github.com/shirou/gopsutil/v4/process` for process CWD resolution on Darwin (`libproc` purego) and Linux (`/proc`).
4. **Zero-Vite Invariant**:
   - `packages/ui-web` must use Bun native bundling and dev server (`Bun.serve`, `@import "tailwindcss";` in `src/theme.css`). No Vite config or dependencies.
5. **No Blind Stubs or Mock Proxies**:
   - Implement real end-to-end functionality. Stubs mimicking success without writing real state or parsing real files are strictly forbidden.

---

## 2. Phase-by-Phase Execution Steps

### Phase 0: Workspace Bootstrap, Shared Types & Pricing Seed

#### 0.1. Root Workspace & Automation
- [ ] Create root `.gitignore` (verifying ignore rules for `.tmp/`, `.cache/`, `dist/`, `*.sqlite-wal`, `node_modules/`).
- [ ] Create root `justfile` providing recipes:
  - `just build-all`: Builds Go binaries to `packages/*/bin/` and bundles `packages/ui-web`.
  - `just test-all`: Runs Go test suites across all three Go modules and Bun test suite in `packages/ui-web`.
  - `just lint-all`: Runs `golangci-lint` and TypeScript type-checking (`bun x tsc --noEmit`).

#### 0.2. Frontend Type Contract (`packages/ui-web`)
- [ ] Initialize `packages/ui-web/package.json` with React, React DOM, Lucide React, and Tailwind v4.
- [ ] Implement `packages/ui-web/src/types/index.ts` declaring exact TypeScript contracts from `docs/prd.md` §6:
  - `LifecycleState` (`"RUNNING" | "EXITED"`)
  - `ActivityState` (`"BUSY" | "WAITING_QUESTION" | "ERROR" | "IDLE"`)
  - `HarnessType` (`"claude-code" | "pi"`)
  - `AgentPhase`
  - `IEnvironmentInfo`, `IWorkspaceInfo`, `IProjectInfo`, `ITokenUsage`, `ICostInfo`, `IRunInfo`, `IAgentActivity`, `IAgentState`, `ITelemetryFrame`, `IServerEvent`.

#### 0.3. Go Shared Types & Model Pricing Seed
- [ ] Create `packages/agent-ingester/internal/pricing/models.json` containing the pre-filtered LiteLLM model pricing map (covering `claude-opus-5`, `claude-fable-5-1`, `claude-sonnet-5`, `claude-haiku-4-5`, `gemini-3.7-flash`).
- [ ] Implement pricing loader `packages/agent-ingester/internal/pricing/pricing.go` with unit tests verifying calculations for standard turns and subagent roll-ups.

---

### Phase 1: Vertical Slice (Claude Code Telemetry & Ingestion)

#### 1.1. `packages/agent-observer` Core
- [ ] Initialize Go module `packages/agent-observer/go.mod` (Go 1.23+).
- [ ] Implement `internal/config/config.go` resolving environment identity (`IOPlatformUUID` on Darwin, `/etc/machine-id` on Linux, combined with UID to compute `envId = sha256(machine_id + ":" + uid)[:16]`).
- [ ] Implement `internal/scanner/claudecode.go`:
  - Scans `${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/sessions/*.json` (evaluating symlinks).
  - Checks PID liveness and process start time via `gopsutil/v4/process`.
  - Identifies parent transcripts `<projects>/<slug>/<sessionId>.jsonl` and child subagents `<projects>/<slug>/<sessionId>/subagents/*.jsonl`.
- [ ] Implement `internal/tailer/cursor.go` and `internal/tailer/tailer.go`:
  - Implements `fsnotify` file watching with `bufio.Scanner` allocated to 16MB max buffer.
  - Maintains atomic persistent cursors in `$XDG_CACHE_HOME/agent-status/observer-cursors.json` keyed by `(device, inode)`.
  - Implements the 3 reset triggers: inode change, size shrink, prefix SHA-256 mismatch.
- [ ] Implement `internal/parser/claudecode.go`:
  - Parses transcript lines (`user`, `assistant`, `last-prompt`, `ai-title`, `tool_result`).
  - Extracts `is_error`, `AskUserQuestion` tool inputs, and 5-way token usages.
- [ ] Implement `internal/shipper/shipper.go`:
  - WebSocket client using `github.com/coder/websocket` connecting to `:4501/ws/ingest`.
  - Unacknowledged ring buffer using `github.com/gammazero/deque` (1,000 frames cap).
  - Reconnect loop using `github.com/cenkalti/backoff/v4` with seq/ack replay.

#### 1.2. `packages/agent-ingester` Gateway & Store
- [ ] Initialize Go module `packages/agent-ingester/go.mod`.
- [ ] Implement `internal/store/store.go` and `internal/store/migrations.go` using `github.com/ncruces/go-sqlite3`:
  - Runs pragmas: WAL mode, synchronous NORMAL, busy timeout 5000, foreign keys ON, incremental auto vacuum.
  - Implements tables: `environments`, `agents`, `runs`, `telemetry_events`, `observer_cursors`.
- [ ] Implement `internal/classifier/classifier.go`:
  - Deterministic 0-ms state reducer computing `(lifecycle, activity)` pairs.
- [ ] Implement `internal/receiver/receiver.go`:
  - Ingestion WebSocket server using `github.com/coder/websocket` with constant-time bearer token check.
  - Deduplication on `(envId, seq)`.
- [ ] Implement `internal/notify/publisher.go`:
  - Unix domain socket listener at `${XDG_RUNTIME_DIR:-/tmp}/agent-status/notify.sock` (mode 0600) emitting `IServerEvent` lines.

#### 1.3. `packages/agent-server` REST & SSE
- [ ] Initialize Go module `packages/agent-server/go.mod`.
- [ ] Implement `internal/store/queries.go` reading SQLite database read-only.
- [ ] Implement `internal/stream/subscriber.go`:
  - Subscribes to `notify.sock`, resyncing full snapshot from SQLite on initial connect and reconnects.
- [ ] Implement `internal/stream/hub.go` and `internal/api/handlers.go` with `go-chi/chi/v5`:
  - REST endpoints: `GET /api/v1/status`, `GET /api/v1/projects`, `GET /api/v1/environments`, `GET /api/v1/agents`, `GET /api/v1/agents/:id`.
  - Streaming endpoints: `GET /api/v1/events` (SSE), `GET /ws/events` (WebSocket), `GET /api/v1/agents/:id/live-tail`.

#### 1.4. `packages/ui-web` Initial Dashboard
- [ ] Implement `src/hooks/useEventStream.ts` subscribing to `/api/v1/events` with auto-reconnect and snapshot hydration.
- [ ] Implement `src/components/AgentCard.tsx` rendering agent status pills, token breakdown, and live timers.
- [ ] Implement `src/server.ts` with `Bun.serve` hosting the dev server and production assets.

---

### Phase 2: Pi Harness & Multi-Host Aggregation

#### 2.1. Pi Discovery & Process Pairing
- [ ] Implement `internal/scanner/pi.go` in `agent-observer`:
  - Scans `${PI_CODING_AGENT_DIR:-~/.pi/agent}/sessions/*/` and decodes directory names.
  - Uses `gopsutil/v4/process` (`libproc` on Darwin, `/proc` on Linux) to enumerate `pi` processes and extract process CWD in sub-millisecond time.
  - Implements process-to-session pairing by earliest write timestamp after process start time $T$.
- [ ] Implement `internal/parser/pi.go`:
  - Parses Pi tree entries (`session`, `message`, `model_change`, `toolCall`, `toolResult`).
  - Extracts Pi `isError` flags and tool call details.

#### 2.2. Git Workspace & Worktree Resolver
- [ ] Implement `internal/git/resolver.go` in `agent-observer`:
  - Resolves canonical root via `git rev-parse --path-format=absolute --git-common-dir`.
  - Normalizes remote origin URLs (`git remote get-url origin`) to `host/owner/repo`.
  - Detects linked worktrees vs. main repositories.
  - Implements thread-safe in-memory cache with 30-second TTL.

#### 2.3. Multi-Host Heartbeat & Staleness Detector
- [ ] Implement observer 10-second heartbeat loop when idle.
- [ ] Implement ingester background routine flagging environments as `isStale = true` if no heartbeat is received for >30 seconds.

---

### Phase 3: AI Action Summarizer

#### 3.1. Summarizer Pipeline
- [ ] Implement `internal/summarizer/summarizer.go` in `agent-ingester`:
  - Token-bucket rate limiter via `golang.org/x/time/rate` enforcing 12-second debounce per active agent.
  - Unified multi-provider LLM client via `github.com/mozilla-ai/any-llm-go`.
  - Primary provider: Gemini 2.5 Flash via `any-llm-go/providers/gemini`.
  - Fallback provider: Claude Haiku 4.5 via `any-llm-go/providers/anthropic`.
  - Handles fallback on 429, 5xx, or 10-second timeout with jittered backoff.
  - `--no-summarize` CLI flag to bypass external LLM calls completely.

---

### Phase 4: Full Dashboard UI

#### 4.1. Dashboard Views & Interactivity
- [ ] Implement `src/components/Header.tsx`: Fleet overview, active count, attention count, and total fleet cost.
- [ ] Implement `src/components/AttentionBanner.tsx`: Two-tier banner (`ACT NOW` live alerts with jump-to-card, `REVIEW` for recent exits <2h).
- [ ] Implement `src/components/ProjectGroup.tsx` and `src/components/EnvironmentGroup.tsx`: Dual-grouping toggle persisted in `localStorage`.
- [ ] Implement `src/components/LogDrawer.tsx`: Streaming slide-over drawer with thought blocks, tool executions, and collapsible subagent traces.

---

### Phase 5: Hardening & End-to-End Verification

#### 5.1. Database Pruning & Pagination
- [ ] Implement `internal/store/prune.go` in `agent-ingester`:
  - Hourly retention pruner enforcing 2,000 parent + 500 subagent event budget.
  - Purges `EXITED` agents untouched for >30 days.
  - Executes `PRAGMA incremental_vacuum(1000)`.
- [ ] Implement keyset pagination with `before` cursor on `/api/v1/agents/:id/logs`.

#### 5.2. End-to-End Integration Tests
- [ ] Replay test: Terminate `agent-ingester` while observers are actively streaming, restart ingester, verify 100% replay without frame loss.
- [ ] PID reuse test: Simulate recycled PID with distinct start timestamps; assert distinct agent rows are created.
- [ ] Pi republish test: Simulate atomic session file replace; assert cursor detects prefix-hash mismatch and recovers cleanly.

---

## 3. Comprehensive Acceptance Criteria

### Package Build & Quality Gates
- [ ] `just build-all` successfully compiles `air-observer`, `air-ingester`, `air-server` binaries into `packages/*/bin/` and bundles `ui-web`.
- [ ] `just test-all` executes all unit and integration test suites with zero failures.
- [ ] Unit test coverage is $\ge 90\%$ across all Go packages.
- [ ] TypeScript strict mode type-check (`bun x tsc --noEmit`) passes with zero errors.

### Observer Functionality
- [ ] Accurately discovers Claude Code descriptors under symlinked `~/.claude/sessions` and `~/.local/share/ai-registry/claude-code/sessions`.
- [ ] Resolves Darwin process CWD via native `libproc` in $<0.1\text{ ms}$ without spawning subprocesses.
- [ ] Correctly tails nested Claude Code subagent transcripts (`<sessionId>/subagents/*.jsonl`).
- [ ] Persists byte-offset cursors atomically in `$XDG_CACHE_HOME/agent-status/observer-cursors.json`.
- [ ] Replays dropped/unacknowledged frames upon WebSocket reconnection.

### Ingester & Server Functionality
- [ ] Ingests frames over WebSocket with constant-time Bearer token validation.
- [ ] Applies 0-ms deterministic state machine accurately classifying `(lifecycle, activity)`.
- [ ] Computes exact 5-way token usages and dollar costs using the vendored LiteLLM pricing map.
- [ ] Broadcasts real-time events over `${XDG_RUNTIME_DIR:-/tmp}/agent-status/notify.sock` to `agent-server`.
- [ ] `agent-server` cold-resyncs snapshot from SQLite upon connection and broadcasts over SSE / WebSockets.

### Web Dashboard
- [ ] Renders live updating agent cards without full page refreshes.
- [ ] Sticky `ACT NOW` alert banner triggers immediately when an agent calls `AskUserQuestion` or hits an unhandled error.
- [ ] Project grouping view accurately clusters agents working in `.workspaces/...` worktrees under the parent repository origin name.
- [ ] Displays accurate 5-way token breakdown tooltips and live cost totals.
- [ ] Live execution drawer displays real-time streaming model thoughts, tool calls, and collapsible subagent child traces.
