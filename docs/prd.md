---
created_on: 2026-09-19 02:00
last_modified: 2026-09-19 02:00
status: current
---

# Product Requirements Document & Architecture Specification: Agent Status

## 1. Executive Summary

`agent-status` is a distributed, multi-environment observability system designed to track, classify, summarize, and display the real-time execution status of AI coding agents across multiple host machines, repositories, and git worktrees.

When running multiple concurrent coding agents (e.g. Claude Code, Pi, OpenCode, Codex) across different development machines (e.g. MacBook, remote Linux dev boxes), engineers lack a centralized pane of glass to know:
1. Which agents are running and in which environments.
2. What project, branch, or git worktree each agent is operating on.
3. What specific task the agent is performing (summarized from thought logs and tool executions).
4. When an agent is blocked waiting for user interaction (e.g. `AskUserQuestion` prompts).
5. When an agent has encountered an error or successfully finished its run.

`agent-status` solves this via a decoupled four-package architecture:
- **`packages/agent-observer`** (Go background daemon on each host): Discovers local agent processes, tracks byte-offset cursors across session logs, normalizes git repository and worktree metadata, and streams structured telemetry over WebSocket.
- **`packages/agent-ingester`** (Go background stream processor): Ingests telemetry frames from observers across all environments, runs zero-latency deterministic state classification, batches turn deltas for cheap AI action summarization (via Gemini 2.5 Flash), and persists structured events into SQLite (WAL mode).
- **`packages/agent-server`** (Go API and real-time event broadcaster): Exposes REST query endpoints and Server-Sent Events (SSE) / WebSocket channels for real-time frontend updates. Runs as an independent process from the ingester and does not embed static UI bundles.
- **`packages/ui-web`** (Bun + TypeScript + Tailwind v4 frontend): Responsive web application providing real-time dashboard visualization grouped by Canonical Project or Host Environment, real-time alert banners for blocked agents, and expandable live execution drawers.

---

## 2. System Architecture & Topology

