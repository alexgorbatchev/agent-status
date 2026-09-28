An offline debugging and transcript replay utility for AI coding agent sessions. It executes the complete telemetry aggregation lifecycle — log parsing, Git resolution, token pricing, state machine classification, and read query verification — in-process against local session transcripts or SQLite databases without running daemons or network services.

# What It Does

- **Replays agent sessions offline** from Claude Code, Pi, OpenAI Codex, and OpenCode transcripts through the ingestion pipeline.
- **Validates state transitions** and lifecycle fidelity (`RUNNING` -> `EXITED`, `IDLE`, `BUSY`, `WAITING_QUESTION`, `ERROR`, `RATE_LIMITED`) and turn completion status (`Completed`) without launching live harnesses.
- **Renders hierarchical fleet views** (`air-debug session fleet` / `air-debug fleet`) grouping sessions by project and linked worktree branches with idle freshness tiering.
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

- `air-debug` runs entirely in-process against an ephemeral in-memory SQLite database (`:memory:`), never mutating live observer cursors or daemon databases unless explicitly exported with `--export-db`.
- Foreign transcript paths are remapped to local checkouts using `--cwd <path>` so Git metadata resolves against local checkouts.
- Sibling subagent directories (`<session_id>/subagents/*.jsonl` or `subagents/*.jsonl`) are automatically discovered and interleaved into the event stream by timestamp.
- Intermediate state inspection is controlled via breakpoints: `--until-line <N>`, `--until-event <type>`, and `--stop-on-error`.
- Dual-mode output (`AGENT=1`) switches from aligned terminal tables and ASCII tree glyphs to token-conservative key-value lines.

# Prerequisites

- [Go](https://go.dev) (v1.26+) - Required for building or running `air-debug` from source.
- [Git](https://git-scm.com) - Used for resolving repository canonical roots and branch names during replay.

# Installation

Download the prebuilt binary archive for your platform from the [latest release](https://github.com/alexgorbatchev/agent-status/releases/latest), replacing `X.X.X` with the version shown on that page.

```bash
# macOS (Apple Silicon)
curl -sSL https://github.com/alexgorbatchev/agent-status/releases/latest/download/air-debug_X.X.X_darwin_arm64.tar.gz | tar -xz -C ~/.local/bin
```

# Quick Start

```bash
# Replay a session transcript or session ID (auto-detects harness)
air-debug session replay ~/.claude/sessions/session-abc.jsonl

# Replay with turn-by-turn state transition diffs
air-debug session replay --diff session-abc.jsonl

# Compare transcript replay against a live agent-status database
air-debug session replay --db ~/.local/share/agent-status/agent-status.db session-abc.jsonl

# Directly inspect persisted agent state in SQLite database
air-debug db inspect 01a0bd5b-0024-7222-9709-194e2fa6a892

# Audit harness process discovery and session pairing across all harnesses
air-debug scanner audit

# Audit process pairing filtered by harness and working directory
air-debug scanner audit --harness pi --cwd /Users/dev/agent-status

# List running harness processes and pairing status
air-debug process list

# Validate session transcript against parser, classifier, and query assertions
air-debug session validate session-abc.jsonl

# View terminal UI card representation of agent session state
air-debug session view session-abc.jsonl

# View hierarchical fleet tree across transcripts or SQLite database
air-debug session fleet ~/.claude/sessions/*.jsonl
air-debug session fleet .tmp/dev-stack/data/agent-status.db

# List sessions inside an OpenCode SQLite database
air-debug session list ~/.local/share/opencode/opencode.db

# Inspect ingested telemetry event stream from live database
air-debug events 01a0b64b-b517-7569-9c97-dac1d07ce4dd --limit 10
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

### `air-debug session replay`

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

### `air-debug session validate`

| Flag                | Short  | Default | Description                                                          |
| :------------------ | :----- | :------ | :------------------------------------------------------------------- |
| `--harness <name>`  | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`) |
| `--session-id <id>` | (none) | `""`    | OpenCode session ID to validate                                      |
| `--cwd <path>`      | (none) | `""`    | Override transcript working directory for local Git resolution       |
| `--db <path>`       | (none) | `""`    | Path to `agent-status.db` for discrepancy comparison (or `auto`)     |
| `--json`            | (none) | `false` | Output validation report as JSON                                     |
| `--env <label>`     | (none) | `debug` | Environment label override                                           |

### `air-debug session view`

| Flag                   | Short  | Default | Description                                                          |
| :--------------------- | :----- | :------ | :------------------------------------------------------------------- |
| `--harness <name>`     | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`) |
| `--session-id <id>`    | (none) | `""`    | OpenCode session ID to view                                          |
| `--cwd <path>`         | (none) | `""`    | Override transcript working directory for local Git resolution       |
| `--env <label>`        | (none) | `debug` | Environment label override                                           |
| `--until-line <N>`     | (none) | `0`     | Halt replay after processing source line N                           |
| `--until-event <type>` | (none) | `""`    | Halt replay after encountering event type                            |
| `--json`               | (none) | `false` | Output final agent state as JSON                                     |

### `air-debug session fleet` / `air-debug fleet`

| Flag               | Short  | Default | Description                                                          |
| :----------------- | :----- | :------ | :------------------------------------------------------------------- |
| `--db <path>`      | (none) | `auto`  | Path to `agent-status.db` (auto-detects if omitted)                  |
| `--cwd <path>`     | (none) | `""`    | Override transcript working directory for local Git resolution       |
| `--env <label>`    | (none) | `debug` | Environment label override                                           |
| `--harness <name>` | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`) |
| `--json`           | (none) | `false` | Output fleet agents as JSON                                          |

### `air-debug db inspect`

| Flag          | Short  | Default | Description                                         |
| :------------ | :----- | :------ | :-------------------------------------------------- |
| `--db <path>` | (none) | `auto`  | Path to `agent-status.db` (auto-detects if omitted) |
| `--json`      | (none) | `false` | Output persisted session record as JSON             |

### `air-debug events`

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

### `air-debug scanner audit` / `air-debug process list`

| Flag               | Short  | Default | Description                                                   |
| :----------------- | :----- | :------ | :------------------------------------------------------------ |
| `--harness <name>` | (none) | `""`    | Filter audit by harness (`claude`, `pi`, `codex`, `opencode`) |
| `--cwd <path>`     | (none) | `""`    | Filter audit by working directory                             |
| `--json`           | (none) | `false` | Output audit results as JSON                                  |
