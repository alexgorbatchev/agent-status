# agent-status

Centralized monitoring dashboard and telemetry aggregation pipeline for AI coding agents across machines and worktrees.

Designed for monitoring long-running agent sessions across minutes and hours, not for sub-second twitch debugging. Semantic clarity, high-level LLM task summarization (1–3s inference latency is expected and intended), and stall detection supersede micro-second telemetry mirroring.

## Shared commands

- Install: `bun install --cwd packages/ui-web`
- Start dev stack: `just run` (or `just start`; native devhost services)
- Stop dev stack: `just stop`
- Check status: `just status` (this checkout's devhost stack; `devhost stack list` shows all stacks)
- Tail dev stack logs: `just logs` (or `tail -f .tmp/dev-stack/logs/*.log`)
- Run debug CLI: `just debug run <args>`
- Replay session offline: `just debug session-replay-[claude|pi|codex|opencode] <session-id>`
- Print session transcript: `just debug print-session [claude|pi] [print|summary|handoff] <session-id>`
- Build all: `just build-all`
- Test all: `just test-all`
- Check coverage: `just check-coverage` (enforces >= 90% statement coverage across all Go packages via `scripts/check-coverage.sh`)
- Lint all: `just lint-all` (Go formatting via `scripts/check-format.sh`, which checks tracked and new source and skips ignored output)
- Run Playwright E2E: `just ui test-e2e` (builds the daemons first; run `bun x playwright install chromium` in `packages/ui-web` once)
- Start Storybook: `just ui storybook`

## Workspace map

- `packages/agent-common/` -> `packages/agent-common/AGENTS.md`
- `packages/agent-observer/` -> `packages/agent-observer/AGENTS.md`
- `packages/agent-ingester/` -> `packages/agent-ingester/AGENTS.md`
- `packages/agent-server/` -> `packages/agent-server/AGENTS.md`
- `packages/agent-debug/` -> `packages/agent-debug/AGENTS.md`
- `packages/ui-web/` -> `packages/ui-web/AGENTS.md`

## Shared gotchas

- Watcher dependency -> observer/debug consume a tagged GitHub version of `agent-watcher`. Builds, tests, and lint must work without a sibling checkout; local workspace overrides are optional development choices.
- Dev stack ports -> devhost allocates loopback ports automatically and routes UI and `/api/*` through the same HTTPS hostname.
- Dev stack execution -> `just run` starts the shared devhost proxy and runs devhost in the foreground; use background process management in tools.
- SQLite database concurrency -> `agent-server` opens SQLite strictly in read-only mode (`?mode=ro`); all mutations occur through `agent-ingester`.
- SQLite driver -> every module uses `modernc.org/sqlite` (driver name `sqlite`), the same pure-Go engine agent-parser and agent-watcher use. Do not add another SQLite driver.
- Unix domain socket binding -> `agent-ingester` creates `$XDG_RUNTIME_DIR/agent-status/notify.sock`; `agent-server` reconnects automatically when the socket becomes available.
- Daemon disk logs -> Installed daemons write `~/.local/state/agent-status/logs/{observer,ingester,server}.log` (or `$AGENT_STATUS_LOG_DIR`); the dev stack writes `.tmp/dev-stack/logs/`. Active logs rotate at 10 MB with 5 backups, 14-day retention, and gzip compression. Tail dev stack logs with `just logs`.
- Dev stack file layout -> package `dev` recipes keep daemon data, cache, logs, and the notify socket under `.tmp/dev-stack/`. Daemons run from the repository root with relative socket `.tmp/dev-stack/run/notify.sock`, avoiding Unix socket address limits in deep checkouts. Devhost owns supervisor and shared Caddy state in its standard locations.
- Devhost routing -> `devhost.toml` reads `DEVHOST_DEV_EXT` inherited from the shell. Direnv automatically loads `.envrc`; invoke devhost directly in recipes without `direnv exec` wrappers. `.envrc` defaults to `.localhost`; ignored `.envrc.local` supplies machine-specific suffixes. Keep private hostnames out of tracked files.

## Shared boundaries

- Always: route `just run` through native devhost services; do not introduce a second stack supervisor.
- Always: keep package-specific commands in each package's lowercase `justfile`; register packages with root `mod` declarations and keep the root justfile focused on workspace orchestration.
- Always: automatically record all new instructions in the most appropriate `AGENTS.md` file immediately upon receipt (check with user if existing instructions conflict)
- Always: keep `agent-watcher` published on GitHub and consume its tagged module version here; do not commit sibling-checkout requirements.
- Always: when resolving tickets, follow the `github-issues` skill workflow (`skills/agent/github-issues` in the private AI registry overlay) for claims, worktree isolation, independent PR review, CI gates, and fast-forward integration
- Always: design dashboard UX, activity summarization, and telemetry pipelines for long-running agent sessions rather than sub-second realtime mirroring (LLM inference latency of 1–3s and ~10s debounced summarization cadences are primary and intended; prioritize semantic stability, high-level intent, and stall detection over micro-second state churn)
- Always: when designing or modifying UI presentation cards, update `agent-debug session view` first as the canonical reference specification, and ensure frontend implementations sync to that output (#50, #51)
- Always: use structured logging via `packages/agent-common/logging` with concise snake_case actions (e.g. `action=hello_received`, no prose, no emojis) and support dual-mode output via `AGENT=1` (prefer human names over opaque IDs in human mode)
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% statement coverage is required and enforced on `just test` and `just check-coverage` via `scripts/check-coverage.sh` (scripts/ folder is excluded from this rule)
- Always: run `just test-all` and `just lint-all` before committing changes
- Ask first: adding database migrations in `packages/agent-ingester/internal/store/migrations.go` or modifying shared types in `packages/agent-common/types/types.go`
- Never: run `just run` or `just start` in a synchronous tool call; devhost runs in the foreground.
- Never: let dev stack daemons write application data outside the repository; databases, caches, logs, and sockets belong under `.tmp/dev-stack/`. Devhost manages its own shared proxy and supervisor state.
- Never: publish releases automatically without explicit user authorization
- Never: parse or transmit credentials, API keys, or session auth tokens from agent harnesses
- Never: write directly to agent harness storage directories or mutate active harness logs

## References

- Architecture & concepts: `CONTEXT.md`
- Dev stack manifest: `devhost.toml`
- Root task runner: `justfile`
- Canonical harness log specs & parser: [`alexgorbatchev/agent-parser`](https://github.com/alexgorbatchev/agent-parser) (`docs/harness-logs/`)
- Reusable watching pipeline: [`alexgorbatchev/agent-watcher`](https://github.com/alexgorbatchev/agent-watcher) (`watcher.Run`, `scanner/`, `tailer/`)
- Standalone session transcript printer: [`alexgorbatchev/agent-print-session-cli`](https://github.com/alexgorbatchev/agent-print-session-cli) (`v1.0.0`)
- CI workflow: `.github/workflows/ci.yml` runs `just lint-all`, `just test-all`, `just build-all`, and the Playwright E2E suite (`just ui test-e2e`, inside Playwright's container image matching the pinned `@playwright/test`) on hosted Ubuntu for pull requests and pushes to `main`
- Ticket workflow skill: `github-issues` (`skills/agent/github-issues` in the private AI registry overlay)
