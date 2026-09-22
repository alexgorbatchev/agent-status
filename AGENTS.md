# agent-status

Centralized monitoring dashboard and telemetry aggregation pipeline for AI coding agents across machines and worktrees.

Designed for monitoring long-running agent sessions across minutes and hours, not for sub-second twitch debugging. Semantic clarity, high-level LLM task summarization (1–3s inference latency is expected and intended), and stall detection supersede micro-second telemetry mirroring.

## Shared commands
- Install: `bun install --cwd packages/ui-web`
- Start dev stack: `just start`
- Stop dev stack: `just stop`
- Check status: `just status`
- Tail daemon logs: `just logs` (or `tail -f ~/.local/state/agent-status/logs/*.log`)
- Run debug CLI: `just debug <args>`
- Replay session offline: `just session-replay-[claude|pi|codex|opencode] <session-id>`
- Build all: `just build-all`
- Test all: `just test-all`
- Lint all: `just lint-all`
- Run Playwright E2E: `just test-e2e`
- Start Storybook: `just storybook-web`

## Workspace map
- `packages/agent-common/` -> `packages/agent-common/AGENTS.md`
- `packages/agent-parser/` -> `packages/agent-parser/AGENTS.md`
- `packages/agent-observer/` -> `packages/agent-observer/AGENTS.md`
- `packages/agent-ingester/` -> `packages/agent-ingester/AGENTS.md`
- `packages/agent-server/` -> `packages/agent-server/AGENTS.md`
- `packages/agent-debug/` -> `packages/agent-debug/AGENTS.md`
- `packages/ui-web/` -> `packages/ui-web/AGENTS.md`
- `docs/harness-logs/` -> `docs/harness-logs/AGENTS.md`

## Shared gotchas
- Port collisions on 3000, 4501, or 4502 -> Run `just stop` to terminate orphaned processes and clean up `.tmp/dev-stack/pids.json`.
- Dev stack execution -> Running `just start` directly in a synchronous tool call blocks indefinitely because `scripts/dev-stack.ts start` streams logs in the foreground and never exits; use background process management or let the user run it.
- SQLite database concurrency -> `air-server` opens SQLite strictly in read-only mode (`?mode=ro`); all mutations occur through `air-ingester`.
- Unix domain socket binding -> `air-ingester` creates `$XDG_RUNTIME_DIR/agent-status/notify.sock`; `air-server` reconnects automatically when the socket becomes available.
- Daemon disk logs -> Stored at `~/.local/state/agent-status/logs/{observer,ingester,server}.log` (or `$AIR_LOG_DIR`). Active logs rotate at 10 MB with 5 backups, 14-day retention, and gzip compression. Tail with `just logs` or view directly on disk.

## Shared boundaries
- Always: automatically record all new instructions in the most appropriate `AGENTS.md` file immediately upon receipt (check with user if existing instructions conflict)
- Always: when working on GitHub issue remediation, check for newly added or pending open issues (`gh issue list --state open`) before yielding the turn to the user
- Always: design dashboard UX, activity summarization, and telemetry pipelines for long-running agent sessions rather than sub-second realtime mirroring (LLM inference latency of 1–3s and ~10s debounced summarization cadences are primary and intended; prioritize semantic stability, high-level intent, and stall detection over micro-second state churn)
- Always: when designing or modifying UI presentation cards, update `air-debug session view` first as the canonical reference specification, and ensure frontend implementations sync to that output (#50, #51)
- Always: use structured logging via `packages/agent-common/logging` with concise snake_case actions (e.g. `action=hello_received`, no prose, no emojis) and support dual-mode output via `AGENT=1` (prefer human names over opaque IDs in human mode)
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Always: run `just test-all` and `just lint-all` before committing changes
- Ask first: adding database migrations in `packages/agent-ingester/internal/store/migrations.go` or modifying shared types in `packages/agent-common/types/types.go`
- Never: run the full dev stack (`just start` or `scripts/dev-stack.ts start`) in a synchronous tool call; it runs in the foreground and never exits
- Never: publish releases automatically without explicit user authorization
- Never: parse or transmit credentials, API keys, or session auth tokens from agent harnesses
- Never: write directly to agent harness storage directories or mutate active harness logs

## References
- Architecture & concepts: `CONTEXT.md`
- Dev stack orchestrator: `scripts/dev-stack.ts`
- Root task runner: `justfile`
