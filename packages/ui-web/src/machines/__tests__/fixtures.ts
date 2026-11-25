import type { IAgentState, IEnvironmentInfo, ITelemetryFrame } from "../../types";

export const fixture_machineEnvironment: IEnvironmentInfo = {
  id: "env-1",
  label: "work-macbook",
  hostname: "macbook.local",
  platform: "darwin",
  user: "alex",
  isStale: false,
  lastHeartbeatAt: 1789850000000,
};

export const fixture_machineAgent1: IAgentState = {
  id: "env-1:claude-code:session-1",
  sessionId: "session-1",
  harness: "claude-code",
  environment: fixture_machineEnvironment,
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/test/agent-status",
  },
  workspace: {
    isWorktree: false,
    branch: "main",
    isDetached: false,
    relPath: null,
    cwd: "/test/agent-status",
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
    lastThinkingExcerpt: null,
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
    subagentCount: 0,
    currentRun: null,
    firstStartedAt: 1789850000000,
    updatedAt: 1789850000000,
  },
};

export const fixture_machineAgent2: IAgentState = {
  ...fixture_machineAgent1,
  id: "env-1:claude-code:session-2",
  sessionId: "session-2",
  activity: {
    ...fixture_machineAgent1.activity,
    title: "Implement user authentication flow",
  },
};

export const fixture_machineTelemetryFrame1: ITelemetryFrame = {
  version: "1.0",
  seq: 1,
  envId: "env-1",
  environment: fixture_machineEnvironment,
  agentId: "env-1:claude-code:session-1",
  sessionId: "session-1",
  subagentId: null,
  pid: 12345,
  harness: "claude-code",
  harnessVersion: "2.1.277",
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/test/agent-status",
  },
  workspace: {
    isWorktree: false,
    branch: "main",
    isDetached: false,
    relPath: null,
    cwd: "/test/agent-status",
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

export const fixture_machineTelemetryFrame2: ITelemetryFrame = {
  ...fixture_machineTelemetryFrame1,
  seq: 2,
  agentId: "env-1:claude-code:session-2",
  sessionId: "session-2",
};
