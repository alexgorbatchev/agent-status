---
created_on: 2026-09-19 02:15
last_modified: 2026-09-19 02:15
status: current
---

# User Specification & Wishes: Agent Status

## 1. Problem Statement & User Context

As a developer running multiple AI coding agents (Claude Code, Pi, OpenCode, Codex) simultaneously across multiple physical and virtual machines (e.g. local MacBook, remote Linux dev boxes, staging VMs), I have no single, unified place to monitor what my agents are doing.

Currently:
- Agents run in disparate terminal windows, background tmux sessions, or remote servers.
- When an agent gets blocked on a user confirmation (such as an `AskUserQuestion` prompt or permission check), it sits idle for hours without me noticing unless I manually cycle through every open terminal.
- Multiple agents frequently work on the same codebase simultaneously—sometimes in the root directory, sometimes in dedicated git worktrees (e.g. `.workspaces/feat-x` or `.claude/worktrees/feat-y`). There is no easy way to group them by project.
- Reading raw model logs across many windows is overwhelming. I want high-level, human-readable summaries of what each agent is actively working on without losing the ability to inspect raw logs when needed.

---

## 2. Core User Wishes & Functional Requirements

### Wish 1: Centralized Fleet Observability
- **Wish**: A single, central dashboard where I can see all my running agents across all my machines at a glance.
- **Details**:
  - Automatically track when agents start, execute tasks, wait for input, or exit.
  - Support agents running on multiple different host machines ("boxes") over the network.
  - Show overall fleet metrics: total running agents, active environments, and count of agents requiring immediate user attention.

### Wish 2: Environment Awareness & Tagging
- **Wish**: Clearly identify which machine each agent is running on.
- **Details**:
  - Every agent must display a clear, readable environment badge (e.g., `work-macbook`, `dev-box-linux`, `staging-vm`).
  - Environment names must be configurable per machine via CLI flags, environment variables, or config files, falling back to hostname.
  - The UI must allow grouping agents either by **Project** (showing all agents on a repo across all machines) or by **Environment** (showing all agents running on a specific box).

### Wish 3: Smart Project & Git Worktree Grouping
- **Wish**: Group agents by project name regardless of what directory, subfolder, or git worktree they are operating in.
- **Details**:
  - When multiple agents work on the same repository (even in separate `.workspaces/...` worktrees), group them under the canonical project name (e.g. `ai-registry`, `dotfiles-installer`).
  - Each agent card must clearly show its active git branch (e.g. `🌿 main`, `🌿 feat-oauth`) and whether it is running in an isolated worktree.

### Wish 4: Instant Status & Attention Signals
- **Wish**: Instantly know what state each agent is in, especially when one is waiting for my input.
- **Details**:
  - **❓ WAITING_QUESTION**: Immediately flag when an agent is blocked on an `AskUserQuestion` or interactive user prompt. Display the exact question text in an alert banner at the top of the dashboard so I can attend to it immediately.
  - **⚡ BUSY / ACTIVE**: Indicate that the agent is actively thinking or running tools. Show the active tool name (e.g. `Edit`, `Bash`, `Read`) and target file path.
  - **⚠️ ERROR / BLOCKED**: Highlight when an agent encounters tool failures, command errors, or non-zero exits.
  - **✅ DONE / IDLE**: Indicate when an agent has completed its task or turn.

### Wish 5: AI-Powered Action & Thought Summarization
- **Wish**: Tap into model thoughts and tool calls, using a very cheap LLM to continuously summarize what the agent is doing into a clean, single-sentence status.
- **Details**:
  - Extract latest assistant thinking excerpts and recent tool invocations.
  - Use a fast, cost-effective model (e.g. **Gemini 2.5 Flash**) to generate concise, human-readable action summaries (e.g., *"Refactoring build pipeline in build.ts"* or *"Running unit tests for zsh configuration"*).
  - Summarization must be debounced (e.g. every 10–15s during active runs or on turn boundaries) to keep LLM API costs negligible.

### Wish 6: Dedicated Log Ingestion & Live Deep-Dive
- **Wish**: Robust log ingestion that tracks all agent activity without data loss, plus the ability to inspect raw logs on demand.
- **Details**:
  - Ingest raw events and logs from agent session files (.jsonl, SQLite DBs) using persistent byte-offset cursors so restarts do not drop logs.
  - Allow clicking into an agent card to open a slide-over Live Execution Drawer displaying real-time streaming logs (model thoughts, tool inputs, stdout/stderr).

---

## 3. Technology & Architectural Constraints

