---
name: session-debug
description: >-
  REQUIRED when diagnosing, replaying, validating, or troubleshooting agent session transcripts
  (`.jsonl`, `opencode.db`) across Claude Code, Pi, OpenAI Codex, or OpenCode harnesses. You MUST
  use this skill whenever debugging parsing discrepancies, activity state glitches, token pricing
  errors, or read-side query failures offline via `air-debug` or `just session-replay-<harness>`.
  Do NOT use for live daemon monitoring (`just logs`) or browser UI testing.
author: alexgorbatchev
metadata:
  created_on: 2026-09-21 13:00
  last_modified: 2026-09-21 13:00
  status: current
---

## 1. Quick Replay by Session ID

When given a session ID, replay it directly using root `just` recipes:

```bash
# Claude Code session
just session-replay-claude <session_id>

# Pi session
just session-replay-pi <session_id>

# OpenAI Codex rollout
just session-replay-codex <session_id>

# OpenCode database session
just session-replay-opencode <session_id>
```

Append replay flags directly to any recipe:
```bash
just session-replay-pi <session_id> --diff
just session-replay-claude <session_id> --until-line 50
just session-replay-opencode <session_id> --json
```

---

## 2. Locating Session Transcripts On Disk

If locating files manually or replaying archived sessions, inspect default harness paths:

| Harness | Default Storage Locations | Session ID Pattern / Format |
| :--- | :--- | :--- |
| **Claude Code** | `~/.claude/projects/*/<id>.jsonl`<br>`~/.claude/sessions/<id>.jsonl` | UUID string (e.g. `c0ffee...-....jsonl`) |
| **Pi** | `~/.pi/agent/sessions/--<slug>--/<id>.jsonl` | `YYYY-MM-DDTHH-mm-ss-SSSZ_<uuid>.jsonl` |
| **OpenAI Codex** | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | Rollout path or UUID inside `session_meta` |
| **OpenCode** | `~/.local/share/opencode/opencode.db` | SQLite database table `session` (`id` column) |

---

## 3. Core Offline Diagnostics Workflows

Execute `just debug` (or `packages/agent-debug/bin/air-debug`) directly against any file or session ID:

### A. Full Session Replay & Summary
```bash
just debug session replay <path-to-transcript.jsonl | session_id>
```
Verifies the complete ingestion lifecycle:
1. Auto-detects harness type.
2. Ingests all events into an in-memory SQLite store (`:memory:`).
3. Evaluates deterministic state machine transitions.
4. Accumulates token usage and prices against `models.json`.
5. Synthesizes `run_started` / `run_exited` lifecycle boundaries.
6. Executes server-side queries (`GetAgentByID`, `GetAgentLogs`) to verify clean schema deserialization.

### B. Offline Session Validation
```bash
just debug session validate <path-to-transcript.jsonl | session_id>
```
Executes assertions against the transcript:
- **Harness Detection**: Confirms recognized log format.
- **Event Framing**: Asserts valid timestamps and unmarshaled data payloads without syntax errors.
- **State Finalization**: Asserts agent terminates cleanly with `Lifecycle=EXITED`.
- **Usage & Pricing**: Asserts non-negative token counts and non-negative cost.
- **Server Queries**: Asserts `GetAgentByID` and `GetAgentLogs` scan without errors.

Exits with `0` on success, `1` on validation failure.

### C. OpenCode Multi-Session Inspection
```bash
# List all sessions in database
just debug session list ~/.local/share/opencode/opencode.db

# Replay specific session by ID
just debug session replay --session-id ses_123 opencode.db
```

### D. Ingested Telemetry Event Inspection (`events`)
Inspect the raw sequence of telemetry events persisted to SQLite `agent-status.db`:

```bash
# View last 20 events for a session
just debug events <session-id>

# Filter by event types (e.g. lifecycle and errors)
just debug events <session-id> --type run_started,run_exited,error

# Inspect specific event payload by ID as JSON
just debug events --id <event-id> --json

# Stream events in ascending chronological order
just debug events <session-id> --asc
```

---

## 4. Root-Cause Analysis Techniques

### Step-by-Step State Mutation Diffs (`--diff`)
Trace activity flipping, active tool transitions, and token accumulation per turn:
```bash
air-debug session replay --diff session.jsonl
```
Inspect field mutations:
- `Activity: IDLE -> BUSY` (prompt received or tool running)
- `ActiveTool: nil -> "Edit"` / `ActiveTarget: nil -> "server.go"`
- `Question: nil -> "Do you want to run this command?"`
- `LastError: nil -> "..."`

### Breakpoint Analysis (`--until-line`, `--until-event`)
Isolate the exact turn where state corrupts or token calculation drifts:
```bash
# Halt replay after line N
air-debug session replay --until-line 42 session.jsonl

# Halt replay after first occurrence of an event type
air-debug session replay --until-event question_prompt session.jsonl

# Halt immediately on the first parse error or state machine failure
air-debug session replay --stop-on-error session.jsonl
```

### Foreign Transcript Path Remapping (`--cwd`)
When diagnosing transcripts captured on other machines, remote worktrees, or CI runners, remap Git resolution to your local checkout:
```bash
air-debug session replay --cwd /Users/you/development/agent-status session.jsonl
```

### Subagent Tree Verification
Claude Code subagent directories (`<session_id>/subagents/*.jsonl` or sibling `subagents/*.jsonl`) are automatically detected and interleaved by timestamp.
Verify:
1. `Subagents: N active/completed` in human summary.
2. `subagent:<id>` tag on step-by-step diff lines.
3. Subagent frames appear in read verification logs (`GetAgentLogs`).

### SQLite Database Export (`--export-db`)
Retain the fully populated SQLite database on disk for manual SQL queries or testing with `air-server`:
```bash
air-debug session replay --export-db .tmp/debug.db session.jsonl

# Inspect database directly
sqlite3 .tmp/debug.db "SELECT id, activity, tokens_total, cost_usd FROM agents;"
sqlite3 .tmp/debug.db "SELECT id, event_type, timestamp FROM telemetry_events ORDER BY id;"
```

---

## 5. Dual-Mode Output (`AGENT=1`)

When running automated checks or script evaluations, invoke with `AGENT=1` for compact, token-conservative output without tables or dividers:

```bash
AGENT=1 air-debug session replay session.jsonl
```
Output contract:
```text
harness:claude-code session_id:ses-1 agent_id:env-debug:claude-code:ses-1 project:my-repo branch:main activity:IDLE lifecycle:EXITED tokens_in:1500 tokens_out:300 tokens_total:1800 cost_usd:0.0045 subagents:0 frames:5 verification:OK ver_logs:5 duration_ms:12
```
