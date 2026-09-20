# agent-observer

Local observer daemon (`air-observer`) that scans harness sessions, tails transcripts incrementally, and ships telemetry over WebSocket.

## Commands
- Dev: `go run ./cmd/air-observer`
- Build: `just build`
- Test: `just test`
- Lint: `just lint`

## Local conventions
- Scanner routines live in `internal/scanner/` and git workspace resolution in `internal/git/`.
- File tailing persists byte offsets in `$XDG_CACHE_HOME/ai-registry/tailer-cursors.json` via `internal/tailer/cursor.go`.

## Local gotchas
- Claude Code session compactions rewrite transcript files atomically; tailer detects inode change or file truncation and resets cursor accordingly (`internal/tailer/tailer.go`).

## Boundaries
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Ask first: modifying cursor storage structure or WebSocket wire protocol
- Never: lock, mutate, or delete active agent session log files
- Never: publish releases automatically without explicit user authorization

## References
- Entry point: `cmd/air-observer/main.go`
- Shipper logic: `internal/shipper/shipper.go`
- Scanner implementations: `internal/scanner/`
