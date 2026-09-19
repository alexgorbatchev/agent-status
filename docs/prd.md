---
created_on: 2026-09-18 20:07
last_modified: 2026-09-18 23:05
status: current
---

# Product Requirements Document & Architecture Specification: Agent Status

## 1. Objective, Scope, and Non-Goals

### 1.1. Objective

`agent-status` is a multi-host observability system that tracks the live execution state of AI coding agents across development machines, repositories, and git worktrees, and presents them in a single web dashboard.

It answers five questions without the engineer cycling through terminals:

1. Which agents are running, and on which host/environment.
2. Which project, branch, and worktree each agent is operating on.
3. What each agent is currently doing (title and live action summary) and how much it has cost.
4. Which agents require immediate attention (waiting on questions, or hit unhandled errors).
5. Which agents have completed their current turn or exited.

### 1.2. In Scope

- Harnesses: **Claude Code** and **Pi**.
- Host platforms: **darwin** and **linux**.
- Four packages: `agent-observer`, `agent-ingester`, `agent-server`, `ui-web`.
- Live token usage (5-way breakdown: input, output, cache read, cache write, reasoning) and cost computation using a centrally synced model pricing map.
- Live cost roll-ups across Agent cards, Project groups, Environment groups, and the Fleet header.
- Subagent observation for delegated runs (nested execution traces in the drawer, usage/cost roll-up to the parent Agent).

### 1.3. Non-Goals

These are rejected during implementation review. Each has a stated reason and, where applicable, the condition under which it would be reconsidered.

| Non-goal | Reason |
|---|---|
| **Codex support** | Deferred by product decision. `HarnessType` does not include it. Revisit after Phase 5. |
| **OpenCode support** | Deferred by product decision. Its store (`~/.local/share/opencode/opencode.db`) is SQLite, not JSONL, and requires a separate tailer design. Revisit after Phase 5. |
| **Windows hosts** | Agent liveness uses POSIX process semantics (`signal 0`, `/proc`, `lsof`/`libproc`). `platform` is `"darwin" \| "linux"` only. |
| **Permission-prompt detection** | No reliable on-disk or transcript signal for a *pending* permission prompt exists without intercepting IPC or mutating user settings. Stalled permission checks currently report as `BUSY`. See §11. |
| **Answering a question from the dashboard** | The UI surfaces blocked agents; the engineer answers in the agent's own terminal. |
| **Historical BI & multi-month time-series cost reporting** | Real-time usage and live cost roll-ups for active/recent agents are in scope; historical time-series analytics, budget alerting, and financial reporting across months are out of scope. |
| **Agent control plane** | Out of scope entirely. This is an observability pane, not a remote execution or process management system. |
| **UI bundle embedding in any Go binary** | Explicit product constraint. `ui-web` is served independently by Bun. |

---

## 2. Verified Baseline

Everything in this section was verified against a live machine on 2026-09-18 and describes the **current** state of the world, not the target design. §4 onward describes the target design.

### 2.1. Claude Code (version 2.1.277)

**Session descriptors.** Claude Code writes one JSON descriptor per live session to its `sessions` store directory, named `<pid>.json`:

```json
{"pid":10554,"sessionId":"12b36299-805c-4fe0-b1cc-74a588fbf520","cwd":"/Users/agorbatchev/development/dotfiles-installer","startedAt":1789780883485,"procStart":"Sat Sep 19 01:21:23 2026","version":"2.1.277","peerProtocol":1,"peerFeatures":["notify_idle","reply_across_default_dirs","artifact_yield"],"kind":"interactive","entrypoint":"cli","pidDomain":"darwin","messagingSocketPath":"/tmp/cc-socks/10554.sock","name":"dotfiles-installer-e6","nameSource":"derived","nameSince":1789780883485,"status":"busy","updatedAt":1789784534686,"statusUpdatedAt":1789784534686}
```

The descriptor carries a coarse `status` (`busy` and `idle` observed), `statusUpdatedAt`, `cwd`, and a stable `sessionId`.

Sibling `<pid>.<64-hex>.key` files (mode `0600`, JSON) sit in the same directory. These are credential tokens for the peer socket. The observer never opens, parses, logs, or ships them.

**Store location and symlinks.** On a machine managed by `ai-registry`, `~/.claude` is a symlink chain:

```
~/.claude                  -> <ai-registry>/.output/claude-code/default
~/.claude/sessions         -> ${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/sessions
~/.claude/projects         -> ${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/projects
```

The observer evaluates symlinks (`filepath.EvalSymlinks`) before recording any path and before comparing paths for identity.

**Transcripts.** Parent transcripts reside at `<projects-store>/<slug(cwd)>/<sessionId>.jsonl`, where `slug` replaces every `/` and every `.` with `-`. Example: `/Users/agorbatchev/development/envoy.com/.workspace` maps to `-Users-agorbatchev-development-envoy-com--workspace`.

**Subagent transcripts.** Subagent session transcripts reside in a nested subfolder alongside the parent transcript:
`<projects-store>/<slug(cwd)>/<sessionId>/subagents/agent-<agentId>.jsonl`.
Every record in a subagent transcript carries `isSidechain: true` and an `agentId`. On live machines, subagent runs can account for over 85% of total token spend for complex workflows.

