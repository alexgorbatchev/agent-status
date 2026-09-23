# agent-parser

Pure Go parsing routines for extracting structured events from Claude Code, Pi, OpenAI Codex, and OpenCode harness logs.

## Commands

- Test: `just test`
- Lint: `just lint`

## Local conventions

- Match parser behavior against canonical schema specifications in `docs/harness-logs/*.ts`.
- Go parser types and unmarshaling structs must be kept strictly in sync with the canonical TypeScript definitions in `docs/harness-logs/*.ts`.
- Parsers must be pure functions without filesystem I/O, network requests, or global state.

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