```
┌────────────────────────────────────────┐       ┌────────────────────────────────────────┐
│         HOST A (work-macbook)          │       │         HOST B (dev-box-linux)         │
│  ┌──────────────────────────────────┐  │       │  ┌──────────────────────────────────┐  │
│  │     packages/agent-observer      │  │       │     packages/agent-observer      │  │
│  │ • Process scanner (Claude/Pi/DB) │  │       │ • Process scanner (Claude/Pi/DB) │  │
│  │ • Byte-offset JSONL log tailers  │  │       │ • Byte-offset JSONL log tailers  │  │
│  │ • Git worktree & branch resolver │  │       │ • Git worktree & branch resolver │  │
│  └─────────────────┬────────────────┘  │       │  └─────────────────┬────────────────┘  │
└────────────────────┼───────────────────┘       └────────────────────┼───────────────────┘
                     │                                                │
                     │ WebSocket Telemetry Stream (JSON Frames)       │
                     ▼                                                ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                packages/agent-ingester                                  │
│                               (Central Go Stream Engine)                                │
│                                                                                         │
│  1. Ingestion Gateway: Ingests WebSocket streams from all active observers              │
│  2. Instant State Classifier: 0-ms deterministic rules (Question, Busy, Error, Done)   │
│  3. AI Action Summarizer: Debounced batch queue calling Gemini 2.5 Flash                │
│  4. Data Store: Writes state transitions and structured logs to shared SQLite (WAL)     │
└────────────────────────────────────────────┬────────────────────────────────────────────┘
                                             │
                                             │ Shared SQLite DB & Internal PubSub
                                             ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                 packages/agent-server                                   │
│                              (Central Go HTTP/SSE Server)                               │
│                                                                                         │
│  1. REST API: GET /api/v1/projects, GET /api/v1/environments, GET /api/v1/agents       │
│  2. Real-Time Broadcaster: SSE (/api/v1/events) and WebSocket (/ws/events) for UI       │
│  3. Log Query API: Historical log search and live tail endpoints                        │
└────────────────────────────────────────────┬────────────────────────────────────────────┘
                                             │
                                             │ REST API + SSE / WebSocket Event Stream
                                             ▼
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                                   packages/ui-web                                       │
│                           (Bun + TypeScript + Tailwind v4)                              │
│                                                                                         │
│  • View by Project (multi-host aggregation) & View by Environment                       │
│  • Instant Action-Required Alert Banners (Question pending, errors)                     │
│  • Live Agent Cards (active tool, target file, AI task summary, elapsed time)           │
│  • Live Execution Drawer (streaming thought excerpts, tool calls, and outputs)         │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Package Responsibilities & Directory Layout

The `agent-status` repository contains four distinct packages under `packages/`:

```
agent-status/
├── docs/
│   └── prd.md                     # This specification
├── packages/
│   ├── agent-observer/            # [Go] Edge telemetry observer daemon
│   │   ├── bin/                   # Git-ignored binary output (air-observer)
│   │   ├── cmd/
│   │   │   └── air-observer/
│   │   │       └── main.go        # Cobra CLI entrypoint
│   │   ├── internal/
│   │   │   ├── config/            # CLI flags, env var resolution, config loader
│   │   │   ├── git/               # Canonical git root, worktree, branch resolver
│   │   │   ├── scanner/           # Process scanner for Claude Code, Pi, OpenCode
│   │   │   ├── tailer/            # Byte-offset JSONL and SQLite WAL tailers
│   │   │   └── shipper/           # Resilient auto-reconnecting WebSocket client
│   │   ├── go.mod
│   │   ├── go.sum
│   │   └── justfile
│   │
│   ├── agent-ingester/            # [Go] Central stream processing and summarizer
│   │   ├── bin/                   # Git-ignored binary output (air-ingester)
│   │   ├── cmd/
│   │   │   └── air-ingester/
│   │   │       └── main.go        # Cobra CLI entrypoint
│   │   ├── internal/
│   │   │   ├── config/            # Ingester configuration (DB path, LLM API keys)
│   │   │   ├── receiver/          # WebSocket listener for Observer connections
│   │   │   ├── classifier/        # 0-ms deterministic state heuristic rules
│   │   │   ├── summarizer/        # Gemini 2.5 Flash / Haiku debounced LLM pipeline
│   │   │   └── store/             # SQLite database layer (WAL mode)
│   │   ├── go.mod
│   │   ├── go.sum
│   │   └── justfile
│   │
│   ├── agent-server/              # [Go] Central REST API and SSE broadcaster
│   │   ├── bin/                   # Git-ignored binary output (air-server)
│   │   ├── cmd/
│   │   │   └── air-server/
│   │   │       └── main.go        # Cobra CLI entrypoint
│   │   ├── internal/
│   │   │   ├── config/            # Server configuration (port, DB path, CORS)
│   │   │   ├── api/               # REST API route handlers
│   │   │   ├── stream/            # SSE and WebSocket real-time broadcast hub
│   │   │   └── store/             # Read-only SQLite query layer
│   │   ├── go.mod
│   │   ├── go.sum
│   │   └── justfile
│   │
│   └── ui-web/                    # [Bun + TS] Web Dashboard Application
│       ├── src/
│       │   ├── App.tsx            # Main application root
│       │   ├── main.tsx           # Entrypoint
│       │   ├── theme.css          # Tailwind v4 entry stylesheet (@import "tailwindcss";)
│       │   ├── components/        # React components
│       │   │   ├── Header.tsx
│       │   │   ├── ProjectGroup.tsx
│       │   │   ├── EnvironmentGroup.tsx
│       │   │   ├── AgentCard.tsx
│       │   │   ├── QuestionBanner.tsx
│       │   │   ├── StatusBadge.tsx
│       │   │   └── LogDrawer.tsx
│       │   ├── hooks/
│       │   │   └── useEventStream.ts
│       │   └── types/
│       │       └── index.ts       # Shared TypeScript schemas
│       ├── package.json
│       ├── tsconfig.json
│       └── justfile
├── .gitignore
└── justfile                       # Root automation (build-all, test-all)
```

---

## 4. Detailed Component Specifications

### 4.1. `packages/agent-observer` (Edge Observer Daemon)

#### Objective
Runs as a lightweight background daemon on every developer machine or remote VM. Discovers active agents, tracks their session files, resolves git repository and worktree context, and ships raw events to the Central Ingester over WebSocket.

#### Key Features
1. **Environment Identity Resolution**:
   - Order of precedence:
     1. `--env <name>` CLI flag (e.g. `--env work-macbook`)
     2. `AI_ENV` environment variable
     3. Local config file `~/.config/ai-registry/telemetry.json` (`{ "environment": "work-macbook" }`)
     4. OS hostname fallback (`os.Hostname()`)
2. **Process Scanner**:
   - **Claude Code**: Reads active session descriptors from `${XDG_DATA_HOME:-~/.local/share}/ai-registry/claude-code/sessions/*.json` and verifies PID liveness using `syscall.Kill(pid, 0)`.
   - **Pi**: Scans running `pi` processes and maps active sessions under `${PI_CODING_AGENT_DIR:-~/.pi/agent}/sessions/--<encoded-cwd>--/*.jsonl`.
   - **OpenCode**: Monitors `${XDG_DATA_HOME:-~/.local/share}/opencode/opencode.db` for recently updated session rows.
3. **Git Workspace & Worktree Resolver**:
   - Executes `git rev-parse --show-toplevel` and `git rev-parse --git-common-dir` on agent working directory (`cwd`).
   - Normalizes canonical project name from the top-level git repository root.
   - Detects whether `cwd` is a linked worktree (e.g. inside `.workspaces/<branch>` or `.claude/worktrees/<name>`).
   - Extracts active branch with `git symbolic-ref --short HEAD`.
4. **Byte-Offset Log Tailer**:
   - Watches `.jsonl` session files using `fsnotify` with a 500ms polling fallback.
   - Maintains byte offset cursors in `${XDG_CACHE_HOME:-~/.cache}/agent-status/observer-cursors.json`.
   - On daemon restart or system resume, resumes tailing from the recorded byte offset. Detects file truncation or inode changes and resets offset cleanly.
5. **Shipper & Resilience**:
   - Maintains an in-memory ring buffer (up to 1,000 unacknowledged events).
   - Auto-reconnects to the Ingester with exponential backoff ($500\text{ms} \to 30\text{s}$).
   - Flushes batched telemetry frames every $200\text{ms}$ or when the batch reaches 50 events.

---

### 4.2. `packages/agent-ingester` (Central Stream Engine)

#### Objective
Acts as the central stream ingestion gateway, running continuously as a background service. It ingests events from all connected observers, classifies agent states instantly, computes AI summaries on debounced turn boundaries, and writes structured records to SQLite.

#### Key Features
1. **WebSocket Ingestion Gateway**:
   - Listens on `:4501/ws/ingest` for authenticated Observer connections.
   - Handles multi-environment multiplexing.
2. **Deterministic State Classifier (Tier 1: 0-ms Heuristics)**:
   - Evaluates incoming frames instantly with zero external API calls:
     - If tool call is `AskUserQuestion` or interactive stdin prompt $\to$ Set status to `WAITING_QUESTION`, extract question text.
     - If tool call is executing (`Bash`, `Edit`, `Read`, `Write`, etc.) $\to$ Set status to `BUSY`, record `activeTool` and `activeTarget`.
     - If tool result returned a non-zero exit code or refusal $\to$ Set status to `ERROR` and attach error message.
     - If assistant message finished with `stop_reason: end_turn` and no tools running $\to$ Set status to `IDLE` or `DONE`.
     - If observer emits agent exit event $\to$ Set status to `DONE`.
3. **AI Action Summarizer (Tier 2: Debounced LLM Pipeline)**:
   - Model: **Gemini 2.5 Flash** (via direct API or LiteLLM proxy) with fallback to Claude 3.5 Haiku.
   - Triggering:
     - On agent turn boundary (`turn_end`).
     - On transition to `WAITING_QUESTION` or `ERROR`.
     - Debounced during long tool sequences (maximum once every $12\text{--}15$ seconds per active agent).
   - Prompt Input:
     - Initial agent prompt / task goal.
     - Recent 3 tool invocations and target files.
     - Recent model thinking excerpt (truncated to 300 tokens).
   - Structured Output:
     ```json
     {
       "summary": "Fixing TypeScript types in build command",
       "phase": "debugging",
       "activeTarget": "packages/registry-cli/src/commands/buildCommand.ts"
     }
     ```
4. **SQLite Persistence Engine**:
   - Database location: `${XDG_DATA_HOME:-~/.local/share}/agent-status/agent-status.db`.
   - Configured with `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000;`.
   - Schema tracks:
     - `agents` (id, environment, project_name, canonical_root, branch, is_worktree, cwd, status, active_tool, active_target, question, summary, phase, started_at, updated_at).
     - `telemetry_events` (id, agent_id, timestamp, event_type, payload_json).

---

### 4.3. `packages/agent-server` (Central HTTP API & SSE Broadcaster)

#### Objective
Provides a dedicated, non-blocking HTTP and streaming server. Reads state from the shared SQLite database and internal event bus, serving queries and broadcasting real-time updates to web clients.

#### Key Features
1. **REST Endpoints**:
   - `GET /api/v1/status`: Health check and system stats (total agents, active environments).
   - `GET /api/v1/projects`: Returns all canonical projects with their active and recent agents.
   - `GET /api/v1/environments`: Returns all connected environments with active agent counts.
   - `GET /api/v1/agents`: List all agents with filtering query params (`?environment=...&project=...&status=...`).
   - `GET /api/v1/agents/:id`: Detailed agent state including full timeline.
   - `GET /api/v1/agents/:id/logs`: Paginated historical log query (`?limit=100&offset=0`).
2. **Real-Time Streaming**:
   - `GET /api/v1/events` (Server-Sent Events) and `GET /ws/events` (WebSocket):
     - Broadcasts agent status changes, new tool calls, questions, and summary updates in real time to connected web clients.
   - `GET /api/v1/agents/:id/live-tail`: Dedicated SSE stream providing real-time raw stdout/stderr and thought blocks for a single agent.
3. **CORS & Middleware**:
   - Configurable CORS for local development (`http://localhost:5173` or custom origin).

---

### 4.4. `packages/ui-web` (Web Dashboard Frontend)

#### Objective
A fast, reactive Bun-powered single-page application built with React, TypeScript, and Tailwind v4. Displays multi-machine agent fleet status with zero refresh lag.

#### Key Features
1. **Fleet Overview Header**:
   - Total active agents, environments online, and count of agents requiring attention.
2. **Action-Required Alert Banner**:
   - Sticky top banner displaying any agent in `WAITING_QUESTION` or `ERROR` state.
   - Clicking an alert jumps immediately to the agent card and opens the question prompt.
3. **Dual Grouping Views**:
   - **Group by Project** (Default): Aggregates all agents working on the same codebase across different machines and worktrees.
   - **Group by Environment**: Shows machines (e.g. `work-macbook`, `dev-box-linux`) and the processes running on each.
4. **Agent Card**:
   - **Status Badge**: `BUSY` (blue pulse), `WAITING_QUESTION` (amber alert), `ERROR` (red), `IDLE` (slate), `DONE` (emerald).
   - **Metadata Row**: Environment badge (`🏷️ work-macbook`), Git Branch (`🌿 feat-oauth`), Worktree indicator (`📁 .workspaces/feat-oauth`), Harness tag (`Claude Code` / `Pi` / `OpenCode`).
   - **AI Action Summary**: Single-sentence high-level summary of what the agent is actively doing.
   - **Active Tool & Target**: e.g., `Edit packages/registry-cli/src/commands/buildCommand.ts`.
   - **Elapsed & Active Time**: Real-time ticker.
5. **Live Execution Drawer**:
   - Slide-over drawer when clicking an agent card.
   - Displays real-time streaming timeline of thought excerpts, tool invocations, and truncated command outputs.

---

## 5. Shared Data Contracts & Protocol Types

### 5.1. Common TypeScript / Go Schema

```typescript
export type AgentStatus = "BUSY" | "WAITING_QUESTION" | "ERROR" | "IDLE" | "DONE";

export type HarnessType = "claude-code" | "pi" | "opencode" | "codex";

export interface IEnvironmentInfo {
  name: string;        // e.g. "work-macbook", "dev-box-linux"
  hostname: string;    // e.g. "Alexs-MacBook-Pro.local"
  platform: "darwin" | "linux" | "win32";
  user: string;
}

export interface IWorkspaceInfo {
  isWorktree: boolean;
  branch: string;       // e.g. "main", "feat-dashboard"
  relPath?: string;     // e.g. ".workspaces/feat-dashboard"
  cwd: string;          // absolute working directory
}

export interface IProjectInfo {
  name: string;          // e.g. "ai-registry"
  canonicalRoot: string; // e.g. "/Users/agorbatchev/development/ai-registry"
}

export interface IAgentActivity {
  status: AgentStatus;
  summary: string;           // LLM: "Refactoring build pipeline in build.ts"
  phase?: string;             // "editing" | "testing" | "planning" | "investigating"
  activeTool?: string;        // "Edit", "Bash", "Read", etc.
  activeTarget?: string;      // "packages/registry-cli/src/commands/buildCommand.ts"
  question?: string | null;   // Extracted prompt when status === WAITING_QUESTION
  lastThinkingExcerpt?: string;
  lastError?: string | null;
  tokens?: {
    input: number;
    output: number;
    total: number;
  };
  startedAt: number;
  updatedAt: number;
}

export interface IAgentState {
  id: string;                 // Global unique ID: e.g. "work-macbook:claude-90166"
  localId: string;            // Local ID: e.g. "90166"
  pid: number;
  harness: HarnessType;
  environment: IEnvironmentInfo;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  activity: IAgentActivity;
}
```

### 5.2. Telemetry Event Frame (Observer $\to$ Ingester)

```typescript
export interface ITelemetryFrame {
  version: "1.0";
  environment: IEnvironmentInfo;
  agentId: string;
  localId: string;
  pid: number;
  harness: HarnessType;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  timestamp: number;
  eventType: 
    | "agent_started"
    | "agent_exited"
    | "turn_start"
    | "thinking"
    | "tool_call"
    | "tool_result"
    | "question_prompt"
    | "error"
    | "turn_end";
  data: {
    role?: "user" | "assistant" | "system";
    content?: string;
    toolName?: string;
    toolInput?: Record<string, unknown>;
    toolOutput?: string;
    exitCode?: number;
    tokens?: {
      input: number;
      output: number;
      total: number;
    };
  };
}
```

---

## 6. Database Schema (SQLite in WAL Mode)

```sql
CREATE TABLE IF NOT EXISTS environments (
    name TEXT PRIMARY KEY,
    hostname TEXT NOT NULL,
    platform TEXT NOT NULL,
    user TEXT NOT NULL,
    last_heartbeat_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY, -- e.g. "work-macbook:claude-90166"
    local_id TEXT NOT NULL,
    environment_name TEXT NOT NULL REFERENCES environments(name),
    pid INTEGER NOT NULL,
    harness TEXT NOT NULL,
    project_name TEXT NOT NULL,
    canonical_root TEXT NOT NULL,
    branch TEXT NOT NULL,
    is_worktree INTEGER NOT NULL DEFAULT 0,
    cwd TEXT NOT NULL,
    status TEXT NOT NULL, -- 'BUSY', 'WAITING_QUESTION', 'ERROR', 'IDLE', 'DONE'
    summary TEXT NOT NULL DEFAULT '',
    phase TEXT NOT NULL DEFAULT '',
    active_tool TEXT,
    active_target TEXT,
    question TEXT,
    last_thinking_excerpt TEXT,
    last_error TEXT,
    started_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agents_project ON agents(project_name);
CREATE INDEX IF NOT EXISTS idx_agents_environment ON agents(environment_name);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);

CREATE TABLE IF NOT EXISTS telemetry_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    data_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_agent_ts ON telemetry_events(agent_id, timestamp);
```

---

## 7. Implementation Roadmap & Definition of Done

### Phase 1: Go Core & Edge Observer (`packages/agent-observer`)
- [ ] Initialize Go module with Cobra CLI and `cobrahelptree`.
- [ ] Implement `git` resolver (`git rev-parse --show-toplevel`, worktree detection).
- [ ] Implement process scanners for Claude Code, Pi, and OpenCode.
- [ ] Implement JSONL byte-offset log tailers with cursor persistence in `$XDG_CACHE_HOME`.
- [ ] Implement resilient WebSocket shipper client.
- [ ] Unit tests for git resolution, cursor tracking, and log parsing ($\ge 90\%$ test coverage).

### Phase 2: Stream Ingester & Summarizer (`packages/agent-ingester`)
- [ ] Initialize Go module with Cobra CLI.
- [ ] Implement WebSocket ingestion gateway for incoming observer frames.
- [ ] Implement Tier 1 deterministic status classifier (`AskUserQuestion`, tool execution, exit).
- [ ] Implement Tier 2 debounced Gemini 2.5 Flash LLM summarization pipeline.
- [ ] Implement SQLite store in WAL mode with auto-migrations.
- [ ] Unit tests for state machine, heuristic classifiers, and database queries.

### Phase 3: Central HTTP API & Broadcaster (`packages/agent-server`)
- [ ] Initialize Go module with Cobra CLI.
- [ ] Implement REST query endpoints (`/api/v1/projects`, `/api/v1/environments`, `/api/v1/agents`).
- [ ] Implement Server-Sent Events (SSE) and WebSocket broadcasting channels.
- [ ] Implement live-tail SSE endpoint for streaming agent logs.
- [ ] Integration tests verifying live broadcast from Ingester SQLite updates to Server SSE clients.

### Phase 4: Web Dashboard Frontend (`packages/ui-web`)
- [ ] Initialize Bun + Vite + React + TypeScript app.
- [ ] Configure Tailwind v4 with CSS theme directives (`@import "tailwindcss";`).
- [ ] Implement reactive SSE event stream hook (`useEventStream`).
- [ ] Implement Dual Grouping Views (By Project vs. By Environment).
- [ ] Implement Action-Required Alert Banners and live agent cards.
- [ ] Implement Live Execution Drawer with real-time log streaming.

### Phase 5: Verification & End-to-End Testing
- [ ] Launch multiple live Claude Code & Pi agents across different projects and worktrees.
- [ ] Verify instant state transition to `WAITING_QUESTION` when an agent invokes `AskUserQuestion`.
- [ ] Verify accurate AI task summarization via Gemini 2.5 Flash.
- [ ] Verify reconnection resilience when disconnecting and reconnecting an observer.
- [ ] Verify clean non-embedding separation between `agent-server` and `ui-web`.
