---
created_on: 2026-10-14 15:07
last_modified: 2026-11-25 05:29
status: current
---

# Configuration

Use local transcripts and an existing monitoring database for offline diagnostics. Optional remote reads contact Agent Status; explicit summarization contacts the configured LLM providers.

## Session discovery

An existing file resolves directly. An Antigravity session directory resolves its native transcript. Other directories resolve their first discovered `.jsonl` or `.db` file. Exact IDs take priority over partial matches. Multiple matches produce an error.

- Claude lookup searches `CLAUDE_SESSIONS_DIR` and `CLAUDE_PROJECTS_DIR` overrides, `~/.claude/{projects,sessions}`, `~/.local/share/ai-registry/claude-code/{projects,sessions}`, and the matching `XDG_DATA_HOME` locations.
- Pi lookup uses `PI_SESSIONS_DIR`, then `~/.pi/agent/sessions`.
- Codex lookup uses `CODEX_SESSIONS_DIR`, then `~/.codex/sessions`.
- Antigravity CLI lookup uses `AGY_SESSIONS_DIR`, then `${GEMINI_HOME:-~/.gemini}/antigravity-cli/brain`. Search by exact ID or unique prefix, and pass `--harness agy` or `--harness antigravity` to restrict the search.
- OpenCode lookup uses `OPENCODE_DB_PATH`, then `${XDG_DATA_HOME:-~/.local/share}/opencode/opencode.db`.
- Native scanner/local discovery additionally honors `PI_CODING_AGENT_DIR` and `CODEX_HOME` when their explicit sessions directory variables are absent. Claude scanner directories use XDG data and home fallbacks beside explicit overrides.
- OpenCode IDs match exactly. JSONL identities derive from the resolved transcript and harness metadata. A telemetry working directory must be representable as UTF-8.

## Monitoring database

Database commands use the same search order:

1. Explicit `--db <path>`.
2. `AGENT_STATUS_DB_PATH`, when set.
3. `${XDG_DATA_HOME:-~/.local/share}/agent-status/agent-status.db`.
4. `./.tmp/dev-stack/data/agent-status/agent-status.db`.

Both explicit sources expand `~/` against the native home directory and fail immediately if the configured file is missing.

These inputs remain read-only. An explicit `--export-db <path>` creates or opens a writable ingestion database and commits processed frames incrementally.

## Remote reads

`--server-url` takes precedence over `AGENT_SERVER_HTTP_URL`, then `http://127.0.0.1:4502`. URLs accept HTTP or HTTPS and exclude embedded credentials, query strings, and fragments. `AGENT_STATUS_TOKEN` supplies an optional bearer token. Local discovery and local database reads operate independently of this token. Server reads retain system proxy configuration and use a ten-second request deadline, one attempt, and zero redirects.

## Optional summarization

Enable `session replay --summarize` to resolve providers from environment variables. Disabled summarization performs zero provider lookups. Providers run in this fallback order:

1. OpenAI-compatible endpoint: `LITELLM_BASE_URL`, then `OPENAI_BASE_URL`; key `LITELLM_API_KEY`, then `LITELLM_KEY`, then `OPENAI_API_KEY`. An endpoint or key activates this provider, whose default model is `gpt-4o-mini`.
2. Gemini: `GEMINI_API_KEY`; default model `gemini-2.0-flash`.
3. Anthropic: `ANTHROPIC_API_KEY`; default model `claude-3-5-haiku-latest`.

`AGENT_STATUS_SUMMARIZER_MODEL` overrides each configured provider's default. Calls have a ten-second timeout and command cancellation; fallback summaries remain available when providers fail. Provider billing follows the configured service's terms.

## Output and logs

`AGENT=1`, `true`, or `yes` selects compact output, case-insensitive after trimming. `COLUMNS` and native terminal width determine human layout. Results use stdout; warnings, returned errors, and explicit `--debug` diagnostics use stderr. All printed paths fold the native home directory to `~` and strip terminal control sequences.

Runtime logging rotates files; metadata/help/version parsing completes before file logging setup. Configuration:

| Variable | Effective default |
| :--- | :--- |
| `AGENT_STATUS_LOG_FILE` | Shared directory plus `agent-debug.log`; native rotation adds its record suffix. |
| `AGENT_STATUS_LOG_DIR` | `${XDG_STATE_HOME}/agent-status/logs`, then `${XDG_DATA_HOME}/agent-status/logs`, then `~/.local/state/agent-status/logs`. |
| `AGENT_STATUS_LOG_TO_DISK` | enabled; `0`, `false`, `no`, or `off` disables files. |
| `AGENT_STATUS_LOG_MAX_SIZE_MB` | 10; positive integer MiB. |
| `AGENT_STATUS_LOG_MAX_BACKUPS` | 5; zero retains an unlimited archive count. |
| `AGENT_STATUS_LOG_MAX_AGE_DAYS` | 14; zero removes the age limit. |
| `AGENT_STATUS_LOG_COMPRESS` | enabled; `0`, `false`, `no`, or `off` disables compression. |

Console logging defaults to warnings and errors; `--debug` enables diagnostic records. SIGINT and SIGTERM cancel current work and wait for owned tasks to finish.