### 1. Edge Telemetry: `agent-observer` (Go)
- **Role**: Lightweight background daemon running on each developer machine/box.
- **Language**: **Go** (for minimal memory footprint, single-binary distribution, and native OS/process/file monitoring).
- **Function**: Scans local agent processes, resolves Git root/worktree context, tails session logs via byte-offset cursors, and ships telemetry frames over WebSocket to the central Ingester.
- **Terminology**: Must be named **`agent-observer`** (avoiding terms like "collector").

### 2. Stream Processing & Summarization: `agent-ingester` (Go)
- **Role**: Central stream ingestion and summarization engine.
- **Language**: **Go** (for concurrent stream handling, low latency, and efficient SQLite writes).
- **Function**: Runs as an independent background service. Receives WebSocket streams from all observers, applies 0-ms deterministic status rules, queues LLM summarization calls, and persists events/states into SQLite in WAL mode.

### 3. API & Real-Time Streaming: `agent-server` (Go)
- **Role**: Backend HTTP API and real-time event broadcaster.
- **Language**: **Go**.
- **Function**: Runs as an independent background process from the Ingester. Exposes REST endpoints (`/api/v1/projects`, `/api/v1/agents`) and broadcasts real-time updates to web clients over Server-Sent Events (SSE) and WebSockets.
- **Constraint**: **No UI embedding** inside the Go binary. `agent-server` operates purely as a standalone API and event server.

### 4. Web Dashboard: `ui-web` (Bun + TypeScript + Tailwind v4)
- **Role**: Real-time web frontend application.
- **Technology**: **Bun + TypeScript + React + Tailwind v4** (using modern CSS-first `@import "tailwindcss";` directives).
- **Naming**: Directory named **`ui-web`**.
- **Function**: Consumes the SSE/REST APIs from `agent-server`, providing reactive project/environment views, question banners, status pills, and live log streaming.

### 5. Future-Proofing for Alternative Frontends
- The architecture must keep backend streaming APIs decoupled and lightweight so alternative frontends (such as an ESP32 microcontroller with a small e-paper/TFT display or terminal TUIs) can easily consume status feeds via compact endpoints in the future.

### 6. Repository Structure
- All four components must live in their own dedicated packages under `packages/` in the `../agent-status` repository:
  - `packages/agent-observer`
  - `packages/agent-ingester`
  - `packages/agent-server`
  - `packages/ui-web`

---

## 4. User Experience & User Journey

### A. Morning / Fleet Check-in
1. I start `agent-ingester` and `agent-server` on my main server (or development machine).
2. On my MacBook and Linux dev boxes, `agent-observer` runs in the background (e.g. as a systemd/launchd service or quick CLI command).
3. I open the Web UI in my browser.
4. The dashboard immediately shows all online environments (`work-macbook`, `dev-box-linux`) and active agents grouped by project.

### B. Active Coding & Multitasking
1. I launch 3 Claude Code agents and 1 Pi agent across different branches and worktrees on different machines.
2. The dashboard updates in real time:
   - `ai-registry` shows 2 agents actively running on `work-macbook` with live action summaries describing their current edits.
   - `dotfiles-installer` shows 1 agent running unit tests on `dev-box-linux`.
3. When one agent encounters a decision point and calls `AskUserQuestion`, a high-visibility amber banner appears at the top:
   > ❓ **dotfiles-installer** (`dev-box-linux` / `feat-zsh`): *"Do you want to enable Oh-My-Zsh plugins by default?"*
4. I see the question immediately, switch to that terminal/box, answer it, and the dashboard seamlessly transitions the agent back to `BUSY`.

### C. Deep-Dive Debugging
1. If an agent fails or enters `ERROR` state, I click the agent card in the dashboard.
2. A slide-over drawer opens showing the exact tool call, the command arguments, the stderr output, and the model's preceding thought block so I understand what broke without manually digging through log directories.

---

## 5. Success Criteria

- [ ] **Multi-Box & Multi-Agent**: Successfully aggregates agents running simultaneously across multiple host environments into one dashboard.
- [ ] **Instant Question Detection**: Zero delay in detecting when an agent is waiting for user input (`AskUserQuestion`).
- [ ] **Accurate Worktree Normalization**: Agents running in `.workspaces/...` or worktree subdirectories are accurately grouped under their parent repository name.
- [ ] **High-Quality AI Summaries**: Every active agent card displays an accurate, single-sentence summary of its active task generated by Gemini 2.5 Flash.
- [ ] **Clean Separation of Concerns**: Observer (Go), Ingester (Go), Server (Go, no UI embedding), and UI (Bun/TS `ui-web`) run as independent, decoupled packages.
