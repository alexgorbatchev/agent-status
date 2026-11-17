A debugging and transcript replay utility for AI coding agent sessions. It replays local transcripts and inspects SQLite databases offline, discovers live local sessions, and queries `agent-server` to list running agents across monitored machines.

# What It Does

- **Replays agent sessions offline** from Claude Code, Pi, OpenAI Codex, and OpenCode transcripts through the ingestion pipeline.
- **Validates state transitions** and lifecycle fidelity (`RUNNING` -> `EXITED`, `IDLE`, `BUSY`, `WAITING_QUESTION`, `ERROR`, `RATE_LIMITED`) and turn completion status (`Completed`) without launching live harnesses.
- **Renders hierarchical fleet views** (`agent-debug session fleet` / `agent-debug fleet`) grouping sessions by project and linked worktree branches with idle freshness tiering.
- **Lists running agents for shell selection** (`agent-debug agent list | fzf`) with live local PID checks and an option to include other monitored machines.
- **Reports observed skills, harness commands, and MCP tools** (`agent-debug agent usage <agent-id>`) with counts, timestamps, supporting evidence, and tracking coverage.
- **Audits token pricing and usage** against static pricing models to diagnose calculation drift.
- **Inspects turn-by-turn state diffs** (`--diff`) to isolate the exact turn where state corrupts.
- **Verifies read-side queries** (`GetAgentByID`, `GetAgentLogs`) against an in-memory SQLite store to guarantee dashboard renderability.

# How It Works

- Auto-detects agent harness format from JSONL lines or SQLite headers.
- Parses logs using pure Go parsing routines (`github.com/alexgorbatchev/agent-parser`).
- Resolves Git repositories, branches, and worktrees (`packages/agent-observer/git`).
- Classifies agent state, calculates pricing, and accumulates token usage (`packages/agent-ingester/classifier`).
- Synthesizes lifecycle boundaries (`run_started`, `run_exited`) when transcripts do not persist OS process boundaries.
- Executes read-side queries (`packages/agent-server/store`) against the populated database to verify schema compliance.

# How it Really Works

- Session replay runs in-process against an ephemeral in-memory SQLite database (`:memory:`), never mutating live observer cursors or daemon databases unless explicitly exported with `--export-db`.
- `agent list --all` queries `agent-server` over HTTP using the configured server URL and bearer token. Requests time out after 10 seconds and do not follow redirects; use the final API base URL.
- Foreign transcript paths are remapped to local checkouts using `--cwd <path>` so Git metadata resolves against local checkouts.
- Sibling subagent directories (`<session_id>/subagents/*.jsonl` or `subagents/*.jsonl`) are automatically discovered and interleaved into the event stream by timestamp.
- Intermediate state inspection is controlled via breakpoints: `--until-line <N>`, `--until-event <type>`, and `--stop-on-error`.
- Dual-mode output (`AGENT=1`) switches from aligned terminal tables and ASCII tree glyphs to token-conservative key-value lines.
- Command results go to stdout. Routine diagnostic logs are suppressed unless `--debug` is set; diagnostics, warnings, and errors go to stderr through the shared structured logger. Command failures are logged once as `command_failed`, exit with status 1, and do not print usage. HTTP failures include the attempted method and endpoint, with specific explanations for connection refusal, DNS failure, timeout, and TLS certificate verification failure. Credentials and response bodies remain excluded.

# Prerequisites

