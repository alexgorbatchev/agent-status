---
created_on: 2026-09-18 20:07
last_modified: 2026-09-18 20:07
status: current
---

# Product Requirements Document & Architecture Specification: Agent Status

## 1. Objective, Scope, and Non-Goals

### 1.1. Objective

`agent-status` is a multi-host observability system that tracks the live execution state of AI coding agents across development machines, repositories, and git worktrees, and presents them in a single web dashboard.

It answers five questions without the engineer cycling through terminals:

1. Which agents are running, and on which host.
2. Which project, branch, and worktree each agent is operating on.
3. What each agent is currently doing, as a one-sentence summary.
4. Which agents are blocked waiting for user input.
5. Which agents have errored or finished.

### 1.2. In Scope

- Harnesses: **Claude Code** and **Pi**.
- Host platforms: **darwin** and **linux**.
- Four packages: `agent-observer`, `agent-ingester`, `agent-server`, `ui-web`.

### 1.3. Non-Goals

These are rejected during implementation review. Each has a stated reason and, where applicable, the condition under which it would be reconsidered.

| Non-goal | Reason |
|---|---|
| **Codex support** | Deferred by product decision. `HarnessType` does not include it. Revisit after Phase 5. |
| **OpenCode support** | Deferred by product decision. Its store (`~/.local/share/opencode/opencode.db`) is SQLite, not JSONL, and requires a separate tailer design. Revisit after Phase 5. |
| **Windows hosts** | Agent liveness uses POSIX process semantics (`signal 0`, `/proc`, `lsof`). `platform` is `"darwin" \| "linux"` only. |
| **Permission-prompt detection** | No observable signal has been verified. See §5.4.3. |
| **Answering a question from the dashboard** | The UI surfaces blocked agents; the engineer answers in the agent's own terminal. |
| **Historical analytics, cost reporting, agent control** | Out of scope entirely. This is a status pane, not a control plane. |
| **UI bundle embedding in any Go binary** | Explicit product constraint. `ui-web` is served independently. |

---

## 2. Verified Baseline

Everything in this section was verified against a live machine on 2026-09-18 and describes the **current** state of the world, not the target design. §4 onward describes the target design.

### 2.1. Claude Code (version 2.1.277)

**Session descriptors.** Claude Code writes one JSON descriptor per live session to its `sessions` store directory, named `<pid>.json`:

```json
{"pid":10554,"sessionId":"12b36299-805c-4fe0-b1cc-74a588fbf520","cwd":"/Users/agorbatchev/development/dotfiles-installer","startedAt":1789780883485,"procStart":"Sat Sep 19 01:21:23 2026","version":"2.1.277","peerProtocol":1,"peerFeatures":["notify_idle","reply_across_default_dirs","artifact_yield"],"kind":"interactive","entrypoint":"cli","pidDomain":"darwin","messagingSocketPath":"/tmp/cc-socks/10554.sock","name":"dotfiles-installer-e6","nameSource":"derived","nameSince":1789780883485,"status":"busy","updatedAt":1789784534686,"statusUpdatedAt":1789784534686}
```

The descriptor already carries a coarse `status` (`busy` and `idle` observed), `statusUpdatedAt`, the `cwd`, and a stable `sessionId`. The design in §4.1 consumes these directly rather than re-deriving them.

Sibling `<pid>.<64-hex>.key` files (mode `0600`, JSON) sit in the same directory. These are credential material for the peer socket. The observer never opens, parses, logs, or ships them.

**Store location and symlinks.** On a machine managed by `ai-registry`, `~/.claude` is a symlink and so are the stores beneath it:

```
~/.claude                  -> <ai-registry>/.output/claude-code/default
~/.claude/sessions         -> ${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/sessions
~/.claude/projects         -> ${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/projects
```

The observer resolves symlinks before recording any path and before comparing paths for identity. Two different literal paths routinely denote the same file.

**Transcripts.** `<projects-store>/<slug(cwd)>/<sessionId>.jsonl`, where `slug` replaces every `/` and every `.` with `-`. Verified: `/Users/agorbatchev/development/envoy.com/.workspace` maps to `-Users-agorbatchev-development-envoy-com--workspace`. All live descriptors resolve to an existing transcript by this rule.

**Transcript record types observed.** `mode`, `permission-mode`, `atis-latch`, `file-history-snapshot`, `user`, `attachment`, `last-prompt`, `ai-title`, `assistant`, `system`.

Every message record carries `sessionId`, `cwd`, `gitBranch`, `version`, `uuid`, `parentUuid`, `isSidechain`, `timestamp`, `userType`, `entrypoint`. **`gitBranch` is present on every record**, so branch resolution for Claude Code needs no `git` subprocess.

