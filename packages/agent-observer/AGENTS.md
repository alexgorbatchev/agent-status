# agent-observer

Local observer daemon (`air-observer`) that scans harness sessions, tails transcripts incrementally, and ships telemetry over WebSocket.

## Commands

- Dev: `go run ./cmd/air-observer`
- Build: `just build`
- Test: `just test`
- Lint: `just lint`

## Local conventions

- Scanner routines live in `internal/scanner/` and git workspace resolution in `internal/git/`.
- Transcript activity timestamps are derived using `ExtractLastActiveTimestamp` (`internal/scanner/tail_timestamp.go`), which reads the tail of transcript files (up to 256KB) scanning backwards for valid JSON lines carrying recognizable timestamps, falling back to file modification time (`ModTime()`) when no timestamp is found.
- File tailing commits persistent byte offsets via `internal/tailer/cursor.go` only after frame batches are acknowledged by `air-ingester` (cursor-committed delivery).
- Shipper implements bounded dispatch with tailer backpressure (`NewTailerWithOffsetHandler`) and sequence-keyed ACK callbacks; reads pause when disconnected or buffer is full.
- Shipper resets exponential backoff only upon receiving an ACK from the ingester or after 5s of sustained error-free streaming, rather than immediately on the hello frame.
- Structured logging uses `packages/agent-common/logging` with snake_case actions and dual-mode (`AGENT=1`) output.

## Local gotchas

- Claude Code session compactions rewrite transcript files atomically; tailer detects inode change or file truncation and resets cursor accordingly (`internal/tailer/tailer.go`).
- Daemon shutdown guarantees persistent cursor serialization to disk via deferred `cursorStore.Save()` in `runObserverContext`.
- When the ingester is disconnected, tailers pause reading from disk once the bounded shipper buffer fills up, preserving transcript files as the durable queue.

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Ask first: modifying cursor storage structure or WebSocket wire protocol
- Never: lock, mutate, or delete active agent session log files
- Never: publish releases automatically without explicit user authorization

## References

- Entry point: `cmd/air-observer/main.go`
- Shipper logic: `internal/shipper/shipper.go`
- Scanner implementations: `internal/scanner/`
