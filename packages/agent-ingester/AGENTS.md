# agent-ingester

Central telemetry ingestion daemon (`air-ingester`) that receives agent events, computes token pricing, manages SQLite storage, and broadcasts updates.

## Commands
- Dev: `just run`
- Build: `just build`
- Test: `just test` (enforces >= 90% statement coverage gate via `scripts/check-coverage.sh`)
- Check coverage: `just check-coverage` (repo root task checking all Go packages)
- Lint: `just lint`

## Local conventions
- SQLite schema migrations are defined sequentially in `internal/store/migrations.go`. Table rebuild migrations must pin a single dedicated connection via `db.Conn(ctx)` to disable foreign keys, run the transaction, verify `PRAGMA foreign_key_check`, and re-enable foreign keys, preventing accidental cascade deletion of related tables across pooled connections.
- Structured rate-limit metadata (`RateLimitResetAt`, `RetryAfterSeconds`) extracted from telemetry frames is persisted to the `agents` table and attached to `IAgentActivity`, elevating activity to `RATE_LIMITED`; rate-limit reset fields are cleared whenever the resulting activity is not `RATE_LIMITED` (`internal/classifier/classifier.go`, `internal/store/store.go`).
- Silence timers are scheduled on rate-limit error frames (`scheduleSilenceTimerLocked`) so that unrecovered or abandoned throttled sessions transition to `IDLE` after the silence timeout window instead of stalling in `RATE_LIMITED` indefinitely (`internal/classifier/classifier.go`).
- Hidden agent state is stored in SQLite (`is_hidden`, `hidden_at`); incoming telemetry frames with new activity automatically reset `is_hidden = 0`.
- Real-time updates broadcast over Unix domain socket `$XDG_RUNTIME_DIR/agent-status/notify.sock` via `internal/notify/publisher.go`.
- Turn-end debouncing uses `DefaultIdleGracePeriod = 5s` (configurable via `--idle-grace-period` CLI flag or `AIR_IDLE_GRACE_PERIOD` environment variable) to smooth short gaps between tool results and subsequent thinking turns before declaring idle; replay disables the idle grace period (`SetIdleGracePeriod(0)`) to evaluate event sequences without simulated wall-clock debounce delays.
- Turn completion retention sets `HasUnseenCompletion = true` and `CompletedAt` timestamp when a turn ends without error or pending question, retaining completion state until a new user turn or explicit dismissal.
- Turn completion dismissal write path: `air-ingester` exposes `PATCH /api/v1/internal/agents/{id}` (supporting unified patch payloads with `dismissCompletion`/`acknowledgeCompletion` alongside `isHidden`), resetting `has_unseen_completion = 0` and clearing `completed_at = NULL` in SQLite via `DismissAgentCompletion`, clearing in-memory status in the classifier, and broadcasting `agent_upsert` over notify.sock.
- Activity classification precedence elevates `RATE_LIMITED` above standard idle/busy states: a root or subagent rate limit takes precedence over idle status, rolling up subagent rate-limit errors (e.g. `[Subagent <id>] rate limit exceeded`).
- WebSocket receiver configures a 64 MiB read limit (`conn.SetReadLimit(64 * 1024 * 1024)`) on observer connections to handle oversized frame batches, diffs, and bash outputs.
- Telemetry and session logging resolves human environment and project names for clean human-facing logs while preserving machine IDs under `AGENT=1`.
- Pricing calculations use static rates defined in `internal/pricing/pricing.go`.
- Action summarization runs asynchronously via per-agent serialized queues to avoid blocking WebSocket ingestion; when an agent is in `ActivityRateLimited`, external LLM summarization is bypassed to preserve the deterministic action (`"Rate limit reached — backoff/retry in progress"`) and `phase = nil`.
- Summaries use specialized prompt templates (Thinking + Tool, Thinking only, Tool only) with sanitized tool arguments (max 300 chars).
- Semantic deduplication rejects state updates with >= 60% word overlap (Jaccard similarity) when target and phase are unchanged.
- Herdr terminal window activation (`activateTerminalWindow`) uses thread-safe `herdr.MockExecCommand` for test mocking to prevent OS window focus-stealing on macOS.

