# agent-parser

Pure Go parsing routines for extracting structured events from Claude Code, Pi, OpenAI Codex, and OpenCode harness logs.

## Commands

- Test: `just test`
- Lint: `just lint`

## Local conventions

- Match parser behavior against canonical schema specifications in `docs/harness-logs/*.ts`.
- Go parser types and unmarshaling structs must be kept strictly in sync with the canonical TypeScript definitions in `docs/harness-logs/*.ts`.
- Parsers must be pure functions without filesystem I/O, network requests, or global state.
- Pi logs emit error events (`EventType: "error"`, `IsError: true`) when assistant `stopReason` is `"error"`, extracting error text from `errorMessage` (or text content fallback) (`pi.go`). Aborted turns (`stopReason: "aborted"`) indicate cancellation and do not emit error events.
- OpenCode tool execution failures emit `tool_result` frames with `IsError: true`, and `EventType: "error"` is emitted on assistant message termination failures (`finish: "error"` or non-aborted `msg.Data.Error`), extracting tool error diagnostic details when available (`opencode.go`). In-flight retry parts (`type: "retry"`) indicating rate limiting (status 429) emit error frames with `IsRateLimit: true`, `RetryAfterSeconds`, and `RateLimitResetAt` so active throttling is observed, while non-rate-limit retry parts do not emit error events if the assistant message finishes cleanly (e.g. `finish: "stop"` or nil). HTTP status codes are never assigned to `ExitCode` (which is strictly for process exit codes 0-255). User aborts (`MessageAbortedError`) are treated as non-errors and do not emit error events.
- Codex parsers emit error events (`EventType: "error"`, `IsError: true`) on error event messages, extracting rate limit and status info from `codex_error_info` (`RateLimitExceeded`, `UsageLimitExceeded`, `ServerOverloaded`, `HttpConnectionFailed`) and HTTP status 429 without loose substring matching on error text or assigning HTTP status codes to `ExitCode` (`codex.go`). Stateful `CodexParser` instances persist parsed rate limit reset and retry duration metadata from `token_count` and discount remaining retry seconds when attaching cached reset targets to subsequent rate limit `error` events (`remSec := (*p.rateLimitResetAt - ts + 999) / 1000`), while non-429 `HttpConnectionFailed` status codes explicitly set `IsRateLimit = false`.
- Claude Code parsers extract HTTP 429 status codes from api_error_status / apiErrorStatus and emit error frames with `IsRateLimit = true` without loose substring matching on error text and without assigning HTTP status codes to `ExitCode` (`claudecode.go`).

## Local gotchas

- Pi logs are DAG trees linked via `parentId`; parse branch histories rather than assuming flat linear streams (`pi.go`).
- Codex tool arguments are double-JSON-encoded strings requiring a second decode step (`codex.go`).
- OpenCode stores events in SQLite WAL tables rather than JSONL files (`opencode.go`).
- Claude Code writes each content block of an API response as a separate JSONL line sharing `message.id`, `requestId`, `stop_reason`, and identical `usage`; usage must be counted once per request (`claudecode.go`, #67, #76, #81).
- Claude Code `last-prompt` records are pointer records without timestamps and must never produce `turn_start` (`claudecode.go`, #65).

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Ask first: modifying parsed event representations or adding support for new harnesses
- Never: introduce side effects, file I/O, or OS calls inside parser functions
- Never: publish releases automatically without explicit user authorization

## References

- Harness log specs: `docs/harness-logs/`
- Claude parser: `claudecode.go`
- Pi parser: `pi.go`
- Codex parser: `codex.go`
- OpenCode parser: `opencode.go`
