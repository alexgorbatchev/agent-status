import { describe, it, expect } from "bun:test";
import type {
  LifecycleState,
  ActivityState,
  HarnessType,
  AgentPhase,
  IEnvironmentInfo,
  IWorkspaceInfo,
  IProjectInfo,
  ITokenUsage,
  ICostInfo,
  IRunInfo,
  IAgentActivity,
  IAgentState,
  ITelemetryFrame,
  ObserverMessage,
  IngesterMessage,
  ServerEvent,
} from "../index";

describe("TypeScript Shared Contracts", () => {
  it("verifies all LifecycleState values", () => {
    const states: LifecycleState[] = ["RUNNING", "EXITED"];
    expect(states.length).toBe(2);
  });

  it("verifies all ActivityState values", () => {
    const activities: ActivityState[] = ["BUSY", "WAITING_QUESTION", "ERROR", "IDLE"];
    expect(activities.length).toBe(4);
  });

  it("verifies HarnessType values", () => {
    const harnesses: HarnessType[] = ["claude-code", "pi"];
    expect(harnesses.length).toBe(2);
  });

  it("verifies AgentPhase values", () => {
    const phases: AgentPhase[] = ["planning", "editing", "testing", "debugging", "investigating"];
    expect(phases.length).toBe(5);
  });

  it("constructs full IAgentState hierarchy", () => {
    const env: IEnvironmentInfo = {
      id: "env-1234",
      label: "macbook-pro",
      hostname: "mbp.local",
      platform: "darwin",
      user: "alex",
      isStale: false,
      lastHeartbeatAt: 1789850000000,
    };

    const ws: IWorkspaceInfo = {
      isWorktree: true,
      branch: "feat/telemetry-coverage",
      isDetached: false,
      relPath: ".workspaces/coverage",
      cwd: "/Users/alex/dev/agent-status/.workspaces/coverage",
    };

    const proj: IProjectInfo = {
      id: "github.com/alexgorbatchev/agent-status",
      name: "agent-status",
      canonicalRoot: "/Users/alex/dev/agent-status",
    };

    const usage: ITokenUsage = {
      input: 10000,
      output: 2000,
      cacheRead: 50000,
      cacheWrite: 5000,
      reasoning: 800,
      total: 67800,
    };

    const cost: ICostInfo = {
      currency: "USD",
      total: 0.125,
      isEstimated: false,
    };

    const run: IRunInfo = {
      id: "run-pid-123",
      pid: 12345,
      harnessVersion: "2.1.277",
      startedAt: 1789850000000,
      endedAt: null,
    };

    const activity: IAgentActivity = {
      lifecycle: "RUNNING",
      activity: "BUSY",
      title: "Increase test coverage to 90%+",
      action: "Writing automated tests",
      actionStale: false,
      phase: "testing",
      activeTool: "Bash",
      activeTarget: "bun test --coverage",
      question: null,
      lastThinkingExcerpt: "Checking statement coverage...",
      lastError: null,
      usage,
      cost,
      subagentCount: 3,
      currentRun: run,
      firstStartedAt: 1789840000000,
      updatedAt: 1789850000000,
    };

    const agent: IAgentState = {
      id: "env-1234:claude-code:sess-abc",
      sessionId: "sess-abc",
      harness: "claude-code",
      environment: env,
      project: proj,
      workspace: ws,
      activity,
    };

    expect(agent.id).toBe("env-1234:claude-code:sess-abc");
    expect(agent.environment.platform).toBe("darwin");
    expect(agent.workspace.isWorktree).toBe(true);
    expect(agent.activity.usage.total).toBe(67800);
    expect(agent.activity.cost?.total).toBe(0.125);
    expect(agent.activity.subagentCount).toBe(3);
  });

  it("constructs ITelemetryFrame and transport messages", () => {
    const frame: ITelemetryFrame = {
      version: "1.0",
      seq: 42,
      envId: "env-1",
      environment: {
        id: "env-1",
        label: "mbp",
        hostname: "mbp",
        platform: "darwin",
        user: "alex",
        isStale: false,
        lastHeartbeatAt: 1000,
      },
      agentId: "env-1:claude-code:s1",
      sessionId: "s1",
      subagentId: "sub-1",
      pid: 9999,
      harness: "claude-code",
      harnessVersion: "2.1.277",
      project: {
        id: "github.com/org/repo",
        name: "repo",
        canonicalRoot: "/repo",
      },
      workspace: {
        isWorktree: false,
        branch: "main",
        isDetached: false,
        relPath: null,
        cwd: "/repo",
      },
      timestamp: 2000,
      eventType: "tool_call",
      data: {
        role: "assistant",
        content: null,
        title: "Test Task",
        lastPrompt: "do task",
        model: "claude-opus-5",
        toolName: "Edit",
        toolCallId: "call_1",
        toolInput: { file: "main.go" },
        toolOutput: null,
        isError: null,
        exitCode: null,
        question: null,
        stopReason: null,
        usage: null,
        rawHarnessCost: null,
      },
    };

    const obsMsg: ObserverMessage = {
      type: "frames",
      frames: [frame],
    };

    const ingMsg: IngesterMessage = {
      type: "ack",
      upTo: 42,
    };

    const srvEvent: ServerEvent = {
      type: "log_append",
      at: 3000,
      agentId: frame.agentId,
      event: frame,
    };

    expect(obsMsg.type).toBe("frames");
    expect(ingMsg.type).toBe("ack");
    expect(srvEvent.type).toBe("log_append");
  });
});