**Transcript record types observed.** `mode`, `permission-mode`, `atis-latch`, `file-history-snapshot`, `user`, `attachment`, `last-prompt`, `ai-title`, `assistant`, `system`.

Every message record carries `sessionId`, `cwd`, `gitBranch`, `version`, `uuid`, `parentUuid`, `isSidechain`, `timestamp`, `userType`, `entrypoint`. `gitBranch` is present on every record, eliminating the need for `git` subprocess execution during transcript parsing.

`ai-title` records carry `aiTitle` (a high-level rolling title).
`last-prompt` records carry `lastPrompt` (the current user task string).

`assistant` records carry `message.stop_reason` (`tool_use`, `end_turn` observed), `message.model`, `message.usage` (input, output, cache creation, cache read tokens), and `message.content` blocks of type `thinking`, `tool_use`, `text`.

`user` records carry `toolUseResult` and `tool_result` content blocks with shape `{type, tool_use_id, content, is_error}`. Correlation between tool invocation and result is strictly by `tool_use_id`.

`AskUserQuestion` appears as a normal `tool_use` block, with question text in `input.questions[0].question`.

### 2.2. Pi

**Session files.** `${PI_CODING_AGENT_DIR:-~/.pi/agent}/sessions/--<encoded-cwd>--/<timestamp>_<session-id>.jsonl`.

The encoding rule from Pi's `session-manager.ts` strips the leading path separator, replaces `/`, `\`, and `:` with `-`, and wraps with `--`:
`--${cwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--`

**Record types observed.** `session` (header), `message`, `model_change`, `thinking_level_change`, `custom_message`, `custom`, `compaction`, `branch_summary`, `label`, `session_info`.

Entries form a tree via `{id, parentId}`. The first record is the header:
```json
{"type":"session","version":3,"id":"01a0b64b-...","timestamp":"2026-09-18T20:53:30.007Z","cwd":"/Users/agorbatchev/development/ai-registry"}
```

**Message shapes.** `message.role` is one of `user`, `assistant`, `toolResult`. Assistant messages carry `model`, `provider`, `usage` (input, output, cacheRead, cacheWrite, reasoning, cost), and content blocks of type `text`, `thinking`, `toolCall`. Tool results carry `{role:"toolResult", toolCallId, toolName, content, isError, timestamp}`. Pi tool results contain `toolName` directly.

**Process mapping.** Pi session files contain no PID. Liveness requires correlating running `pi` processes with session directories by cwd and start timestamps (§4.1.3).

**Two storage generations.** Pi has a legacy v3 layout and a format-4 layout that writes by append-then-atomic-republish. An atomic republish replaces the file inode, invalidating byte offsets unless detected by prefix hashing.

---

## 3. System Architecture & Topology

```
┌──────────────────────────────────────────┐   ┌──────────────────────────────────────────┐
│         ENVIRONMENT A (work-macbook)     │   │        ENVIRONMENT B (dev-box-linux)     │
│  ┌────────────────────────────────────┐  │   │  ┌────────────────────────────────────┐  │
│  │      packages/agent-observer       │  │   │  │      packages/agent-observer       │  │
│  │ • Claude Code descriptor scanner   │  │   │  │ • Claude Code descriptor scanner   │  │
│  │ • Pi session + process scanner     │  │   │  │ • Pi session + process scanner     │  │
│  │ • Subagent directory tailers       │  │   │  │ • Subagent directory tailers       │  │
│  │ • Byte-offset JSONL tailers        │  │   │  │ • Byte-offset JSONL tailers        │  │
│  │ • Git root / worktree resolver     │  │   │  │ • Git root / worktree resolver     │  │
│  └──────────────────┬─────────────────┘  │   │  └──────────────────┬─────────────────┘  │
└─────────────────────┼────────────────────┘   └─────────────────────┼────────────────────┘
                      │                                              │
                      │   ws:// over private overlay network         │
                      │   Bearer token, seq/ack framing              │
                      ▼                                              ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                packages/agent-ingester                                  │
│                                                                                         │
│  1. Ingestion gateway    :4501/ws/ingest  (bound to the overlay interface)              │
│  2. Deterministic classifier — no network calls on the hot path                         │
│  3. Model pricing calculator — vendored LiteLLM pricing + daily refresh                 │
│  4. Debounced LLM action summarizer — Gemini 2.5 Flash, fallback claude-haiku-4-5       │
│  5. SQLite writer (WAL, sole writer) — agents, runs, environments, telemetry_events     │
│  6. Notify publisher — Unix domain socket (0600)                                        │
└──────────────┬──────────────────────────────────────────────┬───────────────────────────┘
               │                                              │
               │ writes                          notify.sock  │ newline-delimited JSON
               ▼                                              ▼
      ┌────────────────────┐                  ┌─────────────────────────────────────────┐
      │   SQLite (WAL)     │ ◀── read-only ── │       packages/agent-server             │
      │ agent-status.db    │                  │                                         │
      └────────────────────┘                  │  1. REST API      127.0.0.1:4502 + ovl  │
                                              │  2. SSE / WS broadcaster                │
                                              │  3. Resync from SQLite on subscribe     │
                                              └──────────────────┬──────────────────────┘
                                                                 │
                                                                 │ REST + SSE, Bearer token
                                                                 ▼
                                              ┌─────────────────────────────────────────┐
                                              │           packages/ui-web               │
                                              │      Bun + React + TS + Tailwind v4     │
                                              └─────────────────────────────────────────┘
```

