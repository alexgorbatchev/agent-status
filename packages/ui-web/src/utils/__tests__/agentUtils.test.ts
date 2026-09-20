import { describe, it, expect } from "bun:test";
import { isAgentRequiringAttention } from "../agentUtils";
import type { IAgentState, IEnvironmentInfo, IAgentActivity, LifecycleState, ActivityState } from "../../types/types";

interface IMockAgentOverrides {
  lifecycle?: LifecycleState;
  activity?: ActivityState;
  isStale?: boolean;
}

function createMockAgent(overrides?: IMockAgentOverrides): IAgentState {
  const env: IEnvironmentInfo = {
    id: "env-1",
    label: "dev-macbook",
    hostname: "macbook.local",
    platform: "darwin",
    user: "alex",
    isStale: overrides?.isStale ?? false,
    lastHeartbeatAt: Date.now(),
  };

  const activity: IAgentActivity = {
    lifecycle: overrides?.lifecycle ?? "RUNNING",
    activity: overrides?.activity ?? "IDLE",
    title: "Task in progress",
    action: "Executing command",
    actionStale: false,
    phase: "testing",
    activeTool: null,
    activeTarget: null,
    question: null,
    lastThinkingExcerpt: null,
    lastError: null,
    usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, reasoning: 0, total: 0 },
    cost: null,
    subagentCount: 0,
    currentRun: null,
    firstStartedAt: Date.now(),
    updatedAt: Date.now(),
  };

  return {
    id: "env-1:claude-code:sess-1",
    sessionId: "sess-1",
    harness: "claude-code",
    environment: env,
    project: {
      id: "agent-status",
      name: "agent-status",
      canonicalRoot: "/repo",
    },
    workspace: {
      isWorktree: false,
      branch: "main",
      isDetached: false,
      relPath: null,
      cwd: "/repo",
    },
    activity,
  };
}

describe("agentUtils", () => {
  describe("isAgentRequiringAttention", () => {
    it("returns true for RUNNING agent with WAITING_QUESTION and active environment", () => {
      const agent = createMockAgent({
        lifecycle: "RUNNING",
        activity: "WAITING_QUESTION",
        isStale: false,
      });
      expect(isAgentRequiringAttention(agent)).toBe(true);
    });

    it("returns true for RUNNING agent with ERROR and active environment", () => {
      const agent = createMockAgent({
        lifecycle: "RUNNING",
        activity: "ERROR",
        isStale: false,
      });
      expect(isAgentRequiringAttention(agent)).toBe(true);
    });

    it("returns false for RUNNING agent with WAITING_QUESTION when environment is stale", () => {
      const agent = createMockAgent({
        lifecycle: "RUNNING",
        activity: "WAITING_QUESTION",
        isStale: true,
      });
      expect(isAgentRequiringAttention(agent)).toBe(false);
    });

    it("returns false for RUNNING agent with ERROR when environment is stale", () => {
      const agent = createMockAgent({
        lifecycle: "RUNNING",
        activity: "ERROR",
        isStale: true,
      });
      expect(isAgentRequiringAttention(agent)).toBe(false);
    });

    it("returns false for RUNNING agent in BUSY or IDLE activity", () => {
      const busyAgent = createMockAgent({
        lifecycle: "RUNNING",
        activity: "BUSY",
        isStale: false,
      });
      const idleAgent = createMockAgent({
        lifecycle: "RUNNING",
        activity: "IDLE",
        isStale: false,
      });
      expect(isAgentRequiringAttention(busyAgent)).toBe(false);
      expect(isAgentRequiringAttention(idleAgent)).toBe(false);
    });

    it("returns false for EXITED agent even if activity was ERROR or WAITING_QUESTION", () => {
      const exitedError = createMockAgent({
        lifecycle: "EXITED",
        activity: "ERROR",
        isStale: false,
      });
      const exitedQuestion = createMockAgent({
        lifecycle: "EXITED",
        activity: "WAITING_QUESTION",
        isStale: false,
      });
      expect(isAgentRequiringAttention(exitedError)).toBe(false);
      expect(isAgentRequiringAttention(exitedQuestion)).toBe(false);
    });
  });
});
