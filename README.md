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

- `air-observer` tails JSONL logs and SQLite databases without holding file locks, persisting byte read offsets in `$XDG_CACHE_HOME/ai-registry/tailer-cursors.json` so restarted observers resume without duplicate telemetry.
- `air-ingester` receives WebSocket connections at `:4501/ws/ingest`, computes token costs using a static model price registry, derives concise action descriptions, and writes structured records into `$XDG_DATA_HOME/agent-status/agent-status.db` using WAL mode.
- `air-server` opens the SQLite database strictly in read-only mode, listens for change signals on `$XDG_RUNTIME_DIR/agent-status/notify.sock`, and fans out events to browser clients over Server-Sent Events (`/api/v1/events/stream`) at `:4502`.
- `ui-web` serves a React 19 dashboard on port `3000`, grouping agents by upstream git project repository and observing environment rather than machine-local folder paths.
- Inactive agents and old event streams are automatically pruned every hour from SQLite according to a 48-hour event and 30-day exited session retention policy.
- Running with `AGENT=1` enables token-conservative, agent-facing CLI output across dev stack orchestration.

# Prerequisites

- [Bun](https://bun.sh) (v1.2+) - Required for running the web server, bundling frontend assets, and orchestrating the dev stack.
- [Go](https://go.dev) (v1.24+) - Required for compiling `air-observer`, `air-ingester`, and `air-server` daemons.
- [Just](https://github.com/casey/just) - Command runner used for build, test, and lifecycle recipes.
- [Git](https://git-scm.com) - Used by observer to resolve project identity, branches, and linked worktrees.

# Installation

Download the prebuilt binary archive for your platform from the [latest release](https://github.com/alexgorbatchev/agent-status/releases/latest), replacing `X.X.X` with the version shown on that page.

```bash
# macOS (Apple Silicon)
curl -sSL https://github.com/alexgorbatchev/agent-status/releases/latest/download/agent-status_X.X.X_darwin_arm64.tar.gz | tar -xz -C ~/.local/bin
```

# Quick Start

```bash
# Start the full agent-status stack (ingester, server, observer, web UI)
just start

# Check status of running processes
just status

# Stop the stack and release ports
just stop
```

Sample Output:
```
🛑 Stopping agent-status stack...
✅ Stack stopped.
🚀 Starting agent-status stack (ingester, server, observer, ui)...
📦 Building Go binaries...
   [1/4] Starting air-ingester (:4501)...
   [2/4] Starting air-server (:4502)...
   [3/4] Starting air-observer...
   [4/4] Starting ui-web (http://localhost:3000)...

✨ agent-status is running at http://localhost:3000
   Press Ctrl+C to stop or run: just stop
```

# Options & Flags

### `just start` / `scripts/dev-stack.ts`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--debug` | `-d` | `false` | Enable debug logging across all stack services |
| `--log-level <level>` | (none) | `info` | Log level for services (`debug`, `info`, `warn`, `error`) |

### `air-observer`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--ingester-url <url>` | (none) | `ws://127.0.0.1:4501/ws/ingest` | Ingester WebSocket endpoint URL |
| `--token <token>` | (none) | `""` | Bearer token for ingester authentication |
| `--env <label>` | (none) | `""` | Environment label override |
| `--scan-interval <duration>` | (none) | `2s` | Interval between harness session scans |
| `--cursor-path <path>` | (none) | `~/.cache/ai-registry/tailer-cursors.json` | Path to persistent tailer cursor file |
| `--claude-sessions-dir <path>` | (none) | `~/.claude/sessions` | Custom directory for Claude Code session metadata |
| `--claude-projects-dir <path>` | (none) | `~/.claude/projects` | Custom directory for Claude Code project transcripts |
| `--pi-sessions-dir <path>` | (none) | `~/.pi/agent/sessions` | Custom directory for Pi session logs |
| `--log-level <level>` | (none) | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `--version` | `-V` | `false` | Print version information and exit |
| `--help` | `-h` | `false` | Print command line help |

### `air-ingester`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--addr <addr>` | (none) | `:4501` | HTTP and WebSocket listen address |
| `--token <token>` | (none) | `dev-secret-token` | Bearer authentication token |
| `--db <path>` | (none) | `~/.local/share/agent-status/agent-status.db` | SQLite database file path |
| `--notify-sock <path>` | (none) | `/tmp/agent-status/notify.sock` | Unix domain socket path for event notifications |
| `--no-summarize` | (none) | `false` | Disable LLM-based action summarization |
| `--litellm-url <url>` | (none) | `""` | Base URL for OpenAI-compatible or LiteLLM proxy |
| `--litellm-key <key>` | (none) | `""` | API key for OpenAI-compatible or LiteLLM proxy |
| `--summarizer-model <name>` | (none) | `""` | Model identifier used for action summarization |
| `--log-level <level>` | (none) | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `--version` | `-V` | `false` | Print version information and exit |
| `--help` | `-h` | `false` | Print command line help |

### `air-server`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--addr <addr>` | `-a` | `:4502` | HTTP listen address |
| `--db <path>` | (none) | `~/.local/share/agent-status/agent-status.db` | Path to agent-status SQLite database (read-only) |
| `--socket <path>` | (none) | `/tmp/agent-status/notify.sock` | Path to notify Unix domain socket |
| `--cors-origin <origins>` | (none) | `http://localhost:3000` | Allowed CORS origins (comma-separated or repeated) |
| `--token <token>` | (none) | `""` | Bearer authentication token |
| `--log-level <level>` | (none) | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `--version` | `-V` | `false` | Print version information and exit |
| `--help` | `-h` | `false` | Print command line help |

# License

MIT License (c) 2026 Alex Gorbatchev