`assistant` records carry `message.stop_reason` (`tool_use`, `end_turn` observed) and `message.content` blocks of type `thinking`, `tool_use`, `text`. `user` records carry `toolUseResult` and `tool_result` content blocks with the shape `{type, tool_use_id, content, is_error}` — `is_error` is present on every one of the 1,959 observed blocks. `tool_result` does **not** carry the tool name; correlation is by `tool_use_id`.

`AskUserQuestion` appears as a normal `tool_use` block, `input.questions[].question`. Verified across 26 occurrences.

`isSidechain: true` marks subagent turns, which share the parent's transcript file.

### 2.2. Pi

**Session files.** `${PI_CODING_AGENT_DIR:-~/.pi/agent}/sessions/--<encoded-cwd>--/<timestamp>_<session-id>.jsonl`.

The encoding is authoritative from Pi's own `session-manager.ts`:

```
--${cwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--
```

Note this differs from Claude Code's slug: Pi strips the leading separator, wraps in `--`, and does **not** replace `.`.

**Record types observed.** `session`, `message`, `model_change`, `thinking_level_change`, `custom_message`, `custom`. Pi's own `session-format.md` documents `compaction`, `branch_summary`, `label`, and `session_info` as well.

Entries form a tree via `{id, parentId}`. The first record is the header:

```json
{"type":"session","version":3,"id":"01a0b64b-...","timestamp":"2026-09-18T20:53:30.007Z","cwd":"/Users/agorbatchev/development/ai-registry"}
```

**Message shapes.** `message.role` is one of `user`, `assistant`, `toolResult`. Assistant messages carry `stopReason`, `rawStopReason`, `model`, `provider`, `usage`, and content blocks of type `text`, `thinking`, `toolCall`. Tool results carry `{role:"toolResult", toolCallId, toolName, content, isError, timestamp}` plus an optional `details`. Unlike Claude Code, **Pi's tool result carries the tool name directly**.

**No PID.** Pi session files contain no process ID. Liveness requires the process table (§4.1.3).

**Two storage generations.** Pi has a legacy v3 layout and a newer "format-4" layout that writes by append-then-atomic-republish. A republish replaces the file, which invalidates a naive byte offset. The cursor design in §4.1.5 handles this.

### 2.3. Repository

At the time of writing the repository contains only `docs/`. No Go module, no Bun app, no `justfile`. Everything in §3 is target state.

---

## 3. System Architecture & Topology

