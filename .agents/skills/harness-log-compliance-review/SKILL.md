---
name: harness-log-compliance-review
description: >-
  REQUIRED when reviewing, auditing, or verifying agent log parsers (`github.com/alexgorbatchev/agent-parser`,
  `packages/agent-observer/`, `packages/agent-ingester/`) against canonical harness specifications
  in `github.com/alexgorbatchev/agent-parser` (`docs/harness-logs/`: `claude-code.ts`, `pi.ts`, `codex.ts`, `opencode.ts`). You MUST read
  this to execute the schema compliance checklist, identify parsing gaps, verify downstream
  telemetry wiring, and file findings as GitHub issues. Do NOT use for general UI component review.
author: alexgorbatchev
metadata:
  created_on: 2026-09-20 04:05
  last_modified: 2026-09-20 04:05
  status: current
---

## 1. Ground Truth Schema Verification

Always inspect the canonical TypeScript reference specifications in the `agent-parser` repository (`https://github.com/alexgorbatchev/agent-parser/tree/main/docs/harness-logs` or local checkout) before reviewing code:

Inspect the 4 reference harness contracts:

- `docs/harness-logs/claude-code.ts` — Flat JSONL event stream, thinking/tool blocks, usage breakdown, attachments, and API error formats.
- `docs/harness-logs/pi.ts` — DAG JSONL tree (`{id, parentId}`), `IPiCostBreakdown` object, model/label updates, compaction, and message roles.
- `docs/harness-logs/codex.ts` — Date-partitioned rollout JSONL (`YYYY/MM/DD/`), string-encoded JSON arguments, subagent thread spawns, and synthetic `<environment_context>` filtering.
- `docs/harness-logs/opencode.ts` — SQLite WAL database (`opencode.db`), read-only mode (`?mode=ro`), and JSON-in-TEXT column deserialization (`session.model`, `message.data`, `part.data`).

---

## 2. Parser Schema Compliance Checklist

Systematically audit `agent-parser` (`github.com/alexgorbatchev/agent-parser`) against each harness contract:

### A. Top-Level Line & Record Parsing

- **Claude Code**: Verify handling for all top-level record types (`assistant`, `user`, `ai-title`, `last-prompt`, `attachment`, `system`, `permission-mode`, `progress`, `pr-link`). Ensure API errors (`error`, `apiErrorStatus`, `isApiErrorMessage`) are parsed and emitted as error events.
- **Pi**: Verify line 1 header (`type: "session"`) vs graph nodes (`message`, `model_change`, `thinking_level_change`, `compaction`, `branch_summary`, `custom_message`, `label`, `session_info`). Verify `model_change` accepts `modelId` (with fallbacks) and `label` accepts `label`.
- **Codex**: Verify `session_meta`, `turn_context`, `event_msg`, and `response_item` unmarshaling. Verify secondary unmarshaling of stringified JSON in `function_call.arguments`.
- **OpenCode**: Verify SQLite read-only connection strings (`?mode=ro&_pragma=busy_timeout(5000)`), query filtering, and JSON-in-TEXT parsing.

### B. Content Block Classification

- **Assistant Conversational Text**: Verify assistant `type: "text"` content blocks are captured and attached to `turn_end` or conversational events, rather than silently dropped.
- **Thinking vs Text Separation**: Ensure regular assistant text is NEVER misclassified or emitted as `EventType: "thinking"`.
- **Question Prompts**: Verify `AskUserQuestion` / `ask_user` tool invocations extract question text from all parameter variants (`question`, `prompt`, or `questions` array) and emit `question_prompt` events.
- **Tool Results**: Verify `is_error` / `isError` boolean flags, error text, and tool output strings are extracted.

### C. Token Usage & Cost Accounting

- **Anthropic / Claude Code Semantics**: `output_tokens` already includes `reasoning_output_tokens` / `thinking_tokens`. Ensure total calculations (`Input + Output + CacheRead + CacheCreation`) do not add reasoning tokens twice.
- **Pi Cost Breakdown**: `usage.cost` may be written as a numeric float or an `IPiCostBreakdown` object (`{input, output, cacheRead, cacheWrite, total}`). Ensure parser unmarshals both forms without dropping turns.
- **Codex & OpenCode Usage**: Ensure reasoning tokens, cached prompt tokens, and completion totals are mapped accurately to `ITokenUsage`.

---

## 3. Downstream Telemetry & Ingestion Wiring Trace

Audit the complete path from parsed event to database and frontend:

1. **Observer Scanner & Tailer (`packages/agent-observer`)**:
   - Verify `air-observer` has active scanners for each supported harness.
   - Verify tailer callbacks route lines through an explicit switch on `frame.Harness` rather than binary fallbacks.
   - Verify `ParsedEvent` properties (`Timestamp`, `EventType`, `CWD`, `GitBranch`, `Data`) are mapped 1-to-1 into `ITelemetryFrame`.
2. **Ingester Storage (`packages/agent-ingester/internal/store`)**:
   - Verify `store.RecordTelemetryEvent(frame)` persists `frame.Data` as `data_json` in `telemetry_events` table so zero data is lost.
3. **Ingester Classifier (`packages/agent-ingester/internal/classifier`)**:
   - Verify deterministic state machine transitions on `frame.EventType`: `run_started`, `run_exited`, `turn_start`, `thinking`, `tool_call`, `tool_result`, `question_prompt`, `error`, `turn_end`, `title_change`.
   - Verify `extractTarget()` keys inspect relevant tool parameters (`file_path`, `path`, `notebook_path`, `command`, `url`, `query`, `skill`, `pattern`, `prompt`).
   - Verify cumulative token accounting in `agent.Activity.Usage` uses `u.Total` directly or adds non-overlapping token components.
4. **Server & Web UI Synchronization (`packages/agent-server`, `packages/ui-web`)**:
   - Verify query handlers and SSE subscriber streams transmit telemetry frames to the UI.
   - Verify TypeScript definitions in `packages/ui-web/src/types/types.ts` stay aligned with Go `packages/agent-common/types/types.go`.

---

## 4. Test Execution & Coverage Verification

Run all test suites across the parser, observer, ingester, and server packages:

```bash
cd packages/agent-parser && go test -v -race -cover ./...
cd ../agent-observer && go test -v -race -cover ./...
cd ../agent-ingester && go test -v -race -cover ./...
cd ../agent-server && go test -v -race -cover ./...
cd ../ui-web && bun run build && bun x tsc --noEmit
```

---

## 5. Issue Formulation Workflow

When compliance gaps, dropped fields, or schema defects are discovered:

1. **Search for duplicates**:
   ```bash
   gh issue list --state all --search "<symbol or keyword>" --limit 30
   ```
2. **Draft issue to `.tmp/issue-<name>.md`**:
   - Line 1: Title (defect and consequence, present tense, naming symbols/files).
   - Line 2: Blank line.
   - Line 3+: Markdown body with `## Summary`, `## Evidence`, `## Why it matters`, `## Expected behavior`, `## Suggested direction` (for bugs) or `## Motivation`, `## Proposed Solution`, `## Proposed Changes` (for enhancements).
3. **Get user confirmation**, then create and verify labels:
   ```bash
   sed '1,2d' .tmp/issue-<name>.md | gh issue create \
     --title "$(sed -n '1p' .tmp/issue-<name>.md)" \
     --label <bug|enhancement> \
     --body-file -
   gh issue view <number> --json number,title,labels
   ```
