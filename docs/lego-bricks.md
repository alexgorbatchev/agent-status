---
created_on: 2026-09-18 22:30
last_modified: 2026-09-19 10:35
status: current
---

# Go Implementation Building Blocks (Lego Bricks): Agent Status

This document provides a verified, code-level catalog of Go libraries, tools, and extractable open-source components for implementing `agent-status`.

Every building block in this document was manually reviewed by inspecting source code fetched via `gitsnip`. All source trees for selected packages and reference implementations are retained locally under `.tmp/gitsnip/` for direct extraction and reference during implementation.

---

## 1. Local Source Repository Map (`.tmp/gitsnip/`)

The following source checkouts are retained on disk:

| Repository / Module                            | Disk Location                          | Key Subdirectories & Files Inspected                                     |
| ---------------------------------------------- | -------------------------------------- | ------------------------------------------------------------------------ |
| `github.com/shirou/gopsutil`                   | `.tmp/gitsnip/gopsutil/process`        | `process_darwin.go`, `process_linux.go`, `process.go`                    |
| `github.com/ProjAnvil/claude-agent-sdk-golang` | `.tmp/gitsnip/claude-agent-sdk-golang` | `sessions.go`, `parser.go`, `types.go`                                   |
| `github.com/sonnes/chitragupt`                 | `.tmp/gitsnip/chitragupt`              | `reader/claude/claude.go`, `core/transcript.go`, `core/session_entry.go` |
| `github.com/bpowers/go-claudecode`             | `.tmp/gitsnip/go-claudecode`           | `transcript.go`, `messages.go`, `sessions.go`                            |
| `github.com/janekbaraniewski/openusage`        | `.tmp/gitsnip/openusage/internal`      | `detect/claude_code.go`, `providers/claude_code/`, `providers/pi/`       |
| `github.com/valpere/session-indexer`           | `.tmp/gitsnip/session-indexer`         | `internal/mine/parse.go`, `internal/mine/mine.go`                        |
| `github.com/robzolkos/claude-session-export`   | `.tmp/gitsnip/claude-session-export`   | `internal/session/discover.go`, `internal/session/parse.go`              |
| `github.com/nicknisi/sessions`                 | `.tmp/gitsnip/nicknisi-sessions/src`   | `pi-tree.ts`, `opencode.ts`, `report/parsers/`                           |
| `github.com/neochoon/agenthud`                 | `.tmp/gitsnip/agenthud`                | `src/data/sessionLiveness.ts`, `src/data/providers/claude.ts`            |
| `github.com/KyleAMathews/claude-code-ui`       | `.tmp/gitsnip/claude-code-ui`          | `packages/daemon/src/status-machine.ts`, `summarizer.ts`                 |
| `github.com/joe-re/eyes-on-claude-code`        | `.tmp/gitsnip/eyes-on-claude-code`     | `src-tauri/src/events.rs`                                                |
| `github.com/markojak/harness`                  | `.tmp/gitsnip/harness`                 | `packages/daemon/src/providers/`                                         |
| `github.com/nxadm/tail`                        | `.tmp/gitsnip/tail`                    | `tail.go`, `watch/`                                                      |
| `github.com/Magic-Man-us/cc-session-core`      | `.tmp/gitsnip/cc-session-core`         | `src/cc_session_core/models.py`, `tests/fixtures/`                       |

---

## 2. `packages/agent-observer`

### 2.1. Process Introspection & Darwin/Linux CWD Resolution

#### Chosen Building Block: `github.com/shirou/gopsutil/v4/process`

- **Location on Disk**: `.tmp/gitsnip/gopsutil/process`
- **Adoption**: 12k+ stars, standard cross-platform system metrics library in Go.
- **Why Chosen**:
  - `docs/prd.md` §4.1.3 initially prescribed executing `lsof -a -p <pid> -d cwd -Fn` per discovered Pi process per 2-second scan.
  - Manual inspection of `.tmp/gitsnip/gopsutil/process/process_darwin.go` confirmed that `Process.CwdWithContext()` uses Apple's native `libproc` API (`proc_pidinfo` with `PROC_PIDVNODEPATHINFO`) via pure-Go dynamic linking (`purego`), requiring zero Cgo.
  - **Empirical Benchmark**: Measured execution time of `Process.CwdWithContext()` on Darwin is **25.3 microseconds (0.025 ms)**, compared to ~50ms for `lsof` subprocess execution (~2,000x faster).
  - Also provides `Process.CreateTime()` (start time in milliseconds since epoch) to prevent PID reuse issues where an OS recycles a PID.
