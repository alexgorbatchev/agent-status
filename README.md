A centralized monitoring dashboard and telemetry aggregation pipeline for AI coding agents running across an engineer's machines and worktrees. It observes agent sessions from Claude Code, Pi, OpenAI Codex, and OpenCode, tracks live activity and token costs, and highlights sessions requiring user attention.

# What It Does

- **Monitors agent fleets** across multiple environments, machines, and git worktrees in real time.
- **Tracks lifecycle and activity** to distinguish active runs from exited sessions, surfacing agents waiting on user input or stopped due to errors.
- **Aggregates token usage and costs** with accurate breakdowns for input, output, cache reads, cache writes, and reasoning tokens against standard pricing tables.
- **Streams live updates** using Server-Sent Events (SSE) and Unix domain socket fan-out from local file observers to the web dashboard.
- **Preserves agent history** across resumes and reconnections without losing prior turns or delegated subagent contributions.

# How It Works

- Scans local harness storage directories to detect active sessions from Claude Code, Pi, OpenAI Codex, and OpenCode.
- Tails session logs and transcript files incrementally, resolving git repositories, branches, and worktrees.
- Streams raw events over WebSocket to a central ingester daemon that classifies state, calculates token pricing, and records structured events into SQLite.
- Broadcasts status changes over a Unix domain socket to a lightweight query server.
- Displays live agent status, attention banners, usage breakdowns, and recent activity in a web dashboard.

# How it Really Works

