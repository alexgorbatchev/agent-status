# print-session-cli

Standalone CLI utility (`print-session`) that uses `agent-parser` to print agent session logs by session ID or path.

## Commands

- Build: `just build`
- Test: `just test`
- Lint: `just lint`
- Run: `just run claude print <session-id>`
- Run (AI mode): `just run-ai claude print <session-id>`

## Usage Examples

- Continuation context for resuming work (~200-400 tokens):
  `print-session claude handoff <session-id>` (or `continue`, `resume`)
  `print-session pi handoff <session-id>`
- High-level session summary (~150-300 tokens):
  `print-session claude summary <session-id>`
  `print-session pi summary <session-id>`
- Print only the last conversational turn:
  `print-session claude print --last-turn <session-id>`
  `print-session pi print --last-turn <session-id>`
- Print session by session ID or shorthand:
  `print-session claude <session-id>`
  `print-session pi <session-id>`
- Print only errors and failed tool calls:
  `print-session claude print --errors <session-id>`
  `print-session pi print --errors <session-id>`
- Print only modified files:
  `print-session claude print --files <session-id>`
  `print-session pi print --files <session-id>`
- Print only tool calls and results:
  `print-session claude print --tools <session-id>`
  `print-session pi print --tools <session-id>`
- Print only user prompts and turns:
  `print-session claude print --prompts <session-id>`
  `print-session pi print --prompts <session-id>`
- Tail the last N events:
  `print-session claude print --tail 10 <session-id>`
  `print-session pi print --tail 10 <session-id>`
- Print session as JSON:
  `print-session claude print --json <session-id>`
  `print-session pi print --json <session-id>`
- Print session from direct file path:
  `print-session claude print /path/to/<session-id>.jsonl`
  `print-session pi print /path/to/<session-id>.jsonl`

## Local conventions

- Follow subject-first CLI hierarchy (`print-session [claude|pi] [print|summary|handoff] <session-id>`).
- Render hierarchical tree-view help screens with `cobra-help-tree`.
- Dual-mode output: formatted readable transcript blocks when `AGENT=0`/unset, compact token-conservative lines when `AGENT=1`.
- Human mode (`AGENT=0`): 4-space indent (`    `) by default for all conversational values (`[USER]`, `[ASSISTANT]`, `[ERROR]`), dynamically wrapped to the active terminal width (`golang.org/x/term`) with continuation indents preserved. Tool results are nested under tool calls (`[TOOL: ...]` -> `    [RESULT]` -> `        <output>`) to provide a clean tree structure.
- Agent mode (`AGENT=1`): untruncated narrative conversation without micro-telemetry (no tokens count, no model IDs, no microsecond timestamps).
- Strictly no emojis in either human or agent output.
- Consume `agent-parser` directly as an external Go library.

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Always: run `just test` and `just lint` before committing changes
- Never: publish releases automatically without explicit user authorization
- Never: parse or transmit credentials or authentication tokens