- **Linux Implementation**: In `process_linux.go`, it reads `/proc/<pid>/cwd` via `os.Readlink`.
- **Key Code References**:
  - `process_darwin.go` lines 390–420 (`CwdWithContext`, `vnodePathInfo`)
  - `process_darwin.go` lines 95–105 (`createTimeWithContext`)

---

### 2.2. Claude Code Discovery & Transcript Parsers

#### Direct Prior Art in Go:

1. **Session & Transcript Parser**: `github.com/sonnes/chitragupt` & `github.com/ProjAnvil/claude-agent-sdk-golang`
   - **Locations on Disk**: `.tmp/gitsnip/chitragupt/reader/claude/claude.go` and `.tmp/gitsnip/claude-agent-sdk-golang/sessions.go`
   - **Why Chosen**: Provides a complete Go struct model mirroring the exact Claude Code JSONL transcript record types:
     ```go
     type rawEntry struct {
         Type        string      `json:"type"`
         UUID        string      `json:"uuid"`
         ParentUUID  *string     `json:"parentUuid"`
         SessionID   string      `json:"sessionId"`
         Timestamp   string      `json:"timestamp"`
         CWD         string      `json:"cwd"`
         GitBranch   string      `json:"gitBranch"`
         IsSidechain bool        `json:"isSidechain"`
         Message     rawMessage  `json:"message"`
     }
     type rawContentBlock struct {
         Type      string `json:"type"`       // "thinking", "tool_use", "text"
         Text      string `json:"text"`
         Thinking  string `json:"thinking"`
         Name      string `json:"name"`       // tool name (e.g. "AskUserQuestion", "Bash")
         ToolUseID string `json:"tool_use_id"`
         Content   any    `json:"content"`
         IsError   bool   `json:"is_error"`
     }
     ```
   - **Buffer Handling**: Both packages configure `bufio.Scanner` with a 10MB–16MB max buffer (`scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)`) to absorb large tool outputs and file snapshots without scanner truncation errors.
2. **Descriptor Discovery**: `github.com/janekbaraniewski/openusage`
   - **Location on Disk**: `.tmp/gitsnip/openusage/internal/providers/claude_code/`
   - **Why Chosen**: Demonstrates discovery across symlink paths (`${XDG_DATA_HOME}/ai-registry/claude-code/sessions` and `~/.claude/sessions`), reading `<pid>.json` descriptors, ignoring `*.key` files, and extracting `status: "busy" | "idle"`.

---

### 2.3. Pi Session Discovery & Tree Parsing

#### Reference Prior Art: `github.com/nicknisi/sessions` & `github.com/janekbaraniewski/openusage`

- **Locations on Disk**: `.tmp/gitsnip/nicknisi-sessions/src/pi-tree.ts` and `.tmp/gitsnip/openusage/internal/providers/pi/session.go`
- **Why Chosen**:
  - Authoritatively documents and implements Pi's tree structure where entries contain `id` and `parentId`.
  - Reverses Pi's CWD directory naming convention: `--${cwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--`.
  - Reads the first JSON line header to extract authoritative session metadata (`type: "session"`, `id`, `cwd`, `timestamp`).
  - Implements tool-call parsing: Pi emits `role: "toolResult"`, `toolCallId`, `toolName`, `isError`, and `content`.
- **Go Representation**:
  ```go
  type PiSessionHeader struct {
      Type      string `json:"type"`
      ID        string `json:"id"`
      Timestamp string `json:"timestamp"`
      CWD       string `json:"cwd"`
      Version   int    `json:"version"`
  }
  type PiEntry struct {
      ID        string         `json:"id"`
      ParentID  *string        `json:"parentId"`
      Type      string         `json:"type"`
      Timestamp string         `json:"timestamp"`
      Message   *PiMessageBody `json:"message,omitempty"`
  }
  ```

---

### 2.4. File Watching & Prefix-Hash Byte-Offset Tailer

#### Chosen Libraries: `github.com/fsnotify/fsnotify` + Custom Atomic Cursor Engine

