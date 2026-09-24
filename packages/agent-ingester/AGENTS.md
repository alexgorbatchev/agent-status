# agent-ingester

Central telemetry ingestion daemon (`air-ingester`) that receives agent events, computes token pricing, manages SQLite storage, and broadcasts updates.

## Commands
- Dev: `just run`
- Build: `just build`
- Test: `just test`
- Lint: `just lint`

## Local conventions
- SQLite migrations are defined sequentially in `internal/store/migrations.go`; migration v6 updates the SQLite `activity` check constraint to include `'RATE_LIMITED'` alongside `'BUSY'`, `'WAITING_QUESTION'`, `'ERROR'`, and `'IDLE'`.
- Hidden agent state is stored in SQLite (`is_hidden`, `hidden_at`); incoming telemetry frames with new activity automatically reset `is_hidden = 0`.
- Real-time updates broadcast over Unix domain socket `$XDG_RUNTIME_DIR/agent-status/notify.sock` via `internal/notify/publisher.go`.
- Turn-end debouncing uses `DefaultIdleGracePeriod = 2500ms` (`DefaultTurnEndGraceWindow`) to smooth short gaps between tool results and subsequent thinking turns before declaring idle.
- Turn completion retention sets `HasUnseenCompletion = true` and `CompletedAt` timestamp when a turn ends without error or pending question, retaining completion state until a new user turn or explicit dismissal.
- Activity classification precedence elevates `RATE_LIMITED` above standard idle/busy states: a root or subagent rate limit takes precedence over idle status, rolling up subagent rate-limit errors (e.g. `[Subagent <id>] rate limit exceeded`).
- WebSocket receiver configures a 64 MiB read limit (`conn.SetReadLimit(64 * 1024 * 1024)`) on observer connections to handle oversized frame batches, diffs, and bash outputs.
- Telemetry and session logging resolves human environment and project names for clean human-facing logs while preserving machine IDs under `AGENT=1`.
- Pricing calculations use static rates defined in `internal/pricing/pricing.go`.
- Action summarization runs asynchronously via per-agent serialized queues to avoid blocking WebSocket ingestion.
- Summaries use specialized prompt templates (Thinking + Tool, Thinking only, Tool only) with sanitized tool arguments (max 300 chars).
- Semantic deduplication rejects state updates with >= 60% word overlap (Jaccard similarity) when target and phase are unchanged.
- Herdr terminal window activation (`activateTerminalWindow`) uses thread-safe `herdr.MockExecCommand` for test mocking to prevent OS window focus-stealing on macOS.

## Local gotchas
- Unlisted models report `nil` cost rather than `$0.00` to distinguish free usage from unknown rates.
- Retention cleanup runs every hour to prune events older than 48 hours and exited agents older than 30 days (`internal/store/prune.go`).
- Subagent frames must not cancel the root idle timer; idle-timer validity is keyed by `agentContext.rootGeneration`, not `UpdatedAt` (`internal/classifier/classifier.go`, #64).
- `current_run_id` in SQLite is non-NULL only for unended runs, and an exited agent reloads with `CurrentRun == nil` (`internal/store/store.go`, `internal/classifier/classifier.go`, #69, #73).
- Root activity context (`actx.rootActivity`) is strictly isolated from subagent composite/rolled-up activity so a busy subagent cannot pin the root to BUSY after subagent completion (`internal/classifier/classifier.go`, #97).
- Minimum visibility dwell smoothing preserves completed short-lived subagents in `ActiveSubagents` for a configurable dwell threshold (`DefaultSubagentDwell = 30s`, `SetSubagentDwell`), tracking `StartedAt`, `CompletedAt`, and `Status` ("running" vs "completed") before eviction (`internal/classifier/classifier.go`, #98).
- Active state floors preserve BUSY during long tool runs and model inference: Thinking Floor (TurnFloor) latches BUSY across inference latency windows, In-Flight Floor (FlightFloor) latches BUSY while a registered tool awaits `tool_result`, and silence timers distinguish open in-flight tools from finished turns with a 30-minute turn silence timeout (`internal/classifier/classifier.go`, #100).
- Failed subagent `tool_result` frames (`IsError = true`) keep the subagent in `BUSY` status (matching root agent behavior) rather than escalating to `ERROR` or `RATE_LIMITED`; only genuine subagent `error` frames escalate activity, and error outputs are truncated strictly on UTF-8 rune boundaries (`internal/classifier/classifier.go`, #119).

## Boundaries
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Ask first: adding database schema migrations or changing activity classification logic
- Never: block WebSocket ingestion handlers on synchronous LLM summarizer calls
- Never: publish releases automatically without explicit user authorization

## References
- Entry point: `cmd/air-ingester/main.go`
- Store & migrations: `internal/store/`
- Classifier: `internal/classifier/classifier.go`
- Summarizer: `internal/summarizer/summarizer.go`
- Pricing engine: `internal/pricing/pricing.go`
