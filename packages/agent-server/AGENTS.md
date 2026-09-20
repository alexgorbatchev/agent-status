# agent-server

Central REST API and Server-Sent Events (SSE) daemon (`air-server`) providing query endpoints and live event streams.

## Commands
- Dev: `just run`
- Build: `just build`
- Test: `just test`
- Lint: `just lint`

## Local conventions
- SQLite store is strictly read-only (`internal/store/queries.go`).
- Live updates subscribe to `$XDG_RUNTIME_DIR/agent-status/notify.sock` and fan out to connected SSE clients via `internal/stream/hub.go`.

## Local gotchas
- The socket subscriber must handle missing sockets or ingester restarts with reconnect backoff (`internal/stream/subscriber.go`).

## Boundaries
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Ask first: modifying REST routes under `/api/v1/*` or changing SSE payload format
- Never: open the SQLite database in read-write mode from `air-server`
- Never: publish releases automatically without explicit user authorization

## References
- Entry point: `cmd/air-server/main.go`
- HTTP API handlers: `internal/api/handlers.go`
- Stream subscriber & hub: `internal/stream/`