- `agent-observer` uses the standalone [`agent-watcher`](https://github.com/alexgorbatchev/agent-watcher) library to discover sessions, follow JSONL transcripts, and poll OpenCode's read-only SQLite database. It formats events and ships them over a bounded WebSocket queue, advancing transcript offsets only after ingester acknowledgements. Cursors live at `$XDG_CACHE_HOME/agent-status/observer-cursors.json` (or `~/.cache/agent-status/observer-cursors.json`); OpenCode event checkpoints live beside them with the `.opencode` suffix. Checkpoints flush periodically and on shutdown.
- `agent-ingester` receives WebSocket connections at `:4501/ws/ingest` with a 64 MiB read limit, computes token costs using a static model price registry, derives concise action descriptions via asynchronous per-agent queues and specialized prompt templates with Jaccard deduplication, and writes structured records into `$XDG_DATA_HOME/agent-status/agent-status.db` using WAL mode.
- `agent-server` opens the SQLite database strictly in read-only mode, listens for change signals on `$XDG_RUNTIME_DIR/agent-status/notify.sock`, and fans out events to browser clients over Server-Sent Events (`/api/v1/events`) at `:4502`.
- `agent-debug` replays and validates agent session transcripts across Claude Code, Pi, OpenAI Codex, and OpenCode through the ingestion and query pipeline offline, running in-process against an ephemeral in-memory SQLite database without modifying active daemon logs or databases.
- `ui-web` serves a React 19 dashboard on port `3000`, grouping agents by upstream git project repository and observing environment rather than machine-local folder paths, reconnecting to the SSE stream using exponential backoff with jitter to prevent reconnect storms.
- Inactive agents and old event streams are automatically pruned every hour from SQLite according to a 48-hour event and 30-day exited session retention policy.
- Running with `AGENT=1` enables token-conservative, agent-facing CLI output across dev stack orchestration.

# Prerequisites

- [Bun](https://bun.sh) (v1.2+) - Required for running the web server and bundling frontend assets.
- [Go](https://go.dev) (v1.24+) - Required for compiling `agent-observer`, `agent-ingester`, and `agent-server` daemons.
- [Just](https://github.com/casey/just) - Command runner used for build, test, and lifecycle recipes.
- [devhost](https://github.com/alexgorbatchev/devhost) - Native service supervision and shared HTTPS proxy; complete its Caddy download and certificate trust setup first.
- [direnv](https://direnv.net) - Loads the hostname suffix from `.envrc` and optional ignored `.envrc.local`.
- [Git](https://git-scm.com) - Used by observer to resolve project identity, branches, and linked worktrees.

# Installation

Download the prebuilt binary archive for your platform from the [latest release](https://github.com/alexgorbatchev/agent-status/releases/latest), replacing `X.X.X` with the version shown on that page.

```bash
# macOS (Apple Silicon)
curl -sSL https://github.com/alexgorbatchev/agent-status/releases/latest/download/agent-status_X.X.X_darwin_arm64.tar.gz | tar -xz -C ~/.local/bin
```

# Quick Start

1. Install the prerequisites, run `just setup`, and complete devhost's Caddy download and certificate trust setup.
2. Run `direnv allow .`. The dashboard defaults to `https://agent-status.localhost`. For a machine-specific hostname, set `export DEVHOST_DEV_EXT='<suffix>'` in ignored `.envrc.local`; the hostname is `agent-status` followed by that suffix.
3. Run `just run` (or `just start`). This starts the shared proxy and all four native devhost services. Each Go service builds before launch. Ctrl+C or `just stop` stops this stack.
4. Use `just status` to see whether this checkout's stack is running and `just logs` to tail rotating daemon logs. Devhost also exposes service status and logs in its dashboard toolbar.

Devhost allocates loopback ports and starts services in dependency order. UI and `/api/*` share the HTTPS hostname. Application state stays under `.tmp/dev-stack/`: `data/` (SQLite), `cache/` (observer checkpoints), `logs/`, and `run/` (notify socket). Devhost stores its own supervisor and shared proxy state in its standard locations.

Use `AGENT_STATUS_NO_SUMMARIZE=1 just run` to disable external summarization calls. Arguments to `just run` are passed to `devhost start`, for example `just run api` starts the API and its ingester dependency. The old stack script and its flags are retired.

Replay sessions offline with `just debug session-replay-pi <session-id>` or `just debug run session replay <path-to-transcript.jsonl>`.

# Options & Flags

### `agent-observer`

| Flag                                 | Short  | Default                                       | Description                                          |
| :----------------------------------- | :----- | :-------------------------------------------- | :--------------------------------------------------- |
| `--ingester-url <url>`               | (none) | `ws://127.0.0.1:4501/ws/ingest`               | Ingester WebSocket endpoint URL                      |
| `--token <token>`                    | (none) | `""`                                          | Bearer token for ingester authentication             |
| `--env <label>`                      | (none) | `""`                                          | Environment label override                           |
| `--scan-interval <duration>`         | (none) | `2s`                                          | Interval between harness session scans               |
| `--cursor-path <path>`               | (none) | `~/.cache/agent-status/observer-cursors.json` | Path to persistent tailer cursor file                |
| `--cursor-retention <duration>`      | (none) | `24h`                                         | Retention duration for inactive cursor entries       |
| `--cursor-prune-interval <duration>` | (none) | `1h`                                          | Interval between periodic cursor pruning sweeps      |
| `--claude-sessions-dir <path>`       | (none) | `~/.claude/sessions`                          | Custom directory for Claude Code session metadata    |
| `--claude-projects-dir <path>`       | (none) | `~/.claude/projects`                          | Custom directory for Claude Code project transcripts |
| `--pi-sessions-dir <path>`           | (none) | `~/.pi/agent/sessions`                        | Custom directory for Pi session logs                 |
| `--log-level <level>`                | (none) | `info`                                        | Log level (`debug`, `info`, `warn`, `error`)         |
| `--version`                          | `-v`   | `false`                                       | Print version information and exit                   |
| `--help`                             | `-h`   | `false`                                       | Print command line help                              |

### `agent-ingester`

| Flag                        | Short  | Default                                       | Description                                                     |
| :-------------------------- | :----- | :-------------------------------------------- | :-------------------------------------------------------------- |
| `--addr <addr>`             | (none) | `:4501`                                       | HTTP and WebSocket listen address                               |
| `--token <token>`           | (none) | `dev-secret-token`                            | Bearer authentication token                                     |
| `--db <path>`               | (none) | `~/.local/share/agent-status/agent-status.db` | SQLite database file path                                       |
| `--notify-sock <path>`      | (none) | `/tmp/agent-status/notify.sock`               | Unix domain socket path for event notifications                 |
| `--no-summarize`            | (none) | `false`                                       | Disable LLM-based action summarization                          |
| `--litellm-url <url>`       | (none) | `""`                                          | Base URL for OpenAI-compatible or LiteLLM proxy                 |
| `--litellm-key <key>`       | (none) | `""`                                          | API key for OpenAI-compatible or LiteLLM proxy                  |
| `--summarizer-model <name>` | (none) | `""`                                          | Model identifier used for action summarization                  |
| `--log-level <level>`       | (none) | `info`                                        | Log level (`debug`, `info`, `warn`, `error`)                    |
| `--idle-grace-period <dur>` | (none) | `5s`                                          | Grace period before transitioning from BUSY to IDLE on turn end |
| `--version`                 | `-v`   | `false`                                       | Print version information and exit                              |
| `--help`                    | `-h`   | `false`                                       | Print command line help                                         |

### `agent-server`

| Flag                      | Short  | Default                                       | Description                                        |
| :------------------------ | :----- | :-------------------------------------------- | :------------------------------------------------- |
| `--addr <addr>`           | `-a`   | `:4502`                                       | HTTP listen address                                |
| `--db <path>`             | (none) | `~/.local/share/agent-status/agent-status.db` | Path to agent-status SQLite database (read-only)   |
| `--socket <path>`         | (none) | `/tmp/agent-status/notify.sock`               | Path to notify Unix domain socket                  |
| `--cors-origin <origins>` | (none) | `http://localhost:3000`                       | Allowed CORS origins (comma-separated or repeated) |
| `--token <token>`         | (none) | `""`                                          | Bearer authentication token                        |
| `--log-level <level>`     | (none) | `info`                                        | Log level (`debug`, `info`, `warn`, `error`)       |
| `--version`               | `-v`   | `false`                                       | Print version information and exit                 |
| `--help`                  | `-h`   | `false`                                       | Print command line help                            |

### `agent-debug` / `just debug run`

| Flag                   | Short  | Default | Description                                                          |
| :--------------------- | :----- | :------ | :------------------------------------------------------------------- |
| `--harness <name>`     | (none) | `""`    | Explicit harness override (`claude-code`, `pi`, `codex`, `opencode`) |
| `--session-id <id>`    | (none) | `""`    | OpenCode session ID to replay                                        |
| `--cwd <path>`         | (none) | `""`    | Override transcript working directory for local Git resolution       |
| `--diff`               | (none) | `false` | Display turn-by-turn state mutations during replay                   |
| `--until-line <N>`     | (none) | `0`     | Halt replay after processing source line N                           |
| `--until-event <type>` | (none) | `""`    | Halt replay after encountering event type                            |
| `--stop-on-error`      | (none) | `false` | Halt immediately on first parse or state machine error               |
| `--json`               | (none) | `false` | Output final state and verification results as JSON                  |
| `--export-db <path>`   | (none) | `""`    | Path to save populated SQLite database on disk                       |
| `--summarize`          | (none) | `false` | Enable live LLM summarizer provider verification                     |
| `--env <label>`        | (none) | `debug` | Environment label override                                           |
| `--version`            | `-v`   | `false` | Print version information and exit                                   |
| `--help`               | `-h`   | `false` | Print command line help                                              |

### Environment Variables

The table below lists environment variables read by the system, in precedence and resolution order:

| Variable                         | Precedence / Default                             | Description                                                                                   |
| :------------------------------- | :----------------------------------------------- | :-------------------------------------------------------------------------------------------- |
| `LITELLM_BASE_URL`               | Fallback: `OPENAI_BASE_URL`                      | Base URL for OpenAI-compatible or LiteLLM proxy endpoint                                      |
| `OPENAI_BASE_URL`                | (none)                                           | Fallback endpoint URL when `LITELLM_BASE_URL` is unset                                        |
| `LITELLM_API_KEY`                | Alias: `LITELLM_KEY`, fallback: `OPENAI_API_KEY` | Primary API authentication key for LiteLLM or OpenAI-compatible proxy                         |
| `LITELLM_KEY`                    | Fallback: `OPENAI_API_KEY`                       | Alias for `LITELLM_API_KEY`                                                                   |
| `OPENAI_API_KEY`                 | (none)                                           | Fallback API authentication key when LiteLLM keys are unset                                   |
| `GEMINI_API_KEY`                 | (none)                                           | API key for Google Gemini action summarization provider                                       |
| `ANTHROPIC_API_KEY`              | (none)                                           | API key for Anthropic Claude action summarization provider                                    |
| `AGENT_STATUS_SUMMARIZER_MODEL`  | Provider default                                 | Custom model identifier override for action summarization                                     |
| `AGENT_STATUS_NO_SUMMARIZE`      | `0`                                              | Set to `1` to disable LLM action summarization and use deterministic fallback heuristics      |
| `AGENT_STATUS_IDLE_GRACE_PERIOD` | `5s`                                             | Grace period to debounce BUSY -> IDLE transitions after turn end                              |
| `AGENT_STATUS_INGESTER_ADDR`     | `:4501`                                          | Listen address for `agent-ingester` WebSocket server                                          |
| `AGENT_STATUS_INGESTER_URL`      | `http://127.0.0.1:4501`                          | HTTP URL of `agent-ingester` service for `agent-server` mutation proxying                     |
| `AGENT_STATUS_INGESTER_WS_URL`   | `ws://127.0.0.1:4501/ws/ingest`                  | WebSocket ingestion endpoint URL of `agent-ingester` for `agent-observer` telemetry streaming |
| `AGENT_STATUS_BEARER_TOKEN`      | `dev-secret-token`                               | Shared bearer token for telemetry ingestion authentication                                    |
| `AGENT_STATUS_TOKEN`             | (none)                                           | Shared bearer authentication token used by `agent-observer` and `agent-server`                |
| `AGENT_STATUS_LOG_DIR`           | `~/.local/state/agent-status/logs`               | Directory path for rotating daemon disk logs                                                  |
| `AGENT_STATUS_LOG_FILE`          | (none)                                           | Explicit file path override for daemon disk logging                                           |

# License

MIT License (c) 2026 Alex Gorbatchev
