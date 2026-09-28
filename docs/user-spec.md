---
created_on: 2026-09-18 20:15
last_modified: 2026-09-18 20:15
status: current
---

# User Specification & Wishes: Agent Status

This document captures the problem, the user's wishes, and the constraints they impose. The engineering decisions that satisfy them live in `docs/prd.md`.

## 1. Problem Statement & User Context

As a developer running multiple AI coding agents simultaneously across multiple physical and virtual machines (local MacBook, remote Linux dev boxes, staging VMs), I have no single, unified place to monitor what my agents are doing.

Currently:

- Agents run in disparate terminal windows, background tmux sessions, or remote servers.
- When an agent gets blocked on a user confirmation, it sits idle for hours without me noticing unless I manually cycle through every open terminal.
- Multiple agents frequently work on the same codebase simultaneously — sometimes in the root directory, sometimes in dedicated git worktrees (`.workspaces/feat-x`, `.claude/worktrees/feat-y`). There is no easy way to group them by project.
- Reading raw model logs across many windows is overwhelming. I want high-level, human-readable summaries of what each agent is actively working on without losing the ability to inspect raw logs when needed.

### Harness Scope

The first release supports **Claude Code** and **Pi**.

**Codex** and **OpenCode** are deferred. Both were considered and dropped from the initial scope: Codex has no specified discovery path yet, and OpenCode stores sessions in SQLite rather than JSONL, which needs a separate tailer design. Neither appears in the type system, the database, or the UI until it is picked back up.

---

## 2. Core User Wishes & Functional Requirements

### Wish 1: Centralized Fleet Observability

- **Wish**: A single, central dashboard where I can see all my running agents across all my machines at a glance.
- **Details**:
  - Automatically track when agents start, execute tasks, wait for input, or exit.
  - Support agents running on multiple different host machines over the network.
  - Show overall fleet metrics: total running agents, active environments, and count of agents requiring immediate user attention.

### Wish 2: Environment Awareness & Tagging

- **Wish**: Clearly identify which machine each agent is running on.
- **Details**:
  - Every agent must display a clear, readable environment badge (`work-macbook`, `dev-box-linux`, `staging-vm`).
  - Environment names must be configurable per machine via CLI flags, environment variables, or config files, falling back to hostname.
  - The UI must allow grouping agents either by **Project** (all agents on a repo across all machines) or by **Environment** (all agents on a specific box).

### Wish 3: Smart Project & Git Worktree Grouping

- **Wish**: Group agents by project name regardless of what directory, subfolder, or git worktree they are operating in.
- **Details**:
  - When multiple agents work on the same repository, even in separate `.workspaces/...` worktrees, group them under the canonical project name.
  - Each agent card must clearly show its active git branch and whether it is running in an isolated worktree.

### Wish 4: Instant Status & Attention Signals

- **Wish**: Instantly know what state each agent is in, especially when one is waiting for my input.
- **Details**:
  - **WAITING_QUESTION**: Immediately flag when an agent is blocked on an `AskUserQuestion` prompt. Display the exact question text in an alert banner at the top of the dashboard.
  - **BUSY**: Indicate that the agent is actively thinking or running tools. Show the active tool name and target file path.
  - **ERROR**: Highlight when an agent hits a genuine tool failure. A tool that reports failure as its normal result — a `grep` with no match, a test suite that is supposed to fail — must not raise this. A banner that cries wolf is worse than no banner.
  - **RATE_LIMITED**: Flag when an agent hits provider rate limits or quotas (HTTP 429), capturing reset timestamps (`rateLimitResetAt`) and retry delays (`retryAfterSeconds`) so backing-off agents are distinct from silent hangs or unhandled errors.
  - **Completed**: Distinctly badge and highlight when an agent has successfully completed its turn and is sitting idle awaiting developer review or new input, persisting until acknowledged/dismissed or a new turn begins.
  - **IDLE / DONE**: Distinguish "finished its turn, still running" from "process is gone".
  - **Idle Freshness Dimming**: Active running agents in `IDLE` state (without unacknowledged completion) tier visually by elapsed time since last activity (`fresh` < 15m, `standard` 15m–2h, and `dormant/stale` > 2h dimmed) so dormant idle sessions do not distract from active work.

