# agent-observer

Local observer daemon (`air-observer`) that scans harness sessions, tails transcripts incrementally, and ships telemetry over WebSocket.

## Commands

- Dev: `go run ./cmd/air-observer`
- Build: `just build`
- Test: `just test` (enforces >= 90% statement coverage gate via `scripts/check-coverage.sh`)
- Check coverage: `just check-coverage` (repo root task checking all Go packages)
- Lint: `just lint`

## Local conventions

- Scanner routines live in `internal/scanner/` and git workspace resolution in `internal/git/`.
- Transcript activity timestamps are derived using `ExtractLastActiveTimestamp` (`internal/scanner/tail_timestamp.go`), which reads backwards in chunks (256KB) from the end of transcript files across oversized records, filtering only to conversational/turn-generating records via an explicit allowlist (`user`, `assistant`, `message`, `event_msg`, `response_item`, `turn_start`, `turn_end`, etc.) while excluding non-conversational records and start times, falling back to file modification time (`ModTime()`) when no valid conversational timestamp is found; the recovered timestamp feeds into session recency for Claude Code, Pi, and Codex so dormant sessions are not shown as active while preserved file modification times allow resumed PID matching. `ExtractFirstActiveTimestamp` reads the first timestamped line from transcript headers to derive subagent start timestamps for `run_started`.
- Pi scanner (`internal/scanner/pi.go`) and Codex scanner (`internal/scanner/codex.go`) maintain an in-memory cache of session file and rollout metadata (`size`, `modTime`, `header`, `lastActive`, `dev`, `ino`) across scan ticks, reusing cached headers and timestamps for unchanged files and pruning entries for deleted, vanished, or unseen sessions after each scan.
- File tailing commits persistent byte offsets in memory via `internal/tailer/cursor.go` upon acknowledgement from `air-ingester` (cursor-committed delivery) and flushes them to disk at a bounded rate via a background flusher (debounced periodic flush, default 2s) and on daemon shutdown, avoiding synchronous atomic file rewrites on every committed line.
- Session tracker (`internal/scanner/session_tracker.go`) writes state to disk only when tracked sessions change (`dirty` flag), eliminating redundant file writes on unchanged scan ticks.
- Hot-path persistence files (cursor store and session tracker) serialize using compact `json.Marshal` rather than indented JSON.
- Cursor store prunes entries for deleted files and entries older than the retention window on daemon startup and periodically via background pruner.
- Shipper implements bounded dispatch with tailer backpressure (`NewTailerWithOffsetHandler`) and sequence-keyed ACK callbacks; reads pause when disconnected or buffer is full.
- Shipper resets exponential backoff only upon receiving an ACK from the ingester or after 5s of sustained error-free streaming, rather than immediately on the hello frame.
- Tailers share directory fsnotify watches via a reference-counted `WatcherRegistry` (`internal/tailer/watcher.go`), dispatching file-change events selectively to the owning tailer to avoid descriptor leaks and O(N^2) wakeup stampedes in multi-subagent sessions.
- Process enumeration is captured at most once per scan tick using a shared `ProcessSnapshot` (`internal/scanner/process_snapshot.go`) storing lightweight scalar metadata (`PID`, `Name`, `CreateTime`, `Alive`) rather than full process handles; on Darwin, bulk kernel querying uses `unix.SysctlKinfoProcSlice("kern.proc.all")` in a single syscall, cheap name filtering excludes non-agent processes before cmdline/cwd inspection, and Claude Code probes candidate PIDs (`unix.Kill(pid, 0)`) to skip snapshot enumeration entirely when all on-disk sessions belong to dead processes. Process finding is skipped in scanners when there are no candidate session files to pair.
- Structured logging uses `packages/agent-common/logging` with snake_case actions and dual-mode (`AGENT=1`) output.

## Local gotchas

- Claude Code session compactions rewrite transcript files atomically; tailer detects inode change or file truncation and resets cursor accordingly (`internal/tailer/tailer.go`).
- Daemon shutdown guarantees persistent cursor serialization to disk via deferred `cursorStore.Save()` in `runObserverContext`.
- When the ingester is disconnected, tailers pause reading from disk once the bounded shipper buffer fills up, preserving transcript files as the durable queue.

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% statement coverage is required and enforced on `just test` via `scripts/check-coverage.sh` (scripts/ folder is excluded from this rule)
- Ask first: modifying cursor storage structure or WebSocket wire protocol
- Never: lock, mutate, or delete active agent session log files
- Never: publish releases automatically without explicit user authorization

## References

- Entry point: `cmd/air-observer/main.go`
- Shipper logic: `internal/shipper/shipper.go`
- Scanner implementations: `internal/scanner/`