- [Go](https://go.dev) (v1.26+) - Required for building or running `agent-debug` from source.
- [Git](https://git-scm.com) - Used for resolving repository canonical roots and branch names during replay.

# Installation

Download the prebuilt binary archive for your platform from the [latest release](https://github.com/alexgorbatchev/agent-status/releases/latest), replacing `X.X.X` with the version shown on that page.

```bash
# macOS (Apple Silicon)
curl -sSL https://github.com/alexgorbatchev/agent-status/releases/latest/download/agent-debug_X.X.X_darwin_arm64.tar.gz | tar -xz -C ~/.local/bin
```

# Quick Start

```bash
# Replay a session transcript or session ID (auto-detects harness)
agent-debug session replay ~/.claude/sessions/session-abc.jsonl

# Replay with turn-by-turn state transition diffs
agent-debug session replay --diff session-abc.jsonl

# Compare transcript replay against a live agent-status database
agent-debug session replay --db ~/.local/share/agent-status/agent-status.db session-abc.jsonl

# Directly inspect persisted agent state in SQLite database
agent-debug db inspect 01a0bd5b-0024-7222-9709-194e2fa6a892

# Audit harness process discovery and session pairing across all harnesses
agent-debug scanner audit

# Audit process pairing filtered by harness and working directory
agent-debug scanner audit --harness pi --cwd /Users/dev/agent-status

# List running harness processes and pairing status
agent-debug process list

# Select a running local agent, including idle and waiting agents
agent-debug agent list | fzf

# Include running agents on other monitored machines
export AGENT_SERVER_HTTP_URL=https://agents.example.com
# Supply AGENT_STATUS_TOKEN through your environment or secret manager.
agent-debug agent list --all | fzf

# Inspect observed skills, harness commands, and MCP calls for an API agent ID
agent-debug agent usage 'env-id:claude-code:session-id' --json

# Validate session transcript against parser, classifier, and query assertions
agent-debug session validate session-abc.jsonl

# View terminal UI card representation of agent session state
agent-debug session view session-abc.jsonl

# View hierarchical fleet tree across transcripts or SQLite database
agent-debug session fleet ~/.claude/sessions/*.jsonl
agent-debug session fleet .tmp/dev-stack/data/agent-status.db

# List sessions inside an OpenCode SQLite database
agent-debug session list ~/.local/share/opencode/opencode.db

# Inspect ingested telemetry event stream from live database
agent-debug events 01a0b64b-b517-7569-9c97-dac1d07ce4dd --limit 10
```

Sample Output:

```
--------------------------------------------------------------------------------
Session Replay Summary (pi)
--------------------------------------------------------------------------------
  Session ID:      01a0b64b-b517-7569-9c97-dac1d07ce4dd
  Agent ID:        env-debug:pi:01a0b64b-b517-7569-9c97-dac1d07ce4dd
  Project:         my-repo (branch: main)
  Directory:       /Users/you/development/my-repo
  Title:           Implement offline session replay tool
  Action:          Idle
  Lifecycle:       EXITED
  Activity:        IDLE
  Token Usage:     Input: 951268 | Output: 29040 | Total: 6960642
  Total Cost:      $0.2563 USD
  Frames:          310 frames replayed in 109 ms
--------------------------------------------------------------------------------
[OK] Read-side query verification passed (GetAgentByID, 310 logs scanned)
--------------------------------------------------------------------------------
```

# Options & Flags

### Global flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--debug` | (none) | `false` | Write diagnostic logs to stderr; warnings and errors are always visible |

### `agent-debug agent list`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--all` | (none) | `false` | Query `agent-server` for running agents across monitored machines |
| `--server-url <url>` | (none) | `AGENT_SERVER_HTTP_URL` or `http://127.0.0.1:4502` | HTTP(S) base URL of `agent-server`; requires `--all` and accepts a reverse-proxy path prefix |
| `--db <path>` | (none) | `auto` | Inspect a local monitoring database offline instead of discovering local sessions; incompatible with `--all` |

The default discovers local sessions directly from harness storage and verifies their running processes. It works without a dashboard, daemon, or agent-status database. It respects the harness discovery paths: `CLAUDE_SESSIONS_DIR`, `PI_SESSIONS_DIR` (or `PI_CODING_AGENT_DIR`), `CODEX_SESSIONS_DIR` (or `CODEX_HOME`), and `OPENCODE_DB_PATH`, with the harnesses' standard locations as defaults. It reads harness storage without modifying it. Titles can be empty when discovery metadata has no name.

`--all` calls `GET /api/v1/agents?lifecycle=RUNNING` on `agent-server`; it does not read or require a local monitoring database. `--server-url` overrides `AGENT_SERVER_HTTP_URL`. The default server URL is `http://127.0.0.1:4502`.

Authentication uses `AGENT_STATUS_TOKEN`, containing the same shared bearer token configured on `agent-server`. The CLI reads it from the environment, sends it only in the `Authorization` header, and does not store it or expose a token flag. Supply it through your secret manager or a private shell environment; use HTTPS for remote connections. URLs containing credentials, query parameters, or fragments are rejected. If the variable is unset, the request has no authorization header; this works only when the server permits unauthenticated access. A rejected token produces an error on stderr and no list on stdout.

Tokens are manually provisioned shared secrets; the server has no login, token issuance, expiry, per-client revocation, or read-only scopes. Rotate the token by changing `AGENT_STATUS_TOKEN` on the server, restarting it, and updating each client. Configure a nonempty server token for authenticated deployments.

An explicit `--db <path>` selects offline monitoring data for the local observer environment. For `--db auto`, auto-detection checks `AGENT_DB_PATH`, then `$XDG_DATA_HOME/agent-status/agent-status.db` (default `~/.local/share/agent-status/agent-status.db`), `.tmp/dev-stack/data/agent-status/agent-status.db` relative to the working directory, and `~/.local/state/agent-status/agent-status.db`. An explicitly set `AGENT_DB_PATH` overrides the search. The database is opened read-only.

Stdout contains one headerless, tab-separated row per session in this order: **PID, harness, session ID, hostname, working directory, title**. Fields are not truncated; ANSI escapes and control characters are sanitized, with tabs and newlines inside fields replaced by spaces. Output is identical with `AGENT=0` and `AGENT=1`. An empty list writes nothing and succeeds; errors go to stderr and exit with status 1.

Local PIDs must remain alive, and start times are compared when available using the watcher's 60-second tolerance. Idle and waiting agents remain selectable. With an explicit `--db`, local entries are restricted to the current machine-and-user observer environment. With `--all`, local entries still receive the live check; remote entries use the observer-reported running lifecycle and require a fresh heartbeat, reported by the server with a 30-second threshold. Remote liveness can therefore lag behind the remote process. Exited sessions and ended runs are excluded. Server-backed listing only includes sessions observed by that server's ingestion pipeline.

### `agent-debug agent usage <agent-id>`

| Flag | Default | Description |
| :--- | :------ | :---------- |
| `--server-url <url>` | `AGENT_SERVER_HTTP_URL` or `http://127.0.0.1:4502` | HTTP(S) base URL of `agent-server` |
| `--json` | `false` | Output usage entries, latest evidence, and coverage as JSON |

Calls `GET /api/v1/agents/{id}/usage` with the same environment-based bearer authentication and HTTP safeguards as `agent list --all`. Use the composite `id` returned by `GET /api/v1/agents`, or `agentId` from replay JSON. The session ID column in `agent list` is the harness session ID, which is not the composite API ID.

The response contains `skills`, `commands`, and `mcp` arrays. Each entry includes its name, operation, count, first and last observed epoch-millisecond timestamps, and latest supporting event. MCP entries also identify the server. Root and subagent observations are combined; latest evidence identifies the originating subagent when present. Operations remain separate: `invoke` records an invocation attempt, `load` records a skill injection or read attempt, and `request` records a slash-prefixed prompt without claiming execution. Counts do not establish successful execution. Skill file reads retain the recorded path as their identifier; tool invocations and injected blocks use their explicit skill name.

Only observed usage is reported. Shell commands, installed skill catalogs, conversational mentions, tool results, and credentials are excluded. Claude command markers and Pi `/skill:name` expansions identify command invocations; other recorded slash prompts count as requests. Pi extension commands and expanded prompt-template names can be absent from transcripts. Codex client-side commands and skill reads through shell tools are also outside coverage. MCP tracking requires explicit `mcp__server__tool` names; unqualified tool names cannot identify the MCP server.

The ingester creates schema version 10 and backfills retained telemetry on startup. Usage observations survive the 48-hour raw telemetry cleanup and are deleted with their agent. Replaying the same native tool call ID does not increase its count; subagent call IDs are scoped separately. Events without a native call ID use a timestamp-based identity, so identical observations at the same timestamp may be undercounted. History already pruned before migration cannot be recovered. `coverage` and `history` describe these limits, including when arrays are empty.

### `agent-debug session replay`

| Flag                   | Short  | Default | Description                                                                |
| :--------------------- | :----- | :------ | :------------------------------------------------------------------------- |
| `--harness <name>`     | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`)       |
| `--session-id <id>`    | (none) | `""`    | OpenCode session ID to replay (required if database has multiple sessions) |
| `--cwd <path>`         | (none) | `""`    | Override transcript working directory for local Git resolution             |
| `--diff`               | (none) | `false` | Display turn-by-turn state mutations during replay                         |
| `--db <path>`          | (none) | `""`    | Path to `agent-status.db` for discrepancy comparison (or `auto`)           |
| `--until-line <N>`     | (none) | `0`     | Halt replay after processing source line N                                 |
| `--until-event <type>` | (none) | `""`    | Halt replay after encountering event type                                  |
| `--stop-on-error`      | (none) | `false` | Halt immediately on first parse or state machine error                     |
| `--json`               | (none) | `false` | Output final state and verification results as JSON                        |
| `--export-db <path>`   | (none) | `""`    | Path to save populated SQLite database on disk                             |
| `--summarize`          | (none) | `false` | Enable live LLM summarizer provider verification                           |
| `--env <label>`        | (none) | `debug` | Environment label override                                                 |

### `agent-debug session validate`

| Flag                | Short  | Default | Description                                                          |
| :------------------ | :----- | :------ | :------------------------------------------------------------------- |
| `--harness <name>`  | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`) |
| `--session-id <id>` | (none) | `""`    | OpenCode session ID to validate                                      |
| `--cwd <path>`      | (none) | `""`    | Override transcript working directory for local Git resolution       |
| `--db <path>`       | (none) | `""`    | Path to `agent-status.db` for discrepancy comparison (or `auto`)     |
| `--json`            | (none) | `false` | Output validation report as JSON                                     |
| `--env <label>`     | (none) | `debug` | Environment label override                                           |

### `agent-debug session view`

| Flag                   | Short  | Default | Description                                                          |
| :--------------------- | :----- | :------ | :------------------------------------------------------------------- |
| `--harness <name>`     | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`) |
| `--session-id <id>`    | (none) | `""`    | OpenCode session ID to view                                          |
| `--cwd <path>`         | (none) | `""`    | Override transcript working directory for local Git resolution       |
| `--env <label>`        | (none) | `debug` | Environment label override                                           |
| `--until-line <N>`     | (none) | `0`     | Halt replay after processing source line N                           |
| `--until-event <type>` | (none) | `""`    | Halt replay after encountering event type                            |
| `--json`               | (none) | `false` | Output final agent state as JSON                                     |

### `agent-debug session fleet` / `agent-debug fleet`

| Flag               | Short  | Default | Description                                                          |
| :----------------- | :----- | :------ | :------------------------------------------------------------------- |
| `--db <path>`      | (none) | `auto`  | Path to `agent-status.db` (auto-detects if omitted)                  |
| `--cwd <path>`     | (none) | `""`    | Override transcript working directory for local Git resolution       |
| `--env <label>`    | (none) | `debug` | Environment label override                                           |
| `--harness <name>` | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`) |
| `--json`           | (none) | `false` | Output fleet agents as JSON                                          |

### `agent-debug db inspect`

| Flag          | Short  | Default | Description                                         |
| :------------ | :----- | :------ | :-------------------------------------------------- |
| `--db <path>` | (none) | `auto`  | Path to `agent-status.db` (auto-detects if omitted) |
| `--json`      | (none) | `false` | Output persisted session record as JSON             |

### `agent-debug events`

| Flag                  | Short  | Default | Description                                                           |
| :-------------------- | :----- | :------ | :-------------------------------------------------------------------- |
| `--db <path>`         | (none) | `auto`  | Path to `agent-status.db` (or auto-detect)                            |
| `--limit <N>`         | (none) | `20`    | Maximum number of telemetry events to retrieve                        |
| `--type <types>`      | (none) | `""`    | Filter by comma-separated event types (e.g. `run_started,run_exited`) |
| `--subagent-id <id>`  | (none) | `""`    | Filter by specific subagent ID                                        |
| `--include-subagents` | (none) | `true`  | Include events from subagents                                         |
| `--json`              | (none) | `false` | Output events as JSON                                                 |
| `--desc`              | (none) | `true`  | Order events by ID descending (newest first)                          |
| `--asc`               | (none) | `false` | Order events by ID ascending (oldest first)                           |
| `--id <id>`           | (none) | `0`     | Inspect specific telemetry event by ID                                |

### `agent-debug scanner audit` / `agent-debug process list`

| Flag               | Short  | Default | Description                                                   |
| :----------------- | :----- | :------ | :------------------------------------------------------------ |
| `--harness <name>` | (none) | `""`    | Filter audit by harness (`claude`, `pi`, `codex`, `opencode`) |
| `--cwd <path>`     | (none) | `""`    | Filter audit by working directory                             |
| `--json`           | (none) | `false` | Output audit results as JSON                                  |
