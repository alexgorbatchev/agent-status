Agent Status tracks coding-agent sessions and usage in a web dashboard and provides offline tools for inspecting recorded sessions. Its daemons and diagnostic CLI are written in Rust.

# What It Does

- **Observe sessions:** Follow Claude Code, Pi, Codex, OpenCode, and Antigravity CLI activity and deliver acknowledged telemetry.
- **Ingest activity:** Commit telemetry, current state, usage, and delivery checkpoints together.
- **Serve queries:** Provide read-only database queries and live streams to the web dashboard, forwarding explicit mutations to the ingester.
- **Diagnose locally:** Replay transcripts, inspect stored records, and audit native session discovery.

# How It Works

1. Run an ingester to own the database and local notification socket.
2. Run observers on machines hosting coding agents.
3. Run the server against the ingester's database and socket for dashboard clients.
4. Use Agent Debug independently for local replay and inspection.

# How it Really Works

The six Cargo packages share one workspace lockfile. The dashboard uses Bun and React. The ingester owns persistent writes while the server opens its database read-only. Observers retain unacknowledged deliveries across reconnects. Debug normally replays in memory and writes only when explicitly exporting a database. Provider summarization is configurable and can incur charges.

| Package | Role | Documentation |
| :--- | :--- | :--- |
| `agent-common` | Shared telemetry, usage, models, logging | [API and configuration](packages/agent-common/README.md) |
| `agent-status-store` | Transactional SQLite schema and queries | [Storage API](packages/agent-status-store/README.md) |
| `agent-observer` | Local discovery and acknowledged delivery | [Observer guide](packages/agent-observer/README.md) |
| `agent-ingester` | Durable ingestion and activity classification | [Ingester guide](packages/agent-ingester/README.md) |
| `agent-server` | Queries, streams, mutation forwarding | [Server guide](packages/agent-server/README.md) |
| `agent-debug` | Offline replay and diagnostics | [Debug guide](packages/agent-debug/README.md) |
| `ui-web` | Live fleet, activity, and log dashboard | [Frontend guide](packages/ui-web/AGENTS.md) |

Cargo pins the external `agent-parser`, `agent-watcher`, and `cli-command-kit` Git revisions. The dashboard and its tests live in [`packages/ui-web`](packages/ui-web/).

# Installation

Build an executable with `just debug build`, `just observer build`, `just ingester build`, or `just server build`. Each recipe copies its optimized executable into that package's `bin/` directory. The Cargo packages declare their required Rust versions. Install dashboard dependencies with `just ui setup` and build the bundle with `just ui build`.

# Quick Start

```sh
just --list --list-submodules
just debug run --version
just debug run-ai session replay session.jsonl
just test
```

With [devhost](https://github.com/alexgorbatchev/devhost) configured, `just start` runs the dashboard and daemons behind local HTTPS. Use `just status` to locate the current checkout's stack and `just stop` to stop it.

Run `just <module> check` for a Rust package's strict formatting, Clippy, tests, and coverage gate, or `just check` for the workspace. `just ui test-e2e` builds the native daemons and exercises the dashboard in Chromium.

# Operational References

The CLI `skill` commands print installation pointers. The corresponding usage documents reside at repository-root paths:

- [Agent Debug](skills/agent-debug/SKILL.md)
- [Agent Observer](skills/agent-observer/SKILL.md)
- [Agent Ingester](skills/agent-ingester/SKILL.md)
- [Agent Server](skills/agent-server/SKILL.md)

Read the selected package's guide and skill before configuring live services.

# License

[MIT](LICENSE), copyright 2026 Alex Gorbatchev.
