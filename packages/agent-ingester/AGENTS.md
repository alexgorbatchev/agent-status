# agent-ingester

Central telemetry ingestion daemon (`air-ingester`) that receives agent events, computes token pricing, manages SQLite storage, and broadcasts updates.

## Commands
- Dev: `just run`
- Build: `just build`
- Test: `just test`
- Lint: `just lint`

## Local conventions
- SQLite migrations are defined sequentially in `internal/store/migrations.go`.
- Real-time updates broadcast over Unix domain socket `$XDG_RUNTIME_DIR/agent-status/notify.sock` via `internal/notify/publisher.go`.
- WebSocket receiver configures a 64 MiB read limit (`conn.SetReadLimit(64 * 1024 * 1024)`) on observer connections to handle oversized frame batches, diffs, and bash outputs.
- Telemetry and session logging resolves human environment and project names for clean human-facing logs while preserving machine IDs under `AGENT=1`.
- Pricing calculations use static rates defined in `internal/pricing/pricing.go`.
- Action summarization runs asynchronously via per-agent serialized queues to avoid blocking WebSocket ingestion.
- Summaries use specialized prompt templates (Thinking + Tool, Thinking only, Tool only) with sanitized tool arguments (max 300 chars).
- Semantic deduplication rejects state updates with >= 60% word overlap (Jaccard similarity) when target and phase are unchanged.

## Local gotchas
- Unlisted models report `nil` cost rather than `$0.00` to distinguish free usage from unknown rates.
- Retention cleanup runs every hour to prune events older than 48 hours and exited agents older than 30 days (`internal/store/prune.go`).

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
