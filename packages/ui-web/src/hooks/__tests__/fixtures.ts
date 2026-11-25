import type { IAgentState, IEnvironmentInfo } from "../../types";

export const fixture_eventStreamEnvironment1: IEnvironmentInfo = {
  id: "env-1",
  label: "macbook-pro",
  hostname: "mbp.local",
  platform: "darwin",
  user: "alex",
  isStale: false,
  lastHeartbeatAt: 1789850000000,
};

export const fixture_eventStreamEnvironment2: IEnvironmentInfo = {
  id: "env-2",
  label: "linux-server",
  hostname: "srv1.internal",
  platform: "linux",
  user: "ubuntu",
  isStale: false,
  lastHeartbeatAt: 1789850000000,
};

export const fixture_eventStreamAgent1: IAgentState = {
  id: "env-1:claude-code:session-1",
  sessionId: "session-1",
  harness: "claude-code",
  environment: fixture_eventStreamEnvironment1,
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
    title: "Implement feature A",
    action: "Writing code",
    actionStale: false,
    phase: "editing",
    activeTool: "Edit",
    activeTarget: "file.ts",
    question: null,
    lastThinkingExcerpt: null,
    lastError: null,
    usage: {
      input: 100,
      output: 50,
      cacheRead: 200,
      cacheWrite: 50,
      reasoning: 0,
      total: 400,
    },
    cost: {
      currency: "USD",
      total: 0.01,
      isEstimated: false,
    },
    subagentCount: 0,
    currentRun: null,
    firstStartedAt: 1789850000000,
    updatedAt: 1789850000000,
  },
};

export const fixture_eventStreamAgent1Updated: IAgentState = {
  ...fixture_eventStreamAgent1,
  activity: {
    ...fixture_eventStreamAgent1.activity,
    title: "Implement feature A (Completed)",
    lifecycle: "EXITED",
    activity: "IDLE",
    action: "Finished all tasks",
  },
};

export const fixture_eventStreamAgent2: IAgentState = {
  id: "env-2:pi:session-2",
  sessionId: "session-2",
  harness: "pi",
  environment: fixture_eventStreamEnvironment2,
  project: {
    id: "github.com/alexgorbatchev/agent-status",
    name: "agent-status",
    canonicalRoot: "/home/ubuntu/agent-status",
  },
  workspace: {
    isWorktree: false,
    branch: "main",
    isDetached: false,
    relPath: null,
    cwd: "/home/ubuntu/agent-status",
  },
  activity: {
    lifecycle: "RUNNING",
    activity: "WAITING_QUESTION",
    title: "Run database migration",
    action: "Waiting for confirmation",
    actionStale: false,
    phase: "planning",
    activeTool: null,
    activeTarget: null,
    question: "Proceed with table drops?",
    lastThinkingExcerpt: null,
    lastError: null,
    usage: {
      input: 500,
      output: 100,
      cacheRead: 1000,
      cacheWrite: 200,
      reasoning: 0,
      total: 1800,
    },
    cost: {
      currency: "USD",
      total: 0.02,
      isEstimated: false,
    },
    subagentCount: 0,
    currentRun: null,
    firstStartedAt: 1789850000000,
    updatedAt: 1789850000000,
  },
};