### Wish 5: AI-Powered Action & Thought Summarization

- **Wish**: Tap into model thoughts and tool calls, using a very cheap LLM to continuously summarize what the agent is doing into a clean, single-sentence status.
- **Details**:
  - Extract the latest assistant thinking excerpt and recent tool invocations.
  - Use a fast, cost-effective model (**Gemini 2.5 Flash**) to generate concise, human-readable action summaries, for example _"Refactoring build pipeline in build.ts"_ or _"Running unit tests for zsh configuration"_.
  - Summarization is debounced to **at most one call per active agent per 12 seconds**, plus turn boundaries and state transitions, to keep API costs negligible.
  - I accept that this sends thinking excerpts, tool names, and file paths to a third-party provider. Command output and tool results must never be included. The whole feature must be switchable off, leaving deterministic status fully working.

### Wish 6: Dedicated Log Ingestion & Live Deep-Dive

- **Wish**: Robust log ingestion that tracks all agent activity without data loss, plus the ability to inspect raw logs on demand.
- **Details**:
  - Ingest raw events from agent session files using persistent byte-offset cursors so restarts do not drop logs.
  - Survive a disconnect between an observer and the central service without losing events.
  - Allow clicking into an agent card to open a slide-over Live Execution Drawer with real-time streaming logs.

### Wish 7: Don't Lie To Me

- **Wish**: The dashboard must never present stale data as live.
- **Details**:
  - A dropped connection shows a disconnected state, not the last known values rendered as current.
  - An agent whose summary could not be refreshed is marked as stale rather than silently showing an old sentence.
  - If an observer dropped events because its buffer overflowed, say so on that environment.

---

## 3. Technology & Architectural Constraints

### 1. Edge Telemetry: `agent-observer` (Go)

- **Role**: Lightweight background daemon on each machine.
- **Language**: **Go**, for minimal memory footprint, single-binary distribution, and native OS process and file monitoring.
- **Function**: Scans local agent processes, resolves git root and worktree context, tails session logs via byte-offset cursors, ships telemetry frames over WebSocket.
- **Terminology**: Must be named **`agent-observer`**, not "collector".

### 2. Stream Processing & Summarization: `agent-ingester` (Go)

- **Role**: Central stream ingestion and summarization engine.
- **Language**: **Go**, for concurrent stream handling, low latency, and efficient SQLite writes.
- **Function**: Independent background service. Receives WebSocket streams from all observers, applies deterministic status rules with no network call on the hot path, queues LLM summarization, persists to SQLite in WAL mode.

### 3. API & Real-Time Streaming: `agent-server` (Go)

- **Role**: Backend HTTP API and real-time event broadcaster.
- **Language**: **Go**.
- **Function**: Runs as an independent process from the ingester. Exposes REST endpoints and broadcasts real-time updates over SSE and WebSockets.
- **Constraint**: **No UI embedding** inside the Go binary.
- **Constraint**: Because it is a separate process, it cannot share an in-process event bus with the ingester, and SQLite offers no cross-process change notification. The two need a real IPC channel; `docs/prd.md` §3.1 specifies it.

### 4. Web Dashboard: `ui-web` (Bun + TypeScript + Tailwind v4)

- **Role**: Real-time web frontend.
- **Technology**: **Bun + TypeScript + React + Tailwind v4**, CSS-first via `@import "tailwindcss";`.
- **Constraint**: **No Vite.** Bun's own bundler and dev server cover the dev loop and the production build; a second toolchain for what Bun does natively is not acceptable.
- **Naming**: Directory named **`ui-web`**.