### 3.1. Inter-Process Communication (IPC)

The ingester and server run as separate processes on the same central host.

- The ingester creates `${XDG_RUNTIME_DIR:-/tmp}/agent-status/notify.sock` (mode `0600`) and accepts subscriber connections.
- It writes newline-delimited JSON. Each line is exactly one `IServerEvent` (§6.3) — the same envelope the server broadcasts over SSE and WebSockets.
- The socket is non-blocking with an internal subscriber buffer. If a subscriber is blocked for more than 2 seconds, the ingester closes that connection.
- On connect and on every reconnect, `agent-server` queries SQLite to obtain a full snapshot of all active environments and agents, emits a `snapshot` event to its connected clients, and then resumes applying live socket events.
- The server automatically redials the notify socket with a 1-second interval on disconnect.

---

## 4. Component Specifications

### 4.1. `packages/agent-observer`

Runs as a lightweight background daemon on every developer machine. Discovers running agents and subagents, tails transcripts, tracks cursors, and ships structured telemetry.

#### 4.1.1. Environment Identity

An Environment is identified by **(machine_id + uid)**:
- Darwin `machine_id`: Hardware platform UUID from `IOPlatformExpertDevice` (`IOPlatformUUID`).
- Linux `machine_id`: `/etc/machine-id` (falling back to `/var/lib/dbus/machine-id`).
- Unique ID: `env_id = sha256(machine_id + ":" + uid)[:16]`.

Display label resolution (first match wins):
1. `--env <name>` CLI flag
2. `AI_ENV` environment variable
3. `environment` in `~/.config/ai-registry/telemetry.json`
4. OS hostname fallback (`os.Hostname()`)

Changing the label updates metadata on the central server without modifying the `env_id` or breaking agent relationships.

#### 4.1.2. Claude Code Discovery

1. Resolve the sessions store: `${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/sessions`, evaluating symlinks via `filepath.EvalSymlinks`. Fallback to `~/.claude/sessions`.
2. Read `<pid>.json` descriptors. Sibling `.key` files are ignored.
3. Check PID liveness with `syscall.Kill(pid, 0)`. Verify process start time against `procStart` to eliminate stale PID reuse.
4. On first sight of an active descriptor, emit `run_started` with `sessionId`, `pid`, `cwd`, `startedAt`, `version`.
5. Map descriptor `status`: `busy` to `BUSY`, `idle` to `IDLE`.
6. Resolve the parent transcript path: `<projects-store>/<slug(cwd)>/<sessionId>.jsonl`.
7. Watch for subagent transcripts: `<projects-store>/<slug(cwd)>/<sessionId>/subagents/*.jsonl`.
8. When a descriptor disappears or its PID is terminated, emit `run_exited`.

Scan interval: 2 seconds, supplemented by `fsnotify` file-watch triggers on the session directory.

#### 4.1.3. Pi Discovery

1. **Session directory scanner**: Scan `${PI_CODING_AGENT_DIR:-~/.pi/agent}/sessions/*/`. Read the JSONL header of each session file to determine `sessionId`, `cwd`, and `timestamp`.
2. **Process scanner**: Enumerate processes where binary name is `pi`. Resolve process cwd (`proc_pidinfo` / `PROC_PIDVNODEPATHINFO` on Darwin via `gopsutil`, `readlink /proc/<pid>/cwd` on Linux) and process start time `T`.
3. **Process-to-session pairing**: For a `pi` process with cwd `C` and start time `T`, pair it with the session in `C`'s encoded directory whose earliest record at or after `T` was written first. Sessions without an active process are marked `EXITED`.
4. When a paired `pi` process terminates, emit `run_exited`.

Scan interval: 2 seconds.

#### 4.1.4. Git Workspace & Worktree Resolver

For each agent's initial working directory:
1. `git rev-parse --path-format=absolute --git-common-dir` determines the shared repository `.git` directory.
2. Canonical root is the directory containing the shared `.git` folder (or parent if common dir is `.git`).
3. Project identity (`projectId`):
   - Query origin URL: `git remote get-url origin`.
   - Normalize to `host/owner/repo` (e.g. `github.com/alexgorbatchev/ai-registry`).
   - If no origin exists, fallback to `local/<canonical-root-basename>`.
   - Display name is the repository slug (or remote name if disambiguation is required).
4. Linked worktree detection: Compare `git rev-parse --show-toplevel` against canonical root. If different, `isWorktree = true` and `relPath` is relative to canonical root.
5. Branch extraction: `gitBranch` field in transcript records (Claude Code) or `git symbolic-ref --short HEAD` (Pi).

Project attribution is **sticky to the first real git repository** encountered by the agent. Subsequent `cd` operations into nested folders or temporary directories do not re-attribute the agent's project group.

#### 4.1.5. Byte-Offset Log Tailer & Subagents

