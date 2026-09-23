# agent-debug

Offline replay and validation CLI tool for agent session transcripts through the ingestion pipeline (`air-debug`).

## Commands

- Build: `just build`
- Test: `just test`
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
- Foreign path remapping: `air-debug session replay --cwd /local/repo session-abc.jsonl`
- Export populated SQLite DB: `air-debug session replay --export-db .tmp/debug.db session-abc.jsonl`
- Inspect persisted SQLite state: `air-debug db inspect <session-id>`
- Audit process discovery & pairing: `air-debug scanner audit` (or `air-debug process list`)

## Local conventions

- Follow subject-first CLI hierarchy (`air-debug session <verb>`).
- Render hierarchical tree-view help screens with `cobra-help-tree`.
- Dual-mode output: formatted tables and trees when `AGENT=0`, compact token-conservative key-value lines when `AGENT=1`.
- Act strictly as an external driver; zero debug flags or conditional forks in production daemons.

## Boundaries

- Always (code-based projects only): any time code is changed such that results from running that code are changed, a test file must be changed as well; 90% code coverage is required (scripts/ folder is excluded from this rule)
- Always: run `just test` and `just lint` before committing changes
- Never: publish releases automatically without explicit user authorization
- Never: parse or transmit credentials or authentication tokens

## References

- CLI entrypoint: `cmd/air-debug/main.go`
- Replay runner: `internal/replay/runner.go`
- Read verification: `internal/replay/verify.go`
- Formatting & dual-mode: `internal/output/format.go`
- Terminal card view: `internal/output/view.go`
