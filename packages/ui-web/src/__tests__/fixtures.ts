<<<<<<< HEAD
import type { IAgentState, IEnvironmentInfo, ITelemetryFrame } from "../types";
=======
import type { IAgentState, IEnvironmentInfo } from "../types";
>>>>>>> issue-21

export const fixture_environmentInfo: IEnvironmentInfo = {
  id: "env-1",
  label: "work-macbook",
  hostname: "macbook.local",
  platform: "darwin",
  user: "alex",
  isStale: false,
  lastHeartbeatAt: 1789850000000,
};

<<<<<<< HEAD
export const fixture_staleEnvironmentInfo: IEnvironmentInfo = {
  id: "env-2",
  label: "cloud-builder",
  hostname: "builder.internal",
  platform: "linux",
  user: "ubuntu",
  isStale: true,
  lastHeartbeatAt: 1789849940000,
};

=======
>>>>>>> issue-21
export const fixture_agentStateRunning: IAgentState = {
  id: "env-1:claude-code:session-1",
  sessionId: "session-1",
  harness: "claude-code",
  environment: fixture_environmentInfo,
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/Users/alex/dev/agent-status",
  },
  workspace: {
    isWorktree: false,
    branch: "main",
    isDetached: false,
    relPath: null,
    cwd: "/Users/alex/dev/agent-status",
  },
  activity: {
    lifecycle: "RUNNING",
    activity: "BUSY",
    title: "Build agent-status dashboard",
    action: "Compiling typescript and verifying bundles",
    actionStale: false,
    phase: "editing",
    activeTool: "Bash",
    activeTarget: "bun test",
    question: null,
    lastThinkingExcerpt: "Writing unit tests for frontend components...",
    lastError: null,
    usage: {
      input: 1000,
      output: 200,
      cacheRead: 5000,
      cacheWrite: 1000,
      reasoning: 100,
      total: 7300,
    },
    cost: {
      currency: "USD",
      total: 0.056,
      isEstimated: false,
    },
    subagentCount: 2,
    currentRun: {
      id: "run-1",
      pid: 12345,
      harnessVersion: "2.1.277",
      startedAt: 1789849940000,
      endedAt: null,
    },
    firstStartedAt: 1789846400000,
    updatedAt: 1789850000000,
  },
};

export const fixture_agentStateWaitingQuestion: IAgentState = {
  id: "env-1:claude-code:session-2",
  sessionId: "session-2",
  harness: "claude-code",
  environment: fixture_environmentInfo,
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/Users/alex/dev/agent-status",
  },
  workspace: {
    isWorktree: true,
    branch: "feat/auth",
    isDetached: false,
    relPath: ".workspaces/feat-auth",
    cwd: "/Users/alex/dev/agent-status/.workspaces/feat-auth",
  },
  activity: {
    lifecycle: "RUNNING",
    activity: "WAITING_QUESTION",
    title: "Implement user authentication flow",
    action: "Waiting for user input on database choice",
    actionStale: false,
    phase: "planning",
    activeTool: null,
    activeTarget: null,
    question: "Should we use SQLite or PostgreSQL for auth tokens?",
    lastThinkingExcerpt: null,
    lastError: null,
    usage: {
      input: 500,
      output: 100,
      cacheRead: 2000,
      cacheWrite: 500,
      reasoning: 50,
      total: 3150,
    },
    cost: {
      currency: "USD",
      total: 0.021,
      isEstimated: false,
    },
    subagentCount: 0,
    currentRun: {
      id: "run-2",
      pid: 23456,
      harnessVersion: "2.1.277",
      startedAt: 1789849880000,
      endedAt: null,
    },
    firstStartedAt: 1789849880000,
    updatedAt: 1789850000000,
  },
};

export const fixture_agentStateError: IAgentState = {
  id: "env-1:claude-code:session-3",
  sessionId: "session-3",
  harness: "claude-code",
  environment: fixture_environmentInfo,
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/Users/alex/dev/agent-status",
  },
  workspace: {
    isWorktree: false,
    branch: "main",
    isDetached: false,
    relPath: null,
    cwd: "/Users/alex/dev/agent-status",
  },
  activity: {
    lifecycle: "RUNNING",
    activity: "ERROR",
    title: "Database schema migration",
    action: "Failed to apply migration: syntax error near 'INDEX'",
    actionStale: false,
    phase: "debugging",
    activeTool: "Bash",
    activeTarget: "migrate up",
    question: null,
    lastThinkingExcerpt: null,
    lastError: "Database migration returned exit code 1: syntax error near 'INDEX'",
    usage: {
      input: 800,
      output: 150,
      cacheRead: 3000,
      cacheWrite: 800,
      reasoning: 80,
      total: 4830,
    },
    cost: {
      currency: "USD",
      total: 0.038,
      isEstimated: false,
    },
    subagentCount: 1,
    currentRun: {
      id: "run-3",
      pid: 34567,
      harnessVersion: "2.1.277",
      startedAt: 1789849820000,
      endedAt: null,
    },
    firstStartedAt: 1789849820000,
    updatedAt: 1789850000000,
  },
};
<<<<<<< HEAD

export const fixture_agentStateExited: IAgentState = {
  id: "env-1:pi:session-4",
  sessionId: "session-4",
  harness: "pi",
  environment: fixture_environmentInfo,
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/Users/alex/dev/agent-status",
  },
  workspace: {
    isWorktree: false,
    branch: "main",
    isDetached: false,
    relPath: null,
    cwd: "/Users/alex/dev/agent-status",
  },
  activity: {
    lifecycle: "EXITED",
    activity: "IDLE",
    title: "Refactor telemetry pipeline",
    action: "Completed task successfully",
    actionStale: false,
    phase: null,
    activeTool: null,
    activeTarget: null,
    question: null,
    lastThinkingExcerpt: null,
    lastError: null,
    usage: {
      input: 2000,
      output: 400,
      cacheRead: 8000,
      cacheWrite: 1500,
      reasoning: 200,
      total: 12100,
    },
    cost: {
      currency: "USD",
      total: 0.092,
      isEstimated: false,
    },
    subagentCount: 0,
    currentRun: null,
    firstStartedAt: 1789842800000,
    updatedAt: 1789846400000,
  },
};

export const fixture_telemetryFrame: ITelemetryFrame = {
  version: "1.0",
  seq: 1,
  envId: "env-1",
  environment: fixture_environmentInfo,
  agentId: "env-1:claude-code:session-1",
  sessionId: "session-1",
  subagentId: null,
  pid: 12345,
  harness: "claude-code",
  harnessVersion: "2.1.277",
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/Users/alex/dev/agent-status",
  },
  workspace: {
    isWorktree: false,
    branch: "main",
    isDetached: false,
    relPath: null,
    cwd: "/Users/alex/dev/agent-status",
  },
  timestamp: 1789850000000,
  eventType: "tool_call",
  data: {
    role: "assistant",
    content: "Executing test suite to verify changes.",
    title: "Build agent-status dashboard",
    lastPrompt: "Run tests",
    model: "claude-3-7-sonnet",
    toolName: "Bash",
    toolCallId: "call_123",
    toolInput: { command: "bun test" },
    toolOutput: "1 pass, 0 fail",
    isError: false,
    exitCode: 0,
    question: null,
    stopReason: null,
    usage: null,
    rawHarnessCost: null,
  },
};
=======
>>>>>>> issue-21