- **Location on Disk**: `.tmp/gitsnip/tail` (reference implementation)
- **Adoption**: `fsnotify` is the standard Go filesystem event watcher (10k+ stars).
- **Implementation Strategy**:
  - `fsnotify` monitors directories for `Create` and `Write` events.
  - Standard file tailers (like `nxadm/tail`) do not support Pi's append-then-atomic-republish recovery (where a file is rewritten in-place with a new inode).
  - To fulfill `docs/prd.md` §4.1.5, we implement a lightweight cursor manager:
    - Cursor key: `(device, inode)`
    - Cursor payload: `offset` (int64), `size` (int64), `inode` (uint64), `prefixHash` (SHA-256 of first 512 bytes).
    - Reset triggers: inode change, file size < offset, or mismatch of the 512-byte prefix hash.
    - Flushed atomically to `${XDG_CACHE_HOME}/agent-status/observer-cursors.json` via tempfile + rename.

---

### 2.5. Git Root & Worktree Resolver

#### Chosen Building Block: Standard Library `os/exec` + In-Memory Mutex TTL Cache

- **Adoption**: Standard Library (`os/exec`, `sync.RWMutex`).
- **Why Chosen**:
  - Branch resolution for Claude Code requires zero subprocesses because `gitBranch` is present on every transcript line.
  - For Pi and worktree topology resolution, two lightweight commands are executed:
    1. `git rev-parse --show-toplevel` (Worktree root)
    2. `git rev-parse --path-format=absolute --git-common-dir` (Shared Git directory)
  - When common dir $\neq$ `<toplevel>/.git`, `isWorktree: true` and `relPath` is calculated relative to canonical root.
  - Results cached with a 30-second TTL per CWD; invalidated immediately if a new `gitBranch` appears in telemetry.

---

### 2.6. Observer-to-Ingester WebSocket Transport

#### Chosen Building Blocks:

1. **WebSocket Framing**: `github.com/coder/websocket` (formerly `nhooyr.io/websocket`)
   - **Adoption**: 5k+ stars. Modern, context-first, zero-allocation Go WebSocket client/server.
   - **Features**: Custom upgrade HTTP headers (`Authorization: Bearer <token>`), native Ping/Pong, structured message reader/writer.
2. **Ring Buffer for Seq/Ack**: `github.com/gammazero/deque` or standard circular slice
   - **Adoption**: 1.1k+ stars.
   - **Features**: Holds 1,000 unacknowledged `ITelemetryFrame` structs; drops oldest frames on overflow and increments `dropped_frames` counter.
3. **Exponential Backoff & Jitter**: `github.com/cenkalti/backoff/v4`
   - **Adoption**: 4k+ stars.
   - **Configuration**: `InitialInterval: 500ms`, `Multiplier: 2.0`, `MaxInterval: 30s`, `RandomizationFactor: 0.5`.

---

## 3. `packages/agent-ingester`

### 3.1. Gateway & Deterministic Classifier

#### Chosen Building Blocks:

1. **WebSocket Ingest Server**: `github.com/coder/websocket`
   - Binds exclusively to the private overlay network interface IP.
   - Validates Bearer token using constant-time comparison: `crypto/subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1`.
   - Validates `version: "1.0"` and deduplicates on `(environment, seq)`.
2. **Deterministic State Machine**:
   - Reference: `.tmp/gitsnip/agenthud/src/data/sessionLiveness.ts` and `.tmp/gitsnip/claude-code-ui/packages/daemon/src/status-machine.ts`.
   - Ordered pure-Go reducer rules:
     1. `run_exited` $\to$ `lifecycle: EXITED`
     2. `run_started` $\to$ `lifecycle: RUNNING`
     3. `question_prompt` / `AskUserQuestion` $\to$ `activity: WAITING_QUESTION` (stores question text)
     4. `tool_result` (`isError: true`) $\to$ `activity: ERROR` (stores `lastError` truncated to 2,000 chars)
     5. `tool_call` $\to$ `activity: BUSY` (records `activeTool` and `activeTarget`)
     6. `turn_end` $\to$ `activity: IDLE` (clears `activeTool`, `activeTarget`, and `question`)
     7. `thinking` / `turn_start` $\to$ `activity: BUSY`

---

### 3.2. Debounced AI Summarizer

#### Chosen Building Blocks:

1. **Unified Multi-Provider Completion Client**: `github.com/mozilla-ai/any-llm-go`
   - **Adoption**: Mozilla.ai open-source Go library providing normalized multi-provider interfaces.
   - **Providers Configured**:
     - OpenAI-compatible / LiteLLM Proxy: `providers/openai` with `anyllm.WithBaseURL(url)` supporting self-hosted instances e.g. `https://litellm.example.com` via `LITELLM_BASE_URL` / `LITELLM_API_KEY` (alias `LITELLM_KEY`).
     - Direct Google Gemini: `providers/gemini`
     - Direct Anthropic Claude: `providers/anthropic`
   - **Features**: Single client abstraction, structured schema enforcement, unified error handling on 429/5xx with retry backoff.
