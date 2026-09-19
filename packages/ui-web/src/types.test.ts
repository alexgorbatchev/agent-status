import { describe, it, expect } from "bun:test";
import type { IAgentState, IEnvironmentInfo } from "./types";

describe("Agent State Contracts", () => {
  it("creates valid agent state model", () => {
    const env: IEnvironmentInfo = {
      id: "env-1",
      label: "work-macbook",
      hostname: "macbook.local",
      platform: "darwin",
      user: "alex",
      isStale: false,
      lastHeartbeatAt: Date.now(),
    };

    const agent: IAgentState = {
      id: "env-1:claude-code:session-1",
      sessionId: "session-1",
      harness: "claude-code",
      environment: env,
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
          startedAt: Date.now() - 60000,
          endedAt: null,
        },
        firstStartedAt: Date.now() - 3600000,
        updatedAt: Date.now(),
      },
    };

    expect(agent.id).toBe("env-1:claude-code:session-1");
    expect(agent.activity.lifecycle).toBe("RUNNING");
    expect(agent.activity.usage.total).toBe(7300);
    expect(agent.activity.subagentCount).toBe(2);
  });
});
