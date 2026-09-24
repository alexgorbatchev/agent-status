# agent-common

Shared Go types and structured logging definitions for the agent-status pipeline.

## Commands

- Test: `just test`
- Lint: `just lint`

## Local conventions

- Keep types in `types/types.go` synchronized with frontend models in `packages/ui-web/src/types/types.ts`.
- `ActivityRateLimited` (`"RATE_LIMITED"`) models rate-limiting state on `IAgentActivity` alongside `RateLimitResetAt` (epoch ms reset timestamp) and `RetryAfterSeconds` (retry delay), with snake_case JSON unmarshaling compatibility.
- Turn completion retention uses `HasUnseenCompletion` (boolean pointer) and `CompletedAt` (epoch ms timestamp) on `IAgentActivity` (and `CompletedAt` on `ISubagentSummary`) to flag unacknowledged completed turns that transitioned to idle.
- Structured logging wraps `log/slog` via `logging/logging.go` and respects dual-mode output via `AGENT=1` (human mode formats clean tags and prefers human names over opaque IDs; agent mode produces token-conservative machine logfmt), with disk rotation and retention limits via lumberjack.

## Local gotchas

- Renaming struct tags in `types/types.go` breaks JSON contracts between `air-observer`, `air-ingester`, `air-server`, and `ui-web`.

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Ask first: adding new lifecycle or activity enum values in `types/types.go`
- Never: import internal packages from other `packages/*` modules into `agent-common`
- Never: publish releases automatically without explicit user authorization

## References

- Domain types: `types/types.go`
- Shared logging: `logging/logging.go`
