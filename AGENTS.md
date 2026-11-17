# agent-status

Centralized monitoring dashboard and telemetry aggregation pipeline for AI coding agents across machines and worktrees.

Designed for monitoring long-running agent sessions across minutes and hours, not for sub-second twitch debugging. Semantic clarity, high-level LLM task summarization (1–3s inference latency is expected and intended), and stall detection supersede micro-second telemetry mirroring.

## Shared commands

- Install: `bun install --cwd packages/ui-web`
- Dev routing setup: `direnv allow`, then `devhost caddy start` (follow the devhost skill for first-machine certificate and privileged-port setup)
- Start dev stack: `just start`
- Stop dev stack: `just stop`
- Check status: `just status`
- Tail dev stack logs: `just logs` (or `tail -f .tmp/dev-stack/logs/*.log`)
- Run debug CLI: `just debug <args>`
- Replay session offline: `just session-replay-[claude|pi|codex|opencode] <session-id>`
- Print session transcript: `just print-session [claude|pi] [print|summary|handoff] <session-id>`
- Build all: `just build-all`
- Test all: `just test-all`
- Check coverage: `just check-coverage` (enforces >= 90% statement coverage across all Go packages via `scripts/check-coverage.sh`)
- Lint all: `just lint-all`
- Run Playwright E2E: `just test-e2e`
- Start Storybook: `just storybook-web`

## Workspace map

- `packages/agent-common/` -> `packages/agent-common/AGENTS.md`
- `packages/agent-observer/` -> `packages/agent-observer/AGENTS.md`
- `packages/agent-ingester/` -> `packages/agent-ingester/AGENTS.md`
- `packages/agent-server/` -> `packages/agent-server/AGENTS.md`
- `packages/agent-debug/` -> `packages/agent-debug/AGENTS.md`
- `packages/ui-web/` -> `packages/ui-web/AGENTS.md`

## Shared gotchas

- Watcher dependency -> observer/debug consume a tagged GitHub version of `agent-watcher`. Builds, tests, and lint must work without a sibling checkout; local workspace overrides are optional development choices.
- Dev stack ports -> devhost assigns automatic loopback ports and owns stack shutdown. `just stop` stops this manifest only; `just status` lists all running devhost stacks. Never kill arbitrary port listeners.
- Dev stack execution -> `just start` builds the stack, then hands supervision to foreground `devhost start`; use `just --no-deps start` to reuse binaries. Pass service settings through `AGENT_DEV_LOG_LEVEL` and `AGENT_NO_SUMMARIZE`; `just start --debug` shows Caddy output.
- SQLite database concurrency -> `agent-server` opens SQLite strictly in read-only mode (`?mode=ro`); all mutations occur through `agent-ingester`.
- Unix domain socket binding -> `agent-ingester` creates `$XDG_RUNTIME_DIR/agent-status/notify.sock`; `agent-server` reconnects automatically when the socket becomes available.
- Daemon disk logs -> Installed daemons write `~/.local/state/agent-status/logs/{observer,ingester,server}.log` (or `$AGENT_LOG_DIR`); the dev stack writes `.tmp/dev-stack/logs/`. Active logs rotate at 10 MB with 5 backups, 14-day retention, and gzip compression. Tail dev stack logs with `just logs`.
- Dev stack file layout -> `scripts/dev-service.sh` keeps service output under `.tmp/dev-stack/`: `data/` (SQLite), `cache/` (observer cursors and session state), `logs/`, and `run/` (`XDG_RUNTIME_DIR`, notify socket). Preserve the observer's inherited `XDG_DATA_HOME` for harness discovery and clear inherited `AGENT_LOG_FILE` / `AGENT_SESSION_STATE_PATH` redirects. Deep checkout paths fail fast because Unix socket paths are capped at 103 bytes.
- Dev routing -> `devhost.toml` routes the dashboard and `/api/*` through the same hostname. Bun inlines only the `AGENT_PUBLIC_*` allowlist, including `AGENT_PUBLIC_API_URL`; never inline the complete service environment. Bun HTML/HMR uses `proxyLocalOrigin = true` on the UI route.