## Local gotchas
- Unlisted models report `nil` cost rather than `$0.00` to distinguish free usage from unknown rates.
- Retention cleanup runs every hour to prune events older than 48 hours and exited agents older than 30 days (`internal/store/prune.go`).
- Subagent frames must not cancel the root idle timer; idle-timer validity is keyed by `agentContext.rootGeneration`, not `UpdatedAt` (`internal/classifier/classifier.go`).
- `current_run_id` in SQLite is non-NULL only for unended runs, and an exited agent reloads with `CurrentRun == nil` (`internal/store/store.go`, `internal/classifier/classifier.go`).
- Root activity context (`actx.rootActivity`) is strictly isolated from subagent composite/rolled-up activity so a busy subagent cannot pin the root to BUSY after subagent completion (`internal/classifier/classifier.go`).
- Minimum visibility dwell smoothing preserves completed short-lived subagents in `ActiveSubagents` for a configurable dwell threshold (`DefaultSubagentDwell = 30s`, `SetSubagentDwell`), tracking `StartedAt`, `CompletedAt`, and `Status` ("running" vs "completed") before eviction (`internal/classifier/classifier.go`).
- Active state floors preserve BUSY during long tool runs and model inference: Thinking Floor (TurnFloor) latches BUSY across inference latency windows, In-Flight Floor (FlightFloor) latches BUSY while a registered tool awaits `tool_result`, and silence timers distinguish open in-flight tools from finished turns with a 30-minute turn silence timeout (`internal/classifier/classifier.go`).
- Failed subagent `tool_result` frames (`IsError = true`) keep the subagent in `BUSY` status (matching root agent behavior) rather than escalating to `ERROR` or `RATE_LIMITED`; only genuine subagent `error` frames escalate activity, and error outputs are truncated strictly on UTF-8 rune boundaries (`internal/classifier/classifier.go`).
- Per-agent persistence locking, save generation counters, and ordering guarantees: to prevent concurrent background writers (idle transitions, silence transitions, subagent dwell expirations, and async summary jobs) from overwriting newer frame states (root or subagent), each agent maintains a `persistMu` mutex and a `saveGeneration uint64` incremented on every incoming frame in `ProcessFrame`. Idle and silence transitions must acquire `persistMu` and re-verify both `rootGeneration` and `saveGeneration` under lock before applying in-memory mutations (`actx.rootActivity = IDLE`, timer cancellations, floor resets) or saving to SQLite, and must release `persistMu` immediately after persistence before job enqueuing or update callbacks. Similarly, subagent dwell expiration and async summary workers must acquire `persistMu`, verify `saveGeneration` under lock, and re-read the fresh agent row under `persistMu` before saving, releasing `persistMu` immediately after persistence before invoking callbacks or hooks (`internal/classifier/classifier.go`).
- Turn-end grace period: `turn_end` events schedule `handleIdleTransition` via `DefaultIdleGracePeriod = 5s` (configurable via `--idle-grace-period` / `AIR_IDLE_GRACE_PERIOD`). This debounces the transition to IDLE across multi-turn agent execution while attention states (`WAITING_QUESTION`, `ERROR`) bypass the grace period immediately. Out-of-scope conversational events (`text`, `agent_message`) do not alter classifier activity states.
- Subagent dwell timer synchronization and testing: `afterSubagentDwellHook` provides deterministic synchronization in tests when subagent dwell timers fire and persist updated state; in `handleSubagentDwellExpired`, timer expirations arriving slightly earlier than `startedAt + dwellMs` due to clock jitter (<= 1ms difference) proceed with eviction, while larger differences reschedule the timer rather than dropping it silently, and async test frames anchor timestamps (`fSubEnd.Timestamp = fSubStart.Timestamp + 1`) to prevent scheduling pauses between test statements from exceeding dwell thresholds before turn completion frames are processed (`internal/classifier/classifier.go`, `internal/classifier/classifier_test.go`).

## Boundaries
- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% statement coverage is required and enforced on `just test` via `scripts/check-coverage.sh` (scripts/ folder is excluded from this rule)
- Always: automatically record all new user instructions in the most appropriate `AGENTS.md` file immediately upon receipt (check with user if existing instructions conflict)
- Ask first: adding database schema migrations or changing activity classification logic
- Never: block WebSocket ingestion handlers on synchronous LLM summarizer calls
- Never: publish releases automatically without explicit user authorization

## References
- Entry point: `cmd/air-ingester/main.go`
- Store & migrations: `internal/store/`
- Classifier: `internal/classifier/classifier.go`
- Summarizer: `internal/summarizer/summarizer.go`
- Pricing engine: `internal/pricing/pricing.go`