```
┌──────────────────────────────────────────┐   ┌──────────────────────────────────────────┐
│          HOST A (work-macbook)           │   │          HOST B (dev-box-linux)          │
│  ┌────────────────────────────────────┐  │   │  ┌────────────────────────────────────┐  │
│  │      packages/agent-observer       │  │   │  │      packages/agent-observer       │  │
│  │ • Claude Code descriptor scanner   │  │   │  │ • Claude Code descriptor scanner   │  │
│  │ • Pi session + process scanner     │  │   │  │ • Pi session + process scanner     │  │
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
│  3. Debounced LLM summarizer — Gemini 2.5 Flash, fallback claude-haiku-4-5              │
│  4. SQLite writer (WAL, sole writer)                                                    │
│  5. Notify publisher — Unix domain socket                                               │
└──────────────┬──────────────────────────────────────────────┬───────────────────────────┘
               │                                              │
               │ writes                          notify.sock  │ newline-delimited JSON
               ▼                                              ▼
      ┌────────────────────┐                  ┌─────────────────────────────────────────┐
      │   SQLite (WAL)     │ ◀── read-only ── │       packages/agent-server             │
      │ agent-status.db    │                  │                                         │
      └────────────────────┘                  │  1. REST API      127.0.0.1 + overlay   │
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

### 3.1. Why the ingester and server are separate, and how they talk

The two are separate processes by product constraint. SQLite provides no cross-process change notification — `sqlite3_update_hook` is in-process only and a WAL commit does not signal other connections — so a shared database file alone cannot drive real-time updates.

**Decision: the ingester publishes to a Unix domain socket; the server subscribes.**

- The ingester creates `${XDG_RUNTIME_DIR:-/tmp}/agent-status/notify.sock` (mode `0600`) and accepts subscriber connections.
- It writes newline-delimited JSON. Each line is exactly one `IServerEvent` (§6.3) — the same envelope the server re-emits over SSE.
- The socket is fire-and-forget. If a subscriber's buffer is full for more than 2 seconds, the ingester closes that connection rather than blocking the ingest path. SQLite remains the durable record.
- On connect and on every reconnect, the server first reads current state from SQLite, emits a `snapshot` event to its own clients, and only then applies socket events. This makes a dropped subscriber self-healing.
- The server redials every 1 second while disconnected.

Both processes run on the same host. Cross-host separation is a non-goal.

---

## 4. Component Specifications

### 4.1. `packages/agent-observer`

Runs as a background daemon on every host. Discovers agents, resolves git context, tails session logs, and ships frames.

#### 4.1.1. Environment identity

Resolved in this order, first match wins:

1. `--env <name>`
2. `AI_ENV`
3. `environment` in `~/.config/ai-registry/telemetry.json`
4. `os.Hostname()`

#### 4.1.2. Claude Code discovery

1. Resolve the sessions store: `${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/sessions`, then `filepath.EvalSymlinks`. If that path does not exist, fall back to `~/.claude/sessions`, also symlink-resolved.
2. Read every `<pid>.json`. Ignore `*.key` files entirely.
3. Confirm liveness with `syscall.Kill(pid, 0)`. Guard against PID reuse by requiring that the process start time match the descriptor's `procStart`; a mismatch means the descriptor is stale and is skipped.
4. Emit `agent_started` on first sight, carrying `sessionId`, `cwd`, `pid`, `startedAt`, and `version`.
5. Map `status` to the reported state: `busy` to `BUSY`, `idle` to `IDLE`. The transcript classifier (§5.4) refines this; the descriptor is the floor, not the ceiling.
6. Resolve the transcript to `<projects-store>/<slug(cwd)>/<sessionId>.jsonl` with `slug` replacing `/` and `.` with `-`, and register it with the tailer.
7. A descriptor that disappears, or whose PID is dead, produces `agent_exited`.

Scan interval: 2 seconds. Descriptor reads are also triggered by `fsnotify` on the sessions directory; the interval is the fallback.

#### 4.1.3. Pi discovery

Pi session files contain no PID, so discovery is two-sided and joined on `cwd`.

1. **Sessions.** Walk `${PI_CODING_AGENT_DIR:-~/.pi/agent}/sessions/*/`. For each directory, decode the `cwd` by reversing the encoder, and read the header record of each `*.jsonl` to obtain the authoritative `cwd`, `id`, and `version`. The header is the first line; reading stops there.
2. **Processes.** Enumerate processes whose executable base name is `pi`. Resolve each PID's working directory: `readlink /proc/<pid>/cwd` on Linux, `lsof -a -p <pid> -d cwd -Fn` on darwin.
3. **Join.** A session is live when a `pi` process exists whose resolved cwd equals the session header's `cwd` and whose start time precedes the session file's creation timestamp. When one cwd has several sessions and one process, the most recently modified session file wins.
4. A session whose process is gone produces `agent_exited`.

Scan interval: 2 seconds.

Phase 1 must verify the darwin `lsof` path on a machine with Pi running and record the measured cost of one scan in the package README. If per-scan cost exceeds 50 ms, the scan interval moves to 5 seconds; nothing else changes.

#### 4.1.4. Git resolver

For each agent `cwd`:

- Claude Code: take `gitBranch` straight from the transcript record. No subprocess.
- Pi: `git symbolic-ref --short HEAD`, and on detached HEAD `git rev-parse --short HEAD` with `isDetached: true`.
- Both: `git rev-parse --show-toplevel` gives the worktree root, `git rev-parse --git-common-dir` gives the shared git directory. When the common dir is not `<toplevel>/.git`, the checkout is a linked worktree: `isWorktree` is true and `relPath` is the worktree root relative to the canonical root.
- The canonical project root is the parent of the common dir with a trailing `/.git` stripped. The project name is that directory's base name.
- Results are cached per `cwd` for 30 seconds. A branch change in a transcript record invalidates the entry immediately.

#### 4.1.5. Byte-offset tailer

- Watches registered `.jsonl` files with `fsnotify`, with a 500 ms poll fallback.
- Cursors persist to `${XDG_CACHE_HOME:-~/.cache}/agent-status/observer-cursors.json`, keyed by `(device, inode)` rather than path, written atomically via temp file plus rename, and flushed at most once per second.
- Each cursor stores `offset`, `size`, `inode`, and a SHA-256 of the first 512 bytes.
- **Reset rules.** Reset the offset to 0 and re-read when the inode changes, when size is less than the recorded offset, or when the 512-byte prefix hash changes. The last rule is what catches Pi's append-then-atomic-republish, where a new inode carries the same logical session.
- Partial trailing lines are buffered and never parsed. A line that fails to parse is counted and skipped; it does not advance past the line and does not stall the tailer.
- Records with `isSidechain: true` are tagged `isSidechain` on the frame and do not change the parent agent's status. They still stream to the live drawer.

#### 4.1.6. Shipper

- Connects to `ws://<ingester-host>:4501/ws/ingest` over the private overlay network, presenting `Authorization: Bearer <token>` on the upgrade request.
- The token is read from `AGENT_STATUS_TOKEN` or `--token-file <path>`. It is never logged and never written to the cursor file.
- Each frame carries a monotonically increasing `seq`, per observer process, starting at 1.
- Frames are held in an in-memory ring buffer of 1,000 entries until acknowledged. The ingester acknowledges with `{"type":"ack","upTo":<seq>}`; everything at or below `upTo` is released.
- On reconnect the observer sends `{"type":"hello","environment":...,"since":<lastAckedSeq>}` and replays every unacknowledged frame in order. The ingester deduplicates on `(environment, seq)`.
- Buffer overflow drops the oldest frames and increments a counter reported on the next `hello`. The dashboard surfaces a non-zero drop count on the environment badge.
- Reconnect backoff: 500 ms doubling to a 30 second ceiling, with full jitter.
- Frames flush every 200 ms or at 50 frames, whichever comes first.

### 4.2. `packages/agent-ingester`

#### 4.2.1. Ingestion gateway

- Listens on `:4501/ws/ingest`, bound to the overlay interface address given by `--bind`. It does not bind `0.0.0.0`.
- Rejects any upgrade without a valid bearer token, comparing with `crypto/subtle.ConstantTimeCompare`.
- Rejects any frame whose `version` is not `"1.0"` with `{"type":"error","code":"unsupported_version"}` and closes the connection. There is no negotiation.
- Deduplicates on `(environment, seq)` against the highest `seq` committed for that environment.
- Acknowledges only after the frame is committed to SQLite.

#### 4.2.2. Deterministic classifier

Evaluated on the ingest path with no network calls. Rules are ordered; the first match wins.

1. An `agent_exited` event sets `DONE`.
2. A `question_prompt` event sets `WAITING_QUESTION` and stores the question text.
3. A `tool_result` whose `isError` is true sets `ERROR` and stores the result content, truncated to 2,000 characters, as `lastError`.
4. A `tool_call` event sets `BUSY` and records `activeTool` and `activeTarget`.
5. A `turn_end` event sets `IDLE` and clears `activeTool`, `activeTarget`, and `question`.
6. A `thinking` or `turn_start` event sets `BUSY`.

`IDLE` and `DONE` are distinct and never ambiguous: **`IDLE` means the turn ended and the process is alive; `DONE` means the process is gone.** Only rule 1 produces `DONE`.

`ERROR` is set only by an explicit harness error flag — Claude Code's `tool_result.is_error` or Pi's `toolResult.isError`. A non-zero process exit code is *not* an error signal on its own: `grep` with no match exits 1 and a deliberately failing test suite exits non-zero, and treating those as errors would make the alert banner useless. `exitCode` is still recorded on the event for the drawer.

`ERROR` is sticky until the next `tool_call`, `turn_start`, or `turn_end` for that agent.

#### 4.2.3. Question detection

`WAITING_QUESTION` is raised when the observer sees a tool call the harness uses to ask the user a question, and no `tool_result` with a matching id has arrived:

- Claude Code: a `tool_use` block named `AskUserQuestion`. The question text is `input.questions[0].question`.
- Pi: a `toolCall` whose name is the Pi equivalent. Phase 2 must confirm that name against a live Pi session before the rule ships; until confirmed, Pi raises `WAITING_QUESTION` only on an explicit harness question event and the gap is recorded in the package README.

The pending tool-call id is held per agent. The arrival of the matching `tool_result` clears `WAITING_QUESTION`.

#### 4.2.4. Summarizer

- Primary model: **Gemini 2.5 Flash**. Fallback: **`claude-haiku-4-5`**.
- The fallback fires on HTTP 429, any 5xx, or a request exceeding a 10 second timeout, after two retries with jitter. If the fallback also fails, the agent keeps its previous summary, `summaryStale` is set true, and the failure is counted. Summarization never blocks classification or persistence.
- Triggers: a `turn_end` event, a transition into `WAITING_QUESTION` or `ERROR`, or the debounce timer expiring during a long tool sequence.
- **Debounce: at most one call per active agent per 12 seconds.** This is the single value; `user-spec.md` agrees.
- Prompt input: the agent's initial task prompt, the three most recent tool invocations with their targets, and the most recent thinking excerpt truncated to 300 tokens.
- Structured output:
  ```json
  {
    "summary": "Fixing TypeScript types in build command",
    "phase": "debugging",
    "activeTarget": "packages/registry-cli/src/commands/buildCommand.ts"
  }
  ```
- `phase` is one of `planning`, `editing`, `testing`, `debugging`, `investigating`. Any other value is discarded and `phase` is left unchanged.

**Data egress.** This feature sends agent thinking excerpts, tool names, and file paths to a third-party model provider. It is enabled by default and disabled by `--no-summarize`, which leaves the dashboard fully functional with deterministic status only. Command output and tool results are never included in the summarizer prompt.

#### 4.2.5. SQLite persistence

- Path: `${XDG_DATA_HOME:-~/.local/share}/agent-status/agent-status.db`.
- The ingester is the **sole writer**. This is an invariant, not a convention: WAL permits exactly one writer, and the server opens the database read-only.
- Connection setup, in order, on every connection:
  ```sql
  PRAGMA journal_mode=WAL;
  PRAGMA synchronous=NORMAL;
  PRAGMA busy_timeout=5000;
  PRAGMA foreign_keys=ON;
  ```
  `foreign_keys=ON` is required. SQLite defaults it off per connection, and without it the `ON DELETE CASCADE` in §7 never fires.
- Migrations run at startup against `PRAGMA user_version`, applying each numbered step in order inside a transaction. `CREATE TABLE IF NOT EXISTS` alone cannot add or alter a column and is not the migration mechanism.

#### 4.2.6. Retention

A pruner runs hourly:

- Delete `telemetry_events` rows older than **14 days**.
- Delete `agents` rows in state `DONE` whose `updated_at` is older than **30 days**, cascading their events.
- Run `PRAGMA incremental_vacuum` after any prune that removed rows. `PRAGMA auto_vacuum=INCREMENTAL` is set at database creation.

Both windows are configurable via `--events-retention-days` and `--agents-retention-days`.

### 4.3. `packages/agent-server`

Read-only consumer of the database and the notify socket.

#### 4.3.1. Binding and access control

- Listens on port **4502**. Binds `127.0.0.1` plus the overlay interface address given by `--bind`. It does not bind `0.0.0.0`.
- Every endpoint, including the event streams, requires `Authorization: Bearer <token>`. The browser obtains the token from a same-origin config endpoint served by `ui-web`'s own dev server, or from a token entered once and held in `sessionStorage`.
- CORS allows exactly the origins listed in `--cors-origin`, defaulting to `http://localhost:3000` — Bun's dev server port. There is no wildcard origin.

#### 4.3.2. REST endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/status` | Health, total agents, connected environments, ingest lag |
| GET | `/api/v1/projects` | Canonical projects with their active and recent agents |
| GET | `/api/v1/environments` | Environments with active agent counts and last heartbeat |
| GET | `/api/v1/agents` | Agent list; filters `environment`, `project`, `status` |
| GET | `/api/v1/agents/:id` | One agent with its recent timeline |
| GET | `/api/v1/agents/:id/logs` | Paginated events; `limit` (max 500, default 100), `before` cursor |

**"Recent" is defined**: an agent is recent when `status = 'DONE'` and `updated_at` is within the last 2 hours. Anything older is excluded from `/api/v1/projects` and returned only by an explicit `/api/v1/agents` query.

Pagination on `/logs` is keyset, not offset — `before` is the `id` of the oldest row already seen. Offset pagination over a table under active insert skips and repeats rows.

#### 4.3.3. Streaming

- `GET /api/v1/events` — SSE. `GET /ws/events` — WebSocket. Both carry `IServerEvent`.
- The first message on any new subscription is a `snapshot` event with the full current agent list, so a client never renders from a partial state.
- `GET /api/v1/agents/:id/live-tail` — SSE of that agent's raw events, including sidechain records.
- SSE heartbeat comment every 15 seconds to defeat idle proxies.

### 4.4. `packages/ui-web`

Bun + React + TypeScript + Tailwind v4. **No Vite.** Bun's own bundler and dev server (`Bun.serve` with an HTML entrypoint, `bun --hot`) provide the dev loop and the production build; adding Vite would mean running a second toolchain for something Bun does natively.

Tailwind v4 is configured CSS-first through `@import "tailwindcss";` in `src/theme.css` with tokens declared in an `@theme` block. There is no `tailwind.config.js`.

1. **Fleet header** — total active agents, environments online, count requiring attention, and a warning when any environment reports dropped frames.
2. **Action-required banner** — sticky, lists every agent in `WAITING_QUESTION` or `ERROR` with its question or error text. Clicking scrolls to the card and opens its drawer.
3. **Dual grouping** — by Project (default) or by Environment. The choice persists in `localStorage`.
4. **Agent card** — status badge (`BUSY` blue pulse, `WAITING_QUESTION` amber, `ERROR` red, `IDLE` slate, `DONE` emerald), environment badge, branch, worktree indicator, harness tag, AI summary with a staleness marker when `summaryStale`, active tool and target, elapsed timer.
5. **Live execution drawer** — slide-over with the streaming timeline of thinking excerpts, tool calls, and truncated outputs, with sidechain records visually nested.

Reconnection: the SSE hook retries with the same backoff curve as the observer and shows a disconnected state rather than stale data presented as live.

---

## 5. Package Layout

```
agent-status/
├── docs/
│   ├── prd.md
│   └── user-spec.md
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
│       │   │   ├── ProjectGroup.tsx
│       │   │   ├── EnvironmentGroup.tsx
│       │   │   ├── AgentCard.tsx
│       │   │   ├── QuestionBanner.tsx
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

All three Go CLIs use Cobra with `cobrahelptree` for help-tree generation, so `just help` renders the full command surface of each binary identically.

---

## 6. Data Contracts

`packages/ui-web/src/types/index.ts` is the source of truth. The Go structs mirror it field for field with matching JSON tags.

### 6.1. Core types

```typescript
export type AgentStatus = "BUSY" | "WAITING_QUESTION" | "ERROR" | "IDLE" | "DONE";

export type HarnessType = "claude-code" | "pi";

export type AgentPhase = "planning" | "editing" | "testing" | "debugging" | "investigating";

export interface IEnvironmentInfo {
  name: string;
  hostname: string;
  platform: "darwin" | "linux";
  user: string;
}

export interface IWorkspaceInfo {
  isWorktree: boolean;
  branch: string;
  isDetached: boolean;
  relPath: string | null;
  cwd: string;
}

export interface IProjectInfo {
  name: string;
  canonicalRoot: string;
}

export interface ITokenUsage {
  input: number;
  output: number;
  total: number;
}

export interface IAgentActivity {
  status: AgentStatus;
  summary: string;
  summaryStale: boolean;
  phase: AgentPhase | null;
  activeTool: string | null;
  activeTarget: string | null;
  question: string | null;
  lastThinkingExcerpt: string | null;
  lastError: string | null;
  tokens: ITokenUsage | null;
  startedAt: number;
  updatedAt: number;
}

export interface IAgentState {
  id: string;
  sessionId: string;
  pid: number;
  harness: HarnessType;
  harnessVersion: string;
  environment: IEnvironmentInfo;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  activity: IAgentActivity;
}
```

**Nullability is uniform.** Every optional field is `T | null`, never `T | undefined` and never an empty string standing in for absence. The Go structs use pointer types for exactly these fields, the SQLite columns are nullable for exactly these fields, and the API emits JSON `null`. The one exception is `summary`, which is `""` until the first summarization completes, because the UI always renders that slot.

**Agent identity.** `IAgentState.id` is `` `${environment}:${harness}:${sessionId}` `` — for example `work-macbook:claude-code:12b36299-805c-4fe0-b1cc-74a588fbf520`. The PID is an attribute, not part of the key. Operating systems recycle PIDs, so a PID-keyed identity would eventually merge two unrelated sessions into one row and one event history.

### 6.2. Telemetry frame, observer to ingester

```typescript
export interface ITelemetryFrame {
  version: "1.0";
  seq: number;
  environment: IEnvironmentInfo;
  agentId: string;
  sessionId: string;
  pid: number;
  harness: HarnessType;
  harnessVersion: string;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  timestamp: number;
  isSidechain: boolean;
  eventType:
    | "agent_started"
    | "agent_exited"
    | "turn_start"
    | "thinking"
    | "tool_call"
    | "tool_result"
    | "question_prompt"
    | "error"
    | "turn_end";
  data: {
    role: "user" | "assistant" | "system" | null;
    content: string | null;
    toolName: string | null;
    toolCallId: string | null;
    toolInput: Record<string, unknown> | null;
    toolOutput: string | null;
    isError: boolean | null;
    exitCode: number | null;
    question: string | null;
    stopReason: string | null;
    tokens: ITokenUsage | null;
  };
}

export type IObserverMessage =
  | { type: "hello"; environment: IEnvironmentInfo; since: number; dropped: number }
  | { type: "frames"; frames: ITelemetryFrame[] };

export type IIngesterMessage =
  | { type: "ack"; upTo: number }
  | { type: "error"; code: "unsupported_version" | "unauthorized" | "malformed"; message: string };
```

`content`, `toolOutput`, and `lastThinkingExcerpt` are truncated by the observer to 8,000 characters before shipping.

### 6.3. Server event, ingester to server to browser

```typescript
export type IServerEvent =
  | { type: "snapshot"; at: number; agents: IAgentState[] }
  | { type: "agent_upsert"; at: number; agent: IAgentState }
  | { type: "agent_removed"; at: number; agentId: string }
  | { type: "environment_upsert"; at: number; environment: IEnvironmentInfo; lastHeartbeatAt: number }
  | { type: "log_append"; at: number; agentId: string; event: ITelemetryFrame };
```

The ingester writes `agent_upsert`, `agent_removed`, `environment_upsert`, and `log_append` to the notify socket. `snapshot` is synthesized by the server from SQLite and is never sent over the socket.

---

## 7. Database Schema

```sql
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;
PRAGMA auto_vacuum=INCREMENTAL;

CREATE TABLE IF NOT EXISTS environments (
    name              TEXT PRIMARY KEY,
    hostname          TEXT NOT NULL,
    platform          TEXT NOT NULL CHECK (platform IN ('darwin','linux')),
    user              TEXT NOT NULL,
    dropped_frames    INTEGER NOT NULL DEFAULT 0,
    last_heartbeat_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
    id                    TEXT PRIMARY KEY,
    session_id            TEXT NOT NULL,
    environment_name      TEXT NOT NULL REFERENCES environments(name) ON DELETE CASCADE,
    pid                   INTEGER NOT NULL,
    harness               TEXT NOT NULL CHECK (harness IN ('claude-code','pi')),
    harness_version       TEXT NOT NULL,
    project_name          TEXT NOT NULL,
    canonical_root        TEXT NOT NULL,
    branch                TEXT NOT NULL,
    is_detached           INTEGER NOT NULL DEFAULT 0,
    is_worktree           INTEGER NOT NULL DEFAULT 0,
    worktree_rel_path     TEXT,
    cwd                   TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (status IN ('BUSY','WAITING_QUESTION','ERROR','IDLE','DONE')),
    summary               TEXT NOT NULL DEFAULT '',
    summary_stale         INTEGER NOT NULL DEFAULT 0,
    phase                 TEXT CHECK (phase IS NULL OR phase IN ('planning','editing','testing','debugging','investigating')),
    active_tool           TEXT,
    active_target         TEXT,
    question              TEXT,
    last_thinking_excerpt TEXT,
    last_error            TEXT,
    tokens_input          INTEGER,
    tokens_output         INTEGER,
    tokens_total          INTEGER,
    started_at            INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL,
    UNIQUE (environment_name, harness, session_id)
);

CREATE INDEX IF NOT EXISTS idx_agents_project     ON agents(project_name);
CREATE INDEX IF NOT EXISTS idx_agents_environment ON agents(environment_name);
CREATE INDEX IF NOT EXISTS idx_agents_status      ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_updated     ON agents(updated_at);

CREATE TABLE IF NOT EXISTS telemetry_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id      TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    event_type    TEXT NOT NULL,
    is_sidechain  INTEGER NOT NULL DEFAULT 0,
    timestamp     INTEGER NOT NULL,
    data_json     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_agent_ts ON telemetry_events(agent_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_ts       ON telemetry_events(timestamp);

CREATE TABLE IF NOT EXISTS observer_cursors (
    environment_name TEXT NOT NULL REFERENCES environments(name) ON DELETE CASCADE,
    last_seq         INTEGER NOT NULL,
    PRIMARY KEY (environment_name)
);
```

`idx_events_ts` exists for the retention pruner, which sweeps by timestamp across all agents. `observer_cursors` holds the highest committed `seq` per environment and is what makes frame deduplication survive an ingester restart.

Token columns exist because `ITokenUsage` is collected on the wire; a field captured and then discarded is a contract the API cannot honor.

---

## 8. Security Model

| Surface | Control |
|---|---|
| Observer to ingester | `ws://` over a private overlay network (Tailscale or WireGuard). Ingester binds the overlay interface only. Bearer token on upgrade, constant-time compared. |
| Ingester notify socket | Unix domain socket, mode `0600`, under `${XDG_RUNTIME_DIR:-/tmp}/agent-status/`. |
| Server API and streams | Binds `127.0.0.1` and the overlay interface. Bearer token required on every endpoint including SSE and WebSocket. Explicit CORS origin allowlist, no wildcard. |
| Database | Mode `0600`. Ingester read-write, server read-only. |
| Summarizer | Thinking excerpts, tool names, and file paths leave the host. Tool output and command results never do. Disable with `--no-summarize`. |
| Claude Code `.key` files | Never opened, never parsed, never shipped, never logged. |
| Tokens | Supplied by `AGENT_STATUS_TOKEN` or `--token-file`. Never logged, never persisted to the cursor file, never included in telemetry. |

The payload on every one of these links includes absolute filesystem paths, branch names, model thinking, tool inputs, and command output. Treating any of it as non-sensitive would be wrong.

---

## 9. Implementation Order

Ordered to make the frame contract real before three packages are built against it.

### Phase 0: Repository foundation
- [ ] Root `justfile` with `build-all`, `test-all`, `lint-all`.
- [ ] `.gitignore`.
- [ ] `packages/ui-web/src/types/index.ts` with §6 types, and the mirrored Go structs in each module.

### Phase 1: Vertical slice
One harness, one host, no LLM. This exists to prove the contract end to end before it is expensive to change.
- [ ] Observer: Claude Code descriptor scanner and liveness.
- [ ] Observer: transcript path resolution with symlink resolution and slug mapping.
- [ ] Observer: byte-offset tailer with cursor persistence and all three reset rules.
- [ ] Observer: WebSocket shipper with seq/ack and replay.
- [ ] Ingester: gateway, dedup, deterministic classifier, SQLite store with migrations.
- [ ] Ingester: notify socket publisher.
- [ ] Server: `/api/v1/agents`, `/api/v1/events`, notify subscriber, SQLite resync.
- [ ] UI: one agent card rendering live status from SSE.
- [ ] Verified: a Claude Code agent started on the host appears in the browser and transitions `BUSY` to `IDLE` to `DONE`.

### Phase 2: Pi and multi-host
- [ ] Observer: Pi session and process scanners, including the darwin `lsof` cwd path.
- [ ] Observer: Pi parser covering both storage generations.
- [ ] Observer: git resolver with worktree and detached-HEAD handling.
- [ ] Ingester: question detection for both harnesses, including the Pi tool-name confirmation in §4.2.3.
- [ ] Verified: agents on two hosts aggregate into one dashboard over the overlay network.

### Phase 3: Summarization
- [ ] Gemini 2.5 Flash client with structured output and phase validation.
- [ ] `claude-haiku-4-5` fallback with the trigger conditions in §4.2.4.
- [ ] Debounce and trigger scheduling.
- [ ] `--no-summarize` verified to leave everything else working.

### Phase 4: Full dashboard
- [ ] Dual grouping views with persisted choice.
- [ ] Action-required banner with jump-to-card.
- [ ] Live execution drawer with sidechain nesting.
- [ ] Fleet header including dropped-frame warning.
- [ ] Disconnected state distinguishable from stale data.

### Phase 5: Hardening
- [ ] Retention pruner and incremental vacuum.
- [ ] Keyset pagination on `/logs`.
- [ ] Reconnection resilience: kill the ingester mid-run, confirm no frame loss on replay.
- [ ] PID-reuse test: confirm a recycled PID does not merge two sessions.
- [ ] Pi atomic-republish test: confirm the prefix-hash reset rule recovers the tail.

---

## 10. Testing and Definition of Done

- Unit test coverage of at least **90%** on every Go package. This bar is uniform; it is not observer-only.
- `ui-web` has component tests for each status rendering and a hook test for reconnection.
- Integration tests cover: observer to ingester replay after disconnect, ingester to server notify to SSE delivery, and cold resync correctness after a server restart.
- Every red/green cycle is verified by temporarily reverting the change and confirming the test fails. A test that passes against the reverted change is not a test.
- The definition of done for each phase includes a **clean review pass by an independent review subagent**, with every finding resolved, not merely acknowledged.

---

## 11. Known Gaps

Recorded rather than papered over.

1. **Permission-prompt detection.** `user-spec.md` names permission checks as a blocking condition. No signal for a *pending* permission prompt has been verified: the Claude Code descriptor `status` shows only `busy` and `idle`, and transcript `permission-mode` records describe mode changes, not pending prompts. Resolving this requires either observing a live permission prompt and identifying its signal, or using the peer socket at `messagingSocketPath`. Until then such an agent reports `BUSY`.
2. **Pi question tool name.** Unconfirmed; see §4.2.3.
3. **The Claude Code peer socket is unused.** `messagingSocketPath` and `peerFeatures: ["notify_idle", ...]` point at a purpose-built local IPC channel with an undocumented protocol. Polling the descriptor is the chosen mechanism because the protocol is not documented and the `.key` credential format is unknown. If gap 1 forces the socket open, revisit §4.1.2 wholesale.
4. **Claude Code hooks were not chosen.** Hooks (`Notification`, `Stop`, `PreToolUse`, `PostToolUse`, `SessionStart`) are the documented push-based event surface and would remove the transcript tailer for Claude Code. They were not chosen because they require mutating the user's `settings.json` on every host, they are Claude-Code-only so Pi still needs a tailer, and a hook that blocks or fails degrades the agent itself. Revisit if transcript parsing proves brittle across Claude Code versions.
5. **Subagents are streamed but not modeled.** `isSidechain` records appear in the drawer and are excluded from parent status. Subagents are not separate cards. Revisit if fan-out work becomes common enough that one card hides real activity.