## Shared boundaries

- Always: name project executables `agent-debug`, `agent-observer`, `agent-ingester`, and `agent-server` to match their packages; use the `AGENT_` prefix for project configuration variables. Keep the observer WebSocket endpoint (`AGENT_INGESTER_WS_URL`) distinct from the server HTTP mutation endpoint (`AGENT_INGESTER_HTTP_URL`).
- Always: automatically record all new instructions in the most appropriate `AGENTS.md` file immediately upon receipt (check with user if existing instructions conflict)
- Always: keep `agent-watcher` published on GitHub and consume its tagged module version here; do not commit sibling-checkout requirements.
- Always: when resolving GitHub issues, follow the `github-fix-issues` skill workflow (located in `alexgorbatchev/ai-registry` under `skills/agent/github-fix-issues` for coordinator lifecycle, worktree isolation, subagent delegation, and verification gates)
- Always: design dashboard UX, activity summarization, and telemetry pipelines for long-running agent sessions rather than sub-second realtime mirroring (LLM inference latency of 1–3s and ~10s debounced summarization cadences are primary and intended; prioritize semantic stability, high-level intent, and stall detection over micro-second state churn)
- Always: when designing or modifying UI presentation cards, update `agent-debug session view` first as the canonical reference specification, and ensure frontend implementations sync to that output (#50, #51)
- Always: use structured logging via `packages/agent-common/logging` for all diagnostics, including command failures and logger setup warnings, with concise snake_case actions (e.g. `action=hello_received`, no prose, no emojis) and support dual-mode output via `AGENT=1` (prefer human names over opaque IDs in human mode). Report a command failure once on stderr; reserve stdout for results. HTTP client failures must identify the attempted method and endpoint without exposing bearer credentials or server-supplied response bodies.
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% statement coverage is required and enforced on `just test` and `just check-coverage` via `scripts/check-coverage.sh` (scripts/ folder is excluded from this rule)
- Always: run `just test-all` and `just lint-all` before committing changes
- Ask first: adding database migrations in `packages/agent-ingester/internal/store/migrations.go` or modifying shared types in `packages/agent-common/types/types.go`
- Never: run the full dev stack (`just start` or `devhost start`) in a synchronous tool call; it runs in the foreground and never exits
- Always: invoke devhost directly from the lifecycle recipes for dev-stack service supervision and read the dashboard hostname from `AGENT_DEV_HOST` in `.envrc`.
- Never: let agent-status dev services write runtime files outside `.tmp/dev-stack/`; keep databases, caches, logs, and sockets local. The shared devhost/Caddy proxy uses its machine-level state and lifecycle independently of this stack.
- Never: publish releases automatically without explicit user authorization
- Never: parse or transmit credentials, API keys, or session auth tokens from agent harnesses
- Never: write directly to agent harness storage directories or mutate active harness logs

## References

- Architecture & concepts: `CONTEXT.md`
- Dev stack manifest: `devhost.toml`; lifecycle recipes: `justfile`; service environment: `scripts/dev-service.sh`
- Root task runner: `justfile`
- Canonical harness log specs & parser: [`alexgorbatchev/agent-parser`](https://github.com/alexgorbatchev/agent-parser) (`docs/harness-logs/`)
- Reusable watching pipeline: [`alexgorbatchev/agent-watcher`](https://github.com/alexgorbatchev/agent-watcher) (`watcher.Run`, `scanner/`, `tailer/`)
- Standalone session transcript printer: [`alexgorbatchev/agent-print-session-cli`](https://github.com/alexgorbatchev/agent-print-session-cli) (`v1.0.0`)
- GitHub issue fixing skill: [`github-fix-issues`](https://github.com/alexgorbatchev/ai-registry/tree/main/skills/agent/github-fix-issues) (`skills/agent/github-fix-issues` in `alexgorbatchev/ai-registry`)
