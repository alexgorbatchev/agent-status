# agent-observer

Local observer daemon (`air-observer`) that consumes `agent-watcher`, formats telemetry, resolves git workspaces, and ships events over WebSocket.

## Commands

- Dev: `go run ./cmd/air-observer`
- Build: `just build`
- Test: `just test` (>=90% statement coverage via `scripts/check-coverage.sh`)
- Lint: `just lint`

## Local conventions

- Keep discovery, lifecycle, transcript/database watching, and parser-event emission in standalone `github.com/alexgorbatchev/agent-watcher`.
- `cmd/air-observer/main.go` maps `watcher.Event` to telemetry. Environment identity, git workspace resolution (`internal/git/`), and WebSocket delivery (`internal/shipper/`) belong here.
- Pass explicit harness paths to `watcher.Config`; do not change process environment variables to configure the library.
- Preserve agent-status cache locations using `config.ObserverCachePath`: `observer-cursors.json` and `observer-sessions.json` under `$XDG_CACHE_HOME/agent-status/` or `~/.cache/agent-status/`.
- OpenCode messages use read-only database access and a separate acknowledged checkpoint file at the resolved cursor path + `.opencode`.
- Pass the shipper's sequence-keyed ingester acknowledgement to the watcher. Transcript cursors advance only through contiguous acknowledged lines; only the last event from a line carries its acknowledgement.
- Watcher pruning removes cursors for deleted or replaced transcripts; cursors for existing files with matching device and inode survive age pruning.
- Shipper bounded dispatch supplies backpressure: reads pause when disconnected or the buffer is full. Backoff resets only on an ACK or after 5s of sustained error-free streaming.
- Structured logging uses `packages/agent-common/logging`, including its logger injected into the watcher, with snake_case actions and dual-mode (`AGENT=1`) output.

## Local gotchas

- Watcher shutdown joins tailers and flushes cursors, database checkpoints, and session state before shipper shutdown.
- Transcript files remain the durable queue when the bounded shipper buffer fills up.
- Claude Code compaction replaces or truncates transcripts; file generation checks belong to the library.
- Observer and debug consume a tagged GitHub version of the watcher. Keep sibling workspace overrides optional and out of committed manifests.

## Boundaries

- Always: keep the full reusable watching pipeline in `agent-watcher`; agent-status owns formatting and transport.
- Always: preserve incoming fixes and their behavioral tests in `agent-watcher` when resolving conflicts in code extracted from observer.
- Always: include OpenCode message watching, with acknowledged database checkpoints separate from transcript byte cursors.
- Always: functional code changes require behavioral test changes, red/green verification, and >=90% statement coverage per package (scripts excluded).
- Always: automatically record new user instructions in the appropriate `AGENTS.md`, checking conflicts with the user.
- Ask first: modifying cursor storage structure or WebSocket wire protocol.
- Never: lock, mutate, or delete active harness logs or databases.
- Never: publish releases automatically without explicit authorization.

## References

- Entry point and telemetry adapter: `cmd/air-observer/main.go`
- Shipper: `internal/shipper/shipper.go`
- Watcher API and implementation: `github.com/alexgorbatchev/agent-watcher`
- Public git resolver used by debug: `git/export.go`
