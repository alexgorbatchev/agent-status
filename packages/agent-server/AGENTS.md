# agent-server

Central REST API and Server-Sent Events (SSE) daemon (`air-server`) providing query endpoints and live event streams.

## Commands

- Dev: `just run`
- Build: `just build`
- Test: `just test`
- Lint: `just lint`

## Local conventions

- SQLite store is strictly read-only (`internal/store/queries.go`).
- Agent mutations (visibility toggling via `isHidden` and turn completion dismissal via `dismissCompletion` or `acknowledgeCompletion`) via `PATCH /api/v1/agents/{id}` forward composite or individual payloads directly to `air-ingester` (`PATCH /api/v1/internal/agents/{id}`) to preserve read-only SQLite access.
- Live updates subscribe to `$XDG_RUNTIME_DIR/agent-status/notify.sock` and fan out to connected SSE clients via `internal/stream/hub.go`.
- Socket subscriber executes `reconcileSnapshot` (`internal/stream/subscriber.go`) on initial connection and upon every reconnection to rebuild and broadcast a full database snapshot, preventing lost frames or stale state across ingester restarts.
- Hub evicts slow global subscribers when buffer is full on snapshot broadcast (`action=subscriber_evicted`) to force client reconnect and prevent permanent desynchronization (#118).

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
