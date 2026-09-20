# agent-common

Shared Go types and structured logging definitions for the agent-status pipeline.

## Commands
- Test: `just test`
- Lint: `just lint`

## Local conventions
- Keep types in `types/types.go` synchronized with frontend models in `packages/ui-web/src/types/types.ts`.
- Structured logging wraps `log/slog` via `logging/logging.go`.

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
