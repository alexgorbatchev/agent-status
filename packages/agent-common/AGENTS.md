# agent-common

Shared Go types and structured logging definitions for the agent-status pipeline.

## Commands

- Test: `just test` (enforces >= 90% statement coverage gate via `scripts/check-coverage.sh`)
- Check coverage: `just check-coverage` (repo root task checking all Go packages)
- Lint: `just lint`

## Local conventions

- Keep types in `types/types.go` synchronized with frontend models in `packages/ui-web/src/types/types.ts`.
- `ActivityRateLimited` (`"RATE_LIMITED"`) models rate-limiting state on `IAgentActivity` alongside `RateLimitResetAt` (epoch ms reset timestamp) and `RetryAfterSeconds` (retry delay).
- Turn completion retention uses `HasUnseenCompletion` (boolean pointer) and `CompletedAt` (epoch ms timestamp) on `IAgentActivity` (and `CompletedAt` on `ISubagentSummary`) to flag unacknowledged completed turns that transitioned to idle.
- Structured logging wraps `log/slog` via `logging/logging.go` and respects dual-mode output via `AGENT=1` (human mode formats clean tags and prefers human names over opaque IDs; agent mode produces token-conservative machine logfmt), with disk rotation and retention limits via lumberjack.

## Local gotchas

- Renaming struct tags in `types/types.go` breaks JSON contracts between `agent-observer`, `agent-ingester`, `agent-server`, and `ui-web`.
- Tests must never read real harness sessions or write real state. Command packages call `testenv.IsolateStateDirs` and, when they discover or resolve sessions, `testenv.IsolateHarnessEnv` from `TestMain`, and a test in that package calls `testenv.RequireIsolated` and `testenv.RequireHarnessIsolated` so removing the isolation fails the package. Tests that create sessions under HOME use `testenv.IsolateHarnessHome(t)`. Add new harness location variables to `harnessEnv` in `testenv/harness.go`.

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% statement coverage is required and enforced on `just test` via `scripts/check-coverage.sh` (scripts/ folder is excluded from this rule)
- Ask first: adding new lifecycle or activity enum values in `types/types.go`
- Never: import internal packages from other `packages/*` modules into `agent-common`
- Never: publish releases automatically without explicit user authorization

## References

- Domain types: `types/types.go`
- Shared logging: `logging/logging.go`
- Test isolation: `testenv/testenv.go`, `testenv/harness.go`
