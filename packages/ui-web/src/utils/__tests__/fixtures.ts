import type { IAgentState, IEnvironmentInfo } from "../../types";

export const fixture_testEnvironment: IEnvironmentInfo = {
  id: "env-test-1",
  label: "test-workstation",
  hostname: "test.local",
  platform: "darwin",
  user: "tester",
  isStale: false,
  lastHeartbeatAt: 1789850000000,
};

export const fixture_testStaleEnvironment: IEnvironmentInfo = {
  id: "env-test-2",
  label: "stale-workstation",
  hostname: "stale.local",
  platform: "linux",
  user: "ubuntu",
  isStale: true,
  lastHeartbeatAt: 1789840000000,
};

export function factory_createAgent(overrides: Partial<IAgentState> = {}): IAgentState {
  return {
    id: "env-test-1:claude-code:sess-1",
    sessionId: "sess-1",
    harness: "claude-code",
    environment: fixture_testEnvironment,
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
      title: "Active Task",
      action: "Working on task",
      actionStale: false,
      phase: "editing",
      activeTool: "Bash",
      activeTarget: "bun test",
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
        total: 0.05,
        isEstimated: false,
      },
      subagentCount: 0,
      currentRun: {
        id: "run-1",
        pid: 1234,
        harnessVersion: "1.0.0",
        startedAt: 1789850000000,
        endedAt: null,
      },
      firstStartedAt: 1789850000000,
      updatedAt: 1789850000000,
    },
    ...overrides,
  };
}