### 5. Host Platform Support

- **darwin** and **linux** only. Windows is not supported; agent liveness relies on POSIX process semantics.

### 6. Security Posture

- Observers reach the ingester over a **private overlay network** (Tailscale or WireGuard), authenticated with a shared bearer token. The ingester binds the overlay interface, never `0.0.0.0`.
- `agent-server` binds **loopback plus the overlay interface** and requires a bearer token on every endpoint, including the event streams. I want the dashboard from my laptop or phone on the overlay, and from nowhere else.
- Everything on these links — absolute paths, branch names, model thinking, tool inputs, command output — is treated as sensitive.

### 7. Future-Proofing for Alternative Frontends

- Backend streaming APIs stay decoupled and lightweight so alternative frontends, such as an ESP32 with a small e-paper display or a terminal TUI, can consume compact endpoints later.

### 8. Repository Structure

- All four components live in their own packages under `packages/` in the `agent-status` repository:
  - `packages/agent-observer`
  - `packages/agent-ingester`
  - `packages/agent-server`
  - `packages/ui-web`

---

## 4. User Experience & User Journey

### A. Morning / Fleet Check-in

1. I start `agent-ingester` and `agent-server` on my main development machine.
2. On my MacBook and Linux dev boxes, `agent-observer` runs in the background as a launchd or systemd service.
3. I open the Web UI in my browser.
4. The dashboard immediately shows all online environments and active agents grouped by project.

### B. Active Coding & Multitasking

1. I launch three Claude Code agents and one Pi agent across different branches and worktrees on different machines.
2. The dashboard updates in real time:
   - `ai-registry` shows two agents actively running on `work-macbook` with live action summaries.
   - `dotfiles-installer` shows one agent running unit tests on `dev-box-linux`.
3. When an agent calls `AskUserQuestion`, a high-visibility amber banner appears at the top:
   > **dotfiles-installer** (`dev-box-linux` / `feat-zsh`): _"Do you want to enable Oh-My-Zsh plugins by default?"_
4. I answer it in that agent's terminal, and the dashboard transitions the agent back to `BUSY`.

### C. Deep-Dive Debugging

1. When an agent enters `ERROR`, I click its card.
2. A slide-over drawer shows the exact tool call, its arguments, the failing output, and the model's preceding thought block, so I understand what broke without digging through log directories.

---

## 5. Success Criteria

- [ ] **Multi-Box & Multi-Agent**: Aggregates agents running simultaneously across multiple hosts into one dashboard.
- [ ] **Instant Question Detection**: An `AskUserQuestion` prompt reaches the banner with no polling delay on the critical path.
- [ ] **Accurate Worktree Normalization**: Agents in `.workspaces/...` or other linked worktrees group under their parent repository name.
- [ ] **Low False-Positive Errors**: A `grep` with no match and a deliberately failing test do not raise the error banner.
- [ ] **No Data Loss**: Killing and restarting the ingester mid-run loses no events; the observer replays them.
- [ ] **Stable Identity**: A recycled process ID does not merge two unrelated sessions into one card.
- [ ] **High-Quality AI Summaries**: Every active agent card shows an accurate single-sentence summary, marked stale when it could not be refreshed.
- [ ] **Clean Separation of Concerns**: Observer, Ingester, Server (no UI embedding), and UI run as independent, decoupled packages.
- [ ] **Honest Failure**: A disconnected dashboard says so instead of showing stale data.

---

## 6. Deferred

Wanted, not in the first release.

- **Codex and OpenCode** harness support.
- **Permission-prompt detection.** Being blocked on a permission check is as bad as being blocked on a question, but no reliable signal for a _pending_ permission prompt has been found yet. See `docs/prd.md` §11.
- **Answering a question from the dashboard.** For now the dashboard tells me where to go; I answer in the agent's own terminal.
- **Subagents as separate cards.** Sidechain activity streams into the drawer but does not get its own card.