2. **Debounce Rate Limiter**: `golang.org/x/time/rate` + `sync.Map`
   - Enforces the 12-second debounce window per active agent.

---

### 3.3. SQLite Store & Migrations

#### Chosen Building Block: `github.com/ncruces/go-sqlite3`

- **Adoption**: Modern pure-Go / Wasm-based SQLite driver providing complete Cgo-free execution across Darwin and Linux.
- **Why Chosen**: Eliminates Cgo build toolchain requirements and cross-compilation headaches while supporting native WAL mode, high concurrency, and standard `database/sql` interfaces.
- **Connection Configuration**:
  ```go
  import (
      _ "github.com/ncruces/go-sqlite3/driver"
      _ "github.com/ncruces/go-sqlite3/embed"
  )

  dsn := "file:" + dbPath + "?" +
      "_pragma=journal_mode(wal)&" +
      "_pragma=synchronous(normal)&" +
      "_pragma=busy_timeout(5000)&" +
      "_pragma=foreign_keys(on)&" +
      "_pragma=auto_vacuum(incremental)"

  db, err := sql.Open("sqlite3", dsn)
  ```
- **Connection Pool Tuning**:
  ```go
  db.SetMaxOpenConns(max(4, runtime.GOMAXPROCS(0)))
  db.SetMaxIdleConns(max(4, runtime.GOMAXPROCS(0)))
  ```
- **Migrations**: Custom transactional runner checking `PRAGMA user_version` sequentially inside single transactions.
- **Retention Pruning**: Background `time.Ticker` (hourly) executing cleanup queries followed by `PRAGMA incremental_vacuum(1000)`.

---

### 3.4. Ingester-to-Server IPC Notify Publisher

#### Chosen Building Block: Standard Library `net.Listen("unix", ...)`

- **Socket Path**: `${XDG_RUNTIME_DIR:-/tmp}/agent-status/notify.sock` (permissions `0600`).
- **Protocol**: Newline-delimited JSON envelopes (`IServerEvent`).
- **Resilience**: Non-blocking broadcast channel per subscriber; drops and closes connection if client buffer is saturated for $>2$ seconds.

---

## 4. `packages/agent-server`

### 4.1. HTTP API & CORS

#### Chosen Building Blocks:

1. **HTTP Router**: `github.com/go-chi/chi/v5`
   - **Adoption**: 18k+ stars, 100% compatible with `net/http` standard handlers.
   - **Endpoints**:
     - `GET /api/v1/status`
     - `GET /api/v1/projects`
     - `GET /api/v1/environments`
     - `GET /api/v1/agents`
     - `GET /api/v1/agents/:id`
     - `GET /api/v1/agents/:id/logs` (keyset pagination with `before` cursor)
2. **CORS Middleware**: `github.com/rs/cors`
   - Enforces strict origin matching against `--cors-origin` (default `http://localhost:3000`).

---

### 4.2. Streaming Broadcaster (SSE & WebSockets)

#### Chosen Building Blocks:

1. **Server-Sent Events (SSE)**: `github.com/tmaxmax/go-sse` or native `http.Flusher`
   - Broadcasts `snapshot` (synthesized from SQLite on connect), `agent_upsert`, `agent_removed`, `environment_upsert`, and `log_append`.
   - Sends SSE heartbeat comment every 15 seconds (`: heartbeat\n\n`).
2. **WebSocket Stream**: `github.com/coder/websocket` on `GET /ws/events`.
3. **Notify Subscriber**: `net.Dial("unix", ...)` with 1-second auto-reconnect loop. On connect/reconnect, triggers cold SQLite resync before streaming socket events.

---

## 5. CLI Architecture & Tooling

### Chosen Building Blocks: `github.com/spf13/cobra` + `github.com/spf13/pflag`

- **Adoption**: 44k+ stars, industry standard Go CLI framework.
- **Used In**: `cmd/air-observer/main.go`, `cmd/air-ingester/main.go`, `cmd/air-server/main.go`.
- **Help Tree Generation**: `github.com/spf13/cobra/doc` generates structured markdown command trees, enabling `just help` to render full CLI surfaces.
