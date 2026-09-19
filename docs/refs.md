---
created_on: 2026-09-18 20:30
last_modified: 2026-09-18 20:30
status: current
---

# External References & Libraries: Agent Status

This document catalogs verified third-party repositories, libraries, SDKs, and architectural reference projects relevant to building, maintaining, and extending `agent-status`.

All repository URLs in this document were verified live via HTTP status checks against upstream hosts.

---

## 1. Go Libraries & Direct Dependencies

### 1.1. Harness Log & Transcript Parsing in Go

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/ProjAnvil/claude-agent-sdk-golang`](https://github.com/ProjAnvil/claude-agent-sdk-golang) | Go SDK for Claude Code. Contains pure-Go transcript parsing in `sessions.go` (`GetSessionMessages`) and builds conversation chains via `parentUuid` links. |
| [`github.com/janekbaraniewski/openusage`](https://github.com/janekbaraniewski/openusage) | Terminal dashboard in Go. Contains clean file discovery and session-reading implementations under `internal/providers/claude_code/` and `internal/detect/`. |
| [`github.com/valpere/session-indexer`](https://github.com/valpere/session-indexer) | Go CLI that parses Claude Code JSONL transcripts from `~/.claude/projects/` and indexes them into SQLite with full-text search. |
| [`github.com/sonnes/chitragupt`](https://github.com/sonnes/chitragupt) | Go CLI (`cg`) that parses AI coding agent session logs and exports structured transcripts. |
| [`github.com/robzolkos/claude-session-export`](https://github.com/robzolkos/claude-session-export) | Zero-dependency Go CLI tool for parsing local Claude Code JSONL session files. |
| [`github.com/bpowers/go-claudecode`](https://github.com/bpowers/go-claudecode) | Native Go SDK for programmatic interaction with the Claude Code CLI and process sandbox runtime. |

### 1.2. Process Discovery & OS Introspection

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/shirou/gopsutil`](https://github.com/shirou/gopsutil) | Cross-platform system and process library (`gopsutil/v4/process`). On Darwin/macOS, provides native CWD extraction via `libproc` (`proc_pidinfo` with `PROC_PIDVNODEPATHINFO`), avoiding subprocess `lsof` execution overhead. |
| [`github.com/mitchellh/go-ps`](https://github.com/mitchellh/go-ps) | Lightweight, cross-platform OS process table enumerator in pure Go. |

### 1.3. File Watching, Tailing, & Cursor Tracking

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/fsnotify/fsnotify`](https://github.com/fsnotify/fsnotify) | Cross-platform file system event notifications for detecting new session descriptors and log file writes. |
| [`github.com/nxadm/tail`](https://github.com/nxadm/tail) | File tailing library in Go supporting log rotation, truncation detection, and seek positions. |
| [`github.com/go-faster/tail`](https://github.com/go-faster/tail) | High-performance Go file tailer integrated with `fsnotify`. |

### 1.4. Network Transport & Persistence

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/coder/websocket`](https://github.com/coder/websocket) | Idiomatic, high-performance WebSocket client and server library in Go (formerly `nhooyr.io/websocket`). |
| [`github.com/mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3) | Cgo SQLite3 driver supporting WAL mode, `PRAGMA incremental_vacuum`, and `PRAGMA user_version` migrations. |
| [`gitlab.com/cznic/sqlite`](https://gitlab.com/cznic/sqlite) | Pure-Go SQLite driver without Cgo requirements, simplifying cross-compilation across Darwin and Linux. |

### 1.5. CLI & Help Generation

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/spf13/cobra`](https://github.com/spf13/cobra) | Standard Go CLI framework for `agent-observer`, `agent-ingester`, and `agent-server`. Built-in doc generators (`doc.GenMarkdownTree`) support rendering command help hierarchies. |

---

## 2. Harness Specifications & SDKs

### 2.1. Pi (`@earendil-works/pi`)

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/earendil-works/pi`](https://github.com/earendil-works/pi) | Core repository for the Pi coding agent toolkit. Contains authoritative documentation for session serialization (`packages/coding-agent/docs/session-format.md`), path encoding, and tree-structured message history. |

### 2.2. Claude Code Transcripts & Parsers (TypeScript / Python)

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/Magic-Man-us/cc-session-core`](https://github.com/Magic-Man-us/cc-session-core) | Exhaustive, typed parser for Claude Code and Codex JSONL transcripts. Implements tool-call pairing (`tool_result.tool_use_id` correlation) and schema modeling. |
| [`github.com/Guiziweb/claude-code-data`](https://github.com/Guiziweb/claude-code-data) | TypeScript parser for Claude Code session files, streaming tokens, transcripts, and subagent turns. |
| [`github.com/simonw/claude-code-transcripts`](https://github.com/simonw/claude-code-transcripts) | Python CLI tool for converting Claude Code session files into structured HTML transcripts. |
| [`github.com/kiliman/claude-transcript`](https://github.com/kiliman/claude-transcript) | TypeScript/Node CLI for auto-detecting and converting Claude Code JSONL files to Markdown. |
| [`github.com/daaain/claude-code-log`](https://github.com/daaain/claude-code-log) | Python CLI tool converting Claude Code transcript JSONL files into structured HTML/Markdown. |

---

## 3. Multi-Agent Observability & Dashboard Projects (Architecture References)

### 3.1. Unified Agent Dashboards & Monitors

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/neochoon/agenthud`](https://github.com/neochoon/agenthud) | Real-time TUI dashboard for multi-agent fleets (Claude Code, OpenAI Codex, OpenCode, AWS Kiro). Implements `agenthud follow`, a unified chronological activity and lifecycle NDJSON event stream. |
| [`github.com/markojak/harness`](https://github.com/markojak/harness) | Real-time observability dashboard across Claude Code, Codex CLI, and OpenCode sessions. |
| [`github.com/KyleAMathews/claude-code-ui`](https://github.com/KyleAMathews/claude-code-ui) | Real-time web dashboard with LLM-generated summaries and status tracking (Working, Needs Approval, Waiting, Idle). |
| [`github.com/onikan27/claude-code-monitor`](https://github.com/onikan27/claude-code-monitor) | Real-time terminal TUI and mobile web dashboard for monitoring concurrent Claude Code sessions. |
| [`github.com/hoangsonww/claude-code-agent-monitor`](https://github.com/hoangsonww/claude-code-agent-monitor) | Full-stack monitoring dashboard with WebSockets, SQLite, and React for tracking sessions and tool calls. |
| [`github.com/ayu5h-raj/claude-monitor`](https://github.com/ayu5h-raj/claude-monitor) | Terminal-themed desktop and web dashboard reading `~/.claude/` session state. |
| [`github.com/rauzaruku/claude-session-visualizer`](https://github.com/rauzaruku/claude-session-visualizer) | Browser-based visualizer for Claude Code execution trees, thinking traces, and tool usage. |
| [`github.com/xtalax/agent-conductor`](https://github.com/xtalax/agent-conductor) | Real-time session tracker and mission control dashboard for multi-session agent workflows. |
| [`github.com/pedropauloai/claude-agent-monitor`](https://github.com/pedropauloai/claude-agent-monitor) | Observability dashboard for tracking agent activity, tool calls, and sprint execution. |
| [`github.com/varunr89/claude-transcript-viewer`](https://github.com/varunr89/claude-transcript-viewer) | Web viewer for Claude Code session transcripts with full-text and semantic search. |
| [`github.com/darklord1611/cc_transcript_viewer`](https://github.com/darklord1611/cc_transcript_viewer) | Local web viewer for browsing Claude Code, Codex, and Cursor transcripts in a unified timeline. |
| [`github.com/ravikanchikare/claude-code-sessions-browser`](https://github.com/ravikanchikare/claude-code-sessions-browser) | Local browser tool for grouping tool calls, tracking token counts, and comparing session transcripts. |
| [`github.com/kosinal/claude-code-dashboard`](https://github.com/kosinal/claude-code-dashboard) | Node.js web dashboard monitoring Claude Code session states via Server-Sent Events (SSE). |
| [`github.com/joe-re/eyes-on-claude-code`](https://github.com/joe-re/eyes-on-claude-code) | Menu bar and dashboard monitoring session states and pending user inputs across projects. |

---

## 4. Deferred Harnesses (Phase 5+ Reference Material)

### 4.1. OpenCode (`opencode.db` SQLite Schema)

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/sst/opencode`](https://github.com/sst/opencode) | Upstream OpenCode repository (`packages/opencode/src/storage/db.ts`). Documents the global SQLite database layout at `${XDG_DATA_HOME:-~/.local/share}/opencode/opencode.db` (`session`, `message`, `part` tables). |
| [`github.com/nicknisi/sessions`](https://github.com/nicknisi/sessions) | Reference implementation for querying the OpenCode SQLite database read-only and synthesizing standard message event streams. |

### 4.2. Codex CLI (`~/.codex/sessions/*.jsonl`)

| Repository | Purpose & Relevance |
|---|---|
| [`github.com/brucehart/codex-transcripts`](https://github.com/brucehart/codex-transcripts) | Python CLI tool for parsing Codex rollout JSONL files (`rollout-*.jsonl`) from `~/.codex/sessions/`. |