- Watches registered `.jsonl` files (parent and subagents) using `fsnotify` with a 500ms polling fallback.
- Persists cursors to `${XDG_CACHE_HOME:-~/.cache}/agent-status/observer-cursors.json`, keyed by `(device, inode)`.
- Cursor fields: `offset`, `size`, `inode`, `prefixHash` (SHA-256 of first 512 bytes).
- Reset rules: Reset offset to 0 if inode changes, if size < recorded offset, or if `prefixHash` changes (detects Pi's atomic-republish).
- Subagent records (`isSidechain: true`) carry parent `sessionId` and child `agentId`. Telemetry frames preserve this linkage.

#### 4.1.6. Shipper & Replay Protocol

- Connects to `ws://<ingester-host>:4501/ws/ingest` over private overlay network with `Authorization: Bearer <token>`.
- Frame sequencing: Monotonically increasing `seq` per observer connection starting at 1.
- In-memory ring buffer holds up to 1,000 unacknowledged frames.
- Ingester acknowledges commits via `{"type":"ack","upTo":<seq>}`.
- Reconnect sends `{"type":"hello","envId":...,"since":<lastAckedSeq>,"dropped":<droppedCount>}` and replays unacknowledged frames.
- Heartbeat: Observer emits a heartbeat frame every 10 seconds if no telemetry frames were sent.

---

### 4.2. `packages/agent-ingester`

Central stream ingestion, deterministic classification, model pricing computation, debounced AI summarization, and WAL SQLite persistence.

#### 4.2.1. Ingestion Gateway

- Listens on `:4501/ws/ingest`, bound exclusively to the overlay interface.
- Validates bearer token in constant time.
- Validates frame version `"1.0"`.
- Deduplicates incoming frames on `(envId, seq)` against persisted `observer_cursors.last_seq`.

#### 4.2.2. Deterministic Classifier (0-ms Heuristics)

Evaluated on the hot path without external network calls:

1. On `run_exited`: Set agent `lifecycle = EXITED`, update `runs.ended_at`, clear `agents.current_run_id`. `activity` remains untouched.
2. On `run_started`: Set agent `lifecycle = RUNNING`, insert/update `runs`, set `agents.current_run_id`.
3. On `question_prompt` (`AskUserQuestion`): Set `activity = WAITING_QUESTION`, store question text.
4. On `tool_result` where `isError == true`: Set `activity = ERROR`, store truncated error message in `lastError`.
5. On `tool_call`: Set `activity = BUSY`, record `activeTool` and `activeTarget`.
6. On `turn_end`: Set `activity = IDLE`, clear `activeTool`, `activeTarget`, and `question`.
7. On `thinking` or `turn_start`: Set `activity = BUSY`.

Subagent events update parent token usage and cost roll-ups, but do not override the parent's top-level `activity` unless the parent is explicitly waiting on subagent completion (`BUSY` with `activeTool = "Agent"`).

#### 4.2.3. Model Pricing Engine

- Maintains an in-memory price map loaded at startup from a vendored copy of LiteLLM's `model_prices_and_context_window.json` (stripped to model pricing tokens).
- A background scheduler refreshes the price map daily from upstream. On network failure, the current in-memory map is preserved.
- Cost computation per assistant turn:
  $$\text{Cost} = (N_{\text{input}} \times P_{\text{input}}) + (N_{\text{output}} \times P_{\text{output}}) + (N_{\text{cache\_read}} \times P_{\text{cache\_read}}) + (N_{\text{cache\_write}} \times P_{\text{cache\_write}})$$
- Model matching handles exact IDs and date-suffixed variants (e.g. `claude-haiku-4-5-20251001`). Unrecognized models or `<synthetic>` records yield `null` cost without aborting usage accumulation.
- Parent agent cost is the sum of its direct turns plus all its subagent turns.

#### 4.2.4. Title & Action Summarizer

- **Title**: Extracted from harness metadata (Claude Code `ai-title` record or Pi session info). Stored in `agents.title`. Free, no LLM call.
- **Action Summarizer**:
  - Model: **Gemini 2.5 Flash** (fallback: **`claude-haiku-4-5`**).
  - Triggers: On `turn_end`, on transition to `WAITING_QUESTION` or `ERROR`, or after 12 seconds of continuous tool activity.
  - Rate limit / Debounce: At most one call per active agent per 12 seconds.
  - Prompt input: `lastPrompt` string, latest 3 tool calls and targets, latest thinking excerpt (max 300 tokens).
  - Structured output schema:
    ```json
    {
      "action": "Fixing TypeScript compilation errors in build.ts",
      "phase": "debugging",
      "activeTarget": "packages/registry-cli/src/commands/buildCommand.ts"
    }
    ```
  - Flag `--no-summarize` disables external LLM calls, falling back to deterministic tool descriptions (`"Editing build.ts"`).

#### 4.2.5. SQLite Persistence & Retention

- Path: `${XDG_DATA_HOME:-~/.local/share}/agent-status/agent-status.db`.
- Sole writer process with WAL mode:
  ```sql
  PRAGMA journal_mode=WAL;
  PRAGMA synchronous=NORMAL;
  PRAGMA busy_timeout=5000;
  PRAGMA foreign_keys=ON;
  PRAGMA auto_vacuum=INCREMENTAL;
  ```
- Startup runs schema migrations tracked by `PRAGMA user_version`.
- **Retention Pruner** (runs hourly):
  1. For each active or recent Agent: retain the most recent 2,000 parent events plus the most recent 500 events per subagent (up to 20 subagents), or all events from the last 48 hours (whichever retains more). Delete older events.
  2. For `EXITED` agents untouched for >30 days: delete agent row, cascading delete of all associated runs and events.
  3. Execute `PRAGMA incremental_vacuum(1000)`.

---

### 4.3. `packages/agent-server`

Dedicated read-only API and real-time streaming broadcaster.

#### 4.3.1. Binding & Auth

- Port: **4502**. Binds `127.0.0.1` and overlay network interfaces.
- Requires `Authorization: Bearer <token>` on all REST and streaming endpoints.
- CORS allowlist defaults to `http://localhost:3000` (Bun dev server).

#### 4.3.2. REST Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/status` | Health, environment statuses, active agent count, total fleet spend |
| GET | `/api/v1/projects` | Grouped projects with active agents and recent agents (<2h) |
| GET | `/api/v1/environments` | Environments with online/stale status, agent counts, and heartbeats |
| GET | `/api/v1/agents` | Filterable agent list (`?project=...&environment=...&lifecycle=...&activity=...`) |
| GET | `/api/v1/agents/:id` | Detailed agent record with full usage, cost, runs, and subagent summary |
| GET | `/api/v1/agents/:id/logs` | Keyset-paginated events (`?limit=100&before=<event_id>&subagent=<agent_id>`) |

#### 4.3.3. Streaming Endpoints

- `GET /api/v1/events` (SSE) and `GET /ws/events` (WebSocket):
  - Initial frame on connection: `snapshot` containing all active environments and agents.
  - Real-time broadcasts: `agent_upsert`, `agent_removed`, `environment_upsert`, `log_append`.
  - Heartbeat: Keep-alive comment every 15 seconds.
- `GET /api/v1/agents/:id/live-tail` (SSE): Dedicated log stream for an individual agent including live subagent traces.

---

### 4.4. `packages/ui-web`

Single-page web application built with **Bun + React + TypeScript + Tailwind v4** (CSS-first `@import "tailwindcss";`).

#### 4.4.1. Core Views & Components

1. **Fleet Header**:
   - Total active agents, online vs. stale environments, count of agents requiring attention, and total live fleet cost.
2. **Two-Tier Attention Banner**:
   - **ACT NOW**: Sticky alert banner for live agents with `lifecycle = RUNNING` and `activity IN ('WAITING_QUESTION', 'ERROR')`. Displays exact question prompt or error snippet. Clicking jumps to the card and opens drawer.
   - **REVIEW (Last 2 Hours)**: Collapsible section for `lifecycle = EXITED` agents that stopped on error or unanswered prompt.
   - Stale environments are excluded from both tiers.
3. **Dual Grouping**:
   - **Group by Project** (Default): Aggregates agents across machines working on the same normalized project, displaying project-level total cost.
   - **Group by Environment**: Shows machines/users with machine status pills (Live / Stale) and environment-level cost.
4. **Agent Card**:
   - **Status Badge**: Computed from `(lifecycle, activity)`:
     - `RUNNING / BUSY`: Blue pulsing indicator
     - `RUNNING / WAITING_QUESTION`: Amber alert pill
     - `RUNNING / ERROR`: Red alert pill
     - `RUNNING / IDLE`: Slate pill
     - `EXITED / IDLE`: Emerald "Finished" pill
     - `EXITED / ERROR`: Red outline "Died on error" pill
     - `EXITED / WAITING_QUESTION`: Amber outline "Abandoned prompt" pill
   - **Title & Action Summary**: Stable title on top, dynamic action line below.
   - **Metadata Row**: Environment pill (`🏷️ work-macbook`), Git Branch (`🌿 feat-x`), Worktree tag (`📁 .workspaces/feat-x`), Harness (`Claude Code` / `Pi`).
   - **Spend & Usage**: Formatted cost (`$12.45`), token breakdown tooltip (Input, Output, Cache Read, Cache Write, Reasoning), and subagent indicator (`3 subagents`).
   - **Timers**: Current run elapsed time and overall session age.
5. **Live Execution Drawer**:
   - Slide-over drawer with streaming thinking excerpts, tool calls, formatted inputs/outputs, and collapsible subagent child runs.

---

## 5. Package Layout

```
agent-status/
├── docs/
│   ├── prd.md
│   ├── user-spec.md
│   ├── refs.md
│   ├── lego-bricks.md
│   └── internal/
│       └── tickets/
│           └── 2026-09-18-wave-1-investigate-prior-art-agent-liveness-and-session-pairing.md
├── CONTEXT.md
├── packages/
│   ├── agent-observer/
│   │   ├── bin/
│   │   ├── cmd/air-observer/main.go
│   │   ├── internal/
│   │   │   ├── config/config.go
│   │   │   ├── git/resolver.go
│   │   │   ├── scanner/claudecode.go
│   │   │   ├── scanner/pi.go
│   │   │   ├── scanner/proc.go
│   │   │   ├── tailer/tailer.go
│   │   │   ├── tailer/cursor.go
│   │   │   ├── parser/claudecode.go
│   │   │   ├── parser/pi.go
│   │   │   └── shipper/shipper.go
│   │   ├── go.mod
│   │   ├── go.sum
│   │   └── justfile
│   ├── agent-ingester/
│   │   ├── bin/
│   │   ├── cmd/air-ingester/main.go
│   │   ├── internal/
│   │   │   ├── config/config.go
│   │   │   ├── receiver/receiver.go
│   │   │   ├── classifier/classifier.go
│   │   │   ├── pricing/pricing.go
│   │   │   ├── pricing/models.json
│   │   │   ├── summarizer/summarizer.go
│   │   │   ├── summarizer/gemini.go
│   │   │   ├── summarizer/anthropic.go
│   │   │   ├── notify/publisher.go
│   │   │   ├── store/store.go
│   │   │   ├── store/migrations.go
│   │   │   └── store/prune.go
│   │   ├── go.mod
│   │   ├── go.sum
│   │   └── justfile
│   ├── agent-server/
│   │   ├── bin/
│   │   ├── cmd/air-server/main.go
│   │   ├── internal/
│   │   │   ├── config/config.go
│   │   │   ├── api/handlers.go
│   │   │   ├── api/middleware.go
│   │   │   ├── stream/hub.go
│   │   │   ├── stream/subscriber.go
│   │   │   └── store/queries.go
│   │   ├── go.mod
│   │   ├── go.sum
│   │   └── justfile
│   └── ui-web/
│       ├── src/
│       │   ├── index.html
│       │   ├── main.tsx
│       │   ├── App.tsx
│       │   ├── theme.css
│       │   ├── components/
│       │   │   ├── Header.tsx
│       │   │   ├── AttentionBanner.tsx
│       │   │   ├── ProjectGroup.tsx
│       │   │   ├── EnvironmentGroup.tsx
│       │   │   ├── AgentCard.tsx
│       │   │   ├── StatusBadge.tsx
│       │   │   └── LogDrawer.tsx
│       │   ├── hooks/useEventStream.ts
│       │   └── types/index.ts
│       ├── server.ts
│       ├── package.json
│       ├── tsconfig.json
│       └── justfile
├── .gitignore
└── justfile
```

---

## 6. Data Contracts

Source of truth: `packages/ui-web/src/types/index.ts`. Go structures match field-for-field with corresponding `json` struct tags.

### 6.1. Core Types

```typescript
export type LifecycleState = "RUNNING" | "EXITED";

export type ActivityState = "BUSY" | "WAITING_QUESTION" | "ERROR" | "IDLE";

export type HarnessType = "claude-code" | "pi";

export type AgentPhase = "planning" | "editing" | "testing" | "debugging" | "investigating";

export interface IEnvironmentInfo {
  id: string;          // sha256(machine_id + ":" + uid)[:16]
  label: string;       // Mutable display name: e.g. "work-macbook"
  hostname: string;
  platform: "darwin" | "linux";
  user: string;
  isStale: boolean;    // true if heartbeat > 30s
  lastHeartbeatAt: number;
}

export interface IWorkspaceInfo {
  isWorktree: boolean;
  branch: string;
  isDetached: boolean;
  relPath: string | null;
  cwd: string;
}

export interface IProjectInfo {
  id: string;          // Normalized origin e.g. "github.com/alexgorbatchev/ai-registry"
  name: string;        // Display name e.g. "ai-registry"
  canonicalRoot: string;
}

export interface ITokenUsage {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  reasoning: number;
  total: number;       // Sum of all 5 components
}

export interface ICostInfo {
  currency: "USD";
  total: number;       // Direct + subagent cost in USD
  isEstimated: boolean;
}

export interface IRunInfo {
  id: string;
  pid: number;
  harnessVersion: string;
  startedAt: number;
  endedAt: number | null;
}

export interface IAgentActivity {
  lifecycle: LifecycleState;
  activity: ActivityState;
  title: string;             // Harness rolling title or prompt summary
  action: string;            // LLM generated 1-sentence current action
  actionStale: boolean;
  phase: AgentPhase | null;
  activeTool: string | null;
  activeTarget: string | null;
  question: string | null;   // Active prompt when WAITING_QUESTION
  lastThinkingExcerpt: string | null;
  lastError: string | null;
  usage: ITokenUsage;
  cost: ICostInfo | null;
  subagentCount: number;
  currentRun: IRunInfo | null;
  firstStartedAt: number;
  updatedAt: number;
}

export interface IAgentState {
  id: string;                // `${envId}:${harness}:${sessionId}`
  sessionId: string;
  harness: HarnessType;
  environment: IEnvironmentInfo;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  activity: IAgentActivity;
}
```

### 6.2. Telemetry Frame (Observer $\to$ Ingester)

```typescript
export interface ITelemetryFrame {
  version: "1.0";
  seq: number;
  envId: string;
  environment: IEnvironmentInfo;
  agentId: string;
  sessionId: string;
  subagentId: string | null;  // Set if event belongs to a subagent
  pid: number;
  harness: HarnessType;
  harnessVersion: string;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  timestamp: number;
  eventType:
    | "run_started"
    | "run_exited"
    | "turn_start"
    | "thinking"
    | "tool_call"
    | "tool_result"
    | "question_prompt"
    | "error"
    | "turn_end"
    | "title_change";
  data: {
    role: "user" | "assistant" | "system" | null;
    content: string | null;
    title: string | null;
    lastPrompt: string | null;
    model: string | null;
    toolName: string | null;
    toolCallId: string | null;
    toolInput: Record<string, unknown> | null;
    toolOutput: string | null;
    isError: boolean | null;
    exitCode: number | null;
    question: string | null;
    stopReason: string | null;
    usage: ITokenUsage | null;
    rawHarnessCost: number | null;
  };
}

export type IObserverMessage =
  | { type: "hello"; envId: string; environment: IEnvironmentInfo; since: number; dropped: number }
  | { type: "frames"; frames: ITelemetryFrame[] };

export type IIngesterMessage =
  | { type: "ack"; upTo: number }
  | { type: "error"; code: "unsupported_version" | "unauthorized" | "malformed"; message: string };
```

### 6.3. Server Events (Ingester $\to$ Server $\to$ UI)

```typescript
export type IServerEvent =
  | { type: "snapshot"; at: number; agents: IAgentState[]; environments: IEnvironmentInfo[] }
  | { type: "agent_upsert"; at: number; agent: IAgentState }
  | { type: "agent_removed"; at: number; agentId: string }
  | { type: "environment_upsert"; at: number; environment: IEnvironmentInfo }
  | { type: "log_append"; at: number; agentId: string; event: ITelemetryFrame };
```

---

## 7. Database Schema (SQLite in WAL Mode)

```sql
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;
PRAGMA auto_vacuum=INCREMENTAL;

CREATE TABLE IF NOT EXISTS environments (
    id                TEXT PRIMARY KEY, -- sha256(machine_id + ":" + uid)[:16]
    label             TEXT NOT NULL,
    hostname          TEXT NOT NULL,
    platform          TEXT NOT NULL CHECK (platform IN ('darwin','linux')),
    user              TEXT NOT NULL,
    dropped_frames    INTEGER NOT NULL DEFAULT 0,
    last_heartbeat_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
    id                    TEXT PRIMARY KEY, -- `${envId}:${harness}:${sessionId}`
    session_id            TEXT NOT NULL,
    environment_id        TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    harness               TEXT NOT NULL CHECK (harness IN ('claude-code','pi')),
    project_id            TEXT NOT NULL,
    project_name          TEXT NOT NULL,
    canonical_root        TEXT NOT NULL,
    branch                TEXT NOT NULL,
    is_detached           INTEGER NOT NULL DEFAULT 0,
    is_worktree           INTEGER NOT NULL DEFAULT 0,
    worktree_rel_path     TEXT,
    cwd                   TEXT NOT NULL,
    current_run_id        TEXT REFERENCES runs(id) ON DELETE SET NULL,
    activity              TEXT NOT NULL CHECK (activity IN ('BUSY','WAITING_QUESTION','ERROR','IDLE')),
    title                 TEXT NOT NULL DEFAULT '',
    action                TEXT NOT NULL DEFAULT '',
    action_stale          INTEGER NOT NULL DEFAULT 0,
    phase                 TEXT CHECK (phase IS NULL OR phase IN ('planning','editing','testing','debugging','investigating')),
    active_tool           TEXT,
    active_target         TEXT,
    question              TEXT,
    last_thinking_excerpt TEXT,
    last_error            TEXT,
    tokens_input          INTEGER NOT NULL DEFAULT 0,
    tokens_output         INTEGER NOT NULL DEFAULT 0,
    tokens_cache_read     INTEGER NOT NULL DEFAULT 0,
    tokens_cache_write    INTEGER NOT NULL DEFAULT 0,
    tokens_reasoning      INTEGER NOT NULL DEFAULT 0,
    tokens_total          INTEGER NOT NULL DEFAULT 0,
    cost_usd              REAL,
    subagent_count        INTEGER NOT NULL DEFAULT 0,
    first_started_at      INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL,
    UNIQUE (environment_id, harness, session_id)
);

CREATE INDEX IF NOT EXISTS idx_agents_project     ON agents(project_id);
CREATE INDEX IF NOT EXISTS idx_agents_environment ON agents(environment_id);
CREATE INDEX IF NOT EXISTS idx_agents_activity    ON agents(activity);
CREATE INDEX IF NOT EXISTS idx_agents_updated     ON agents(updated_at);

CREATE TABLE IF NOT EXISTS runs (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    pid             INTEGER NOT NULL,
    harness_version TEXT NOT NULL,
    started_at      INTEGER NOT NULL,
    ended_at        INTEGER
);

CREATE INDEX IF NOT EXISTS idx_runs_agent ON runs(agent_id);

CREATE TABLE IF NOT EXISTS telemetry_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    subagent_id   TEXT,
    event_type    TEXT NOT NULL,
    timestamp     INTEGER NOT NULL,
    data_json     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_agent_ts ON telemetry_events(agent_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_subagent ON telemetry_events(agent_id, subagent_id);
CREATE INDEX IF NOT EXISTS idx_events_ts       ON telemetry_events(timestamp);

CREATE TABLE IF NOT EXISTS observer_cursors (
    environment_id   TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    last_seq         INTEGER NOT NULL,
    PRIMARY KEY (environment_id)
);
```

---

## 8. Security Model

| Surface | Control |
|---|---|
| Observer to Ingester | `ws://` over private overlay network (Tailscale / WireGuard). Ingester binds overlay interface only. Bearer token on upgrade compared with `crypto/subtle.ConstantTimeCompare`. |
| Ingester Notify Socket | Unix domain socket, mode `0600`, under `${XDG_RUNTIME_DIR:-/tmp}/agent-status/`. |
| Server API & Streams | Binds `127.0.0.1:4502` and overlay interface. Bearer token required on all REST, SSE, and WS routes. CORS restricted to explicit allowlist. |
| Database | Mode `0600`. Ingester is sole read-write connection; Server is strictly read-only. |
| AI Summarizer Egress | Only `lastPrompt`, active tool names/targets, and truncated thought excerpts leave the network to Gemini / Anthropic. File contents, tool outputs, and command results are never sent. `--no-summarize` disables all egress. |
| Credential Protection | Claude Code `.key` files are never parsed, read, or shipped. `AGENT_STATUS_TOKEN` is never persisted in cursor files or database logs. |

---

## 9. Implementation Order

### Phase 0: Repository Foundation & Shared Schemas
- [ ] Root `justfile` with `build-all`, `test-all`, `lint-all`.
- [ ] TypeScript schemas in `packages/ui-web/src/types/index.ts`.
- [ ] Go struct models in shared internal packages.
- [ ] Vendored LiteLLM pricing seed file in `packages/agent-ingester/internal/pricing/models.json`.

### Phase 1: Vertical Slice (Claude Code + Host Telemetry)
- [ ] Observer: Environment ID resolver (`IOPlatformUUID` / `/etc/machine-id` + `uid`).
- [ ] Observer: Claude Code descriptor scanner and process liveness check.
- [ ] Observer: Transcript resolver with symlink evaluation and slug encoding.
- [ ] Observer: Subagent directory tailer (`<sessionId>/subagents/*.jsonl`).
- [ ] Observer: Byte-offset tailer with cursor persistence and reset heuristics.
- [ ] Observer: Resilient WebSocket shipper with `seq/ack` and buffer replay.
- [ ] Ingester: Gateway receiver, token validator, deduplicator on `(envId, seq)`.
- [ ] Ingester: Deterministic classifier (`(lifecycle, activity)` state machine).
- [ ] Ingester: Pricing calculator computing cost across main and subagent usage.
- [ ] Ingester: SQLite store with WAL mode, migrations, and foreign keys.
- [ ] Ingester: Unix socket notify publisher.
- [ ] Server: REST `/api/v1/agents`, SSE `/api/v1/events`, notify subscriber with initial SQLite snapshot resync.
- [ ] UI: Bun + React dashboard rendering live agent cards from SSE stream.
- [ ] Verification: Live Claude Code run with subagents transitions `RUNNING/BUSY` to `RUNNING/IDLE` to `EXITED/IDLE` with accurate usage and cost.

### Phase 2: Pi Harness & Multi-Host Aggregation
- [ ] Wave 1 Investigation: Execute `docs/internal/tickets/2026-09-18-wave-1-investigate-prior-art-agent-liveness-and-session-pairing.md`.
- [ ] Observer: Pi session and process scanner, implementing verified process-to-session pairing.
- [ ] Observer: Git worktree resolver normalizing project origin URLs.
- [ ] Ingester: Question detection for both harnesses.
- [ ] Ingester: Stale environment detector (>30s without heartbeat).
- [ ] Verification: Multi-host aggregation with agents running on Darwin and Linux over overlay network.

### Phase 3: AI Action Summarizer
- [ ] Ingester: Gemini 2.5 Flash client with structured JSON output and phase classification.
- [ ] Ingester: `claude-haiku-4-5` fallback with retry backoff.
- [ ] Ingester: 12-second debouncer and event trigger scheduler.
- [ ] Ingester: `--no-summarize` toggle verification.

### Phase 4: Full Dashboard UI
- [ ] UI: Two-tier Attention Banner (`ACT NOW` live alerts vs `REVIEW` recent exits).
- [ ] UI: Dual grouping views (By Project with project cost totals, By Environment with machine pills).
- [ ] UI: Live Execution Drawer with streaming timeline and collapsible subagent traces.
- [ ] UI: Fleet header with live active counts, attention counters, and fleet spend total.
- [ ] UI: Stale environment and disconnected status indicators.

### Phase 5: Hardening & Retention
- [ ] Ingester: Retention pruner enforcing per-agent event budgets (2,000 parent + 500/subagent) and 30-day agent cleanup.
- [ ] Server: Keyset pagination on `/api/v1/agents/:id/logs`.
- [ ] Reconnection resilience test: Ingester restart mid-run with zero frame loss.
- [ ] PID reuse test: Validating that reused PIDs do not collide sessions.
- [ ] Pi republish test: Validating that atomic rewrite triggers prefix-hash reset cleanly.

---

## 10. Testing & Definition of Done

- Unit test coverage $\ge 90\%$ on all Go packages (`agent-observer`, `agent-ingester`, `agent-server`).
- UI component and hook tests for reconnection and state transitions.
- Integration tests for observer replay, notify socket broadcasting, SQLite snapshot recovery, and subagent token roll-up.
- Mandatory independent review pass for every phase and ticket before closing.

---

## 11. Known Gaps

1. **Permission-prompt detection**: No signal for a *pending* permission prompt exists in transcript logs. Investigated under Wave 1 (`docs/internal/tickets/2026-09-18-wave-1-investigate-prior-art-agent-liveness-and-session-pairing.md`). Until resolved, agents awaiting permission confirmation report as `BUSY`.
2. **Pi question tool name**: To be verified against live Pi sessions during Wave 1 investigation.
3. **Claude Code IPC socket unused**: Socket `/tmp/cc-socks/<pid>.sock` and `.key` credentials remain unparsed in favor of file-based transcript observation.
4. **Claude Code hooks not configured**: Global hooks (`settings.json`) avoided to prevent modifying user configuration and to maintain parity with non-hook harnesses.
