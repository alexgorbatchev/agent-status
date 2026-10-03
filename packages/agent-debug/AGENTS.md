# agent-debug

Offline replay and validation CLI tool for agent session transcripts through the ingestion pipeline (`air-debug`).

## Commands

- Build: `just build`
- Test: `just test` (enforces >= 90% statement coverage gate via `scripts/check-coverage.sh`)
- Check coverage: `just check-coverage` (repo root task checking all Go packages)
- Lint: `just lint`
- Run: `just run session replay <file>`
- Run (AI mode): `just run-ai session replay <file>`

## Usage Examples

- Replay session transcript: `air-debug session replay ~/.claude/sessions/session-abc.jsonl`
- Replay with turn-by-turn state diffs: `air-debug session replay --diff session-abc.jsonl`
- Replay OpenCode database session: `air-debug session replay --session-id ses_123 opencode.db`
- List sessions in OpenCode database: `air-debug session list opencode.db`
- Validate session offline: `air-debug session validate session-abc.jsonl`
- View terminal UI card representation: `air-debug session view session-abc.jsonl`
- View hierarchical fleet tree: `air-debug session fleet` (or `air-debug fleet`)
- View fleet for transcripts or DB: `air-debug session fleet session1.jsonl session2.jsonl`
- Foreign path remapping: `air-debug session replay --cwd /local/repo session-abc.jsonl`
- Export populated SQLite DB: `air-debug session replay --export-db .tmp/debug.db session-abc.jsonl`
- Inspect persisted SQLite state: `air-debug db inspect <session-id>`
- Audit process discovery & pairing: `air-debug scanner audit` (or `air-debug process list`)

## Local conventions

- Follow subject-first CLI hierarchy (`air-debug session <verb>`).
- Render hierarchical tree-view help screens with `cobra-help-tree`.
- Dual-mode output: formatted tables and trees when `AGENT=0`, compact token-conservative key-value lines when `AGENT=1`.
- Act strictly as an external driver; zero debug flags or conditional forks in production daemons.
- Scanner audit imports `github.com/alexgorbatchev/agent-watcher/scanner` directly; public git resolution remains in `agent-observer/git`.
- Offline session replay disables the ingestion classifier idle grace period (`SetIdleGracePeriod(0)`) to evaluate event sequences without simulated wall-clock debounce delays.
- Terminal width and alignment rely on `runewidth` cell calculations via `VisualWidth` (`runewidth.StringWidth(StripANSI(s))`) and rune-safe truncation via `TruncateVisual` to account for wide Unicode characters and zero-width ANSI escape sequences.
- Default runewidth condition is pinned via `runewidth.DefaultCondition.EastAsianWidth = false` to guarantee deterministic single-cell width across all environments and locales.
- Tables enforce ASCII tablewriter configuration (`tw.StyleASCII`, `tw.Off` EastAsian), and tree structures use ASCII-only structural glyphs (`v`, `|-`, `+-`) to avoid ambiguous width misalignment.
- Redundant project prefixes in session titles are stripped using `TrimProjectPrefix` (`internal/output/format.go`) strictly when displayed under project headers or tree views in a project context (e.g. `RenderFleetViewAt`); single-card views preserve full titles. Matching requires exact case and whitespace-bounded separators (colons, slashes, hyphens, brackets) sharing test vectors with `packages/agent-common/testdata/project-prefix-fixtures.json`.
- Semantic color precedence strictly reserves Red (Error), Green (Success / Finished / Completed), Yellow (Warning / Waiting on Question / Rate Limited), and Blue (Busy / Working) for lifecycle and activity states; brand colors are sanitized via `SanitizeBrandColor` to prevent visual collision with error, prompt, or busy states.
- Five-state activity model (`BUSY`, `WAITING_QUESTION`, `ERROR`, `RATE_LIMITED`, `IDLE`) and `Completed` status are rendered across card and fleet views, mapping `RATE_LIMITED` to warning styling and unacknowledged completions to emerald success styling.
- Harness vendor brand styling assigns distinct styles per harness (`AnsiMagenta` for Claude Code, `AnsiBold + AnsiMagenta` for Pi, `AnsiCyan` for Codex, `AnsiBold + AnsiCyan` for OpenCode) without colliding with semantic colors.
- CLI color gating: ANSI color output is gated using `go-isatty` to verify terminal file descriptors and strictly respects the `NO_COLOR` environment variable by wrapping output writers with an ANSI-stripping writer (`wrapWriter`, `isColorEnabled`).
- Idle freshness tiers categorize idle sessions as `fresh` (<15m), `standard` (15m–2h), or `stale` (>2h, rendered with `AnsiDim` in card and fleet views unless an unacknowledged completion is pending; idle sessions in fleet views are stably partitioned into warm idle before stale dormant idle while preserving relative input order).
- Hierarchical worktree nesting in `RenderFleetView` groups sessions under project families with linked worktree branches (`internal/output/view.go`).
- All transcript-derived fields (titles, actions, errors, harness labels, questions, branches, project names) must be passed through `SanitizeTranscriptText` before formatting to strip ANSI escape sequences and control characters.

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% statement coverage is required and enforced on `just test` via `scripts/check-coverage.sh` (scripts/ folder is excluded from this rule)
- Always: automatically record all new user instructions in the most appropriate `AGENTS.md` file immediately upon receipt (check with user if existing instructions conflict)
- Always: run `just test` and `just lint` before committing changes
- Ask first: modifying output formats, CLI command hierarchies, or shared terminal styling contracts
- Never: publish releases automatically without explicit user authorization
- Never: parse or transmit credentials or authentication tokens

## References

- CLI entrypoint: `cmd/air-debug/main.go`
- Replay runner: `internal/replay/runner.go`
- Read verification: `internal/replay/verify.go`
- Formatting & dual-mode: `internal/output/format.go`
- Terminal card view: `internal/output/view.go`
