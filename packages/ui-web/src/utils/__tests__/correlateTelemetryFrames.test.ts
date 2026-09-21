import { describe, it, expect } from "bun:test";
import { correlateTelemetryFrames } from "../correlateTelemetryFrames";
import type { ITelemetryFrame, IEnvironmentInfo, IProjectInfo, IWorkspaceInfo } from "../../types";

const mockEnv: IEnvironmentInfo = {
  id: "env-1",
  label: "local-dev",
  hostname: "macbook",
  platform: "darwin",
  user: "dev",
  isStale: false,
  lastHeartbeatAt: Date.now(),
};

const mockProject: IProjectInfo = {
  id: "proj-1",
  name: "agent-status",
  canonicalRoot: "/Users/dev/agent-status",
};

const mockWorkspace: IWorkspaceInfo = {
  isWorktree: false,
  branch: "main",
  isDetached: false,
  relPath: null,
  cwd: "/Users/dev/agent-status",
};

function createFrame(
  seq: number,
  timestamp: number,
  eventType: ITelemetryFrame["eventType"],
  data: Partial<ITelemetryFrame["data"]> = {},
): ITelemetryFrame {
  return {
    version: "1.0",
    seq,
    envId: "env-1",
    environment: mockEnv,
    agentId: "env-1:claude-code:session-1",
    sessionId: "session-1",
    subagentId: null,
    pid: 1234,
    harness: "claude-code",
    harnessVersion: "1.0.0",
    project: mockProject,
    workspace: mockWorkspace,
    timestamp,
    eventType,
    data: {
      role: null,
      content: null,
      title: null,
      lastPrompt: null,
      model: null,
      toolName: null,
      toolCallId: null,
      toolInput: null,
      toolOutput: null,
      isError: null,
      exitCode: null,
      durationMs: null,
      question: null,
      stopReason: null,
      usage: null,
      rawHarnessCost: null,
      ...data,
    },
  };
}

describe("correlateTelemetryFrames", () => {
  it("returns empty array when input is empty", () => {
    expect(correlateTelemetryFrames([])).toEqual([]);
  });

  it("preserves non-tool frames in original order", () => {
    const frames: ITelemetryFrame[] = [
      createFrame(1, 1000, "run_started"),
      createFrame(2, 1010, "thinking", { content: "Pondering the solution..." }),
      createFrame(3, 1020, "turn_end"),
    ];

    const result = correlateTelemetryFrames(frames);
    expect(result).toHaveLength(3);
    expect(result[0].eventType).toBe("run_started");
    expect(result[1].eventType).toBe("thinking");
    expect(result[2].eventType).toBe("turn_end");
  });

  it("correlates sequential tool_call and tool_result frames in-place", () => {
    const callFrame = createFrame(1, 1000, "tool_call", {
      toolCallId: "call-1",
      toolName: "bash",
      toolInput: { command: "bun test" },
    });
    const resultFrame = createFrame(2, 1450, "tool_result", {
      toolCallId: "call-1",
      toolOutput: "192 pass\n0 fail",
      exitCode: 0,
      isError: false,
    });

    const result = correlateTelemetryFrames([callFrame, resultFrame]);

    // Should merge tool_result into tool_call and drop standalone tool_result frame
    expect(result).toHaveLength(1);
    const merged = result[0];
    expect(merged.eventType).toBe("tool_call");
    expect(merged.data.toolCallId).toBe("call-1");
    expect(merged.data.toolName).toBe("bash");
    expect(merged.data.toolInput).toEqual({ command: "bun test" });
    expect(merged.data.toolOutput).toBe("192 pass\n0 fail");
    expect(merged.data.exitCode).toBe(0);
    expect(merged.data.isError).toBe(false);
    expect(merged.data.durationMs).toBe(450);
  });

  it("handles multiple sequential tool calls and results", () => {
    const frames: ITelemetryFrame[] = [
      createFrame(1, 1000, "tool_call", {
        toolCallId: "call-1",
        toolName: "bash",
        toolInput: { command: "git status" },
      }),
      createFrame(2, 1100, "tool_result", {
        toolCallId: "call-1",
        toolOutput: "nothing to commit",
        exitCode: 0,
        isError: false,
      }),
      createFrame(3, 1200, "tool_call", {
        toolCallId: "call-2",
        toolName: "edit",
        toolInput: { path: "src/App.tsx" },
      }),
      createFrame(4, 1500, "tool_result", {
        toolCallId: "call-2",
        toolOutput: "file modified",
        exitCode: 0,
        isError: false,
      }),
    ];

    const result = correlateTelemetryFrames(frames);
    expect(result).toHaveLength(2);

    expect(result[0].data.toolCallId).toBe("call-1");
    expect(result[0].data.toolName).toBe("bash");
    expect(result[0].data.toolOutput).toBe("nothing to commit");
    expect(result[0].data.durationMs).toBe(100);

    expect(result[1].data.toolCallId).toBe("call-2");
    expect(result[1].data.toolName).toBe("edit");
    expect(result[1].data.toolOutput).toBe("file modified");
    expect(result[1].data.durationMs).toBe(300);
  });

  it("correlates concurrent/interleaved tool calls and results correctly", () => {
    const call1 = createFrame(1, 1000, "tool_call", {
      toolCallId: "call-A",
      toolName: "fetch",
      toolInput: { url: "https://example.com/a" },
    });
    const call2 = createFrame(2, 1050, "tool_call", {
      toolCallId: "call-B",
      toolName: "fetch",
      toolInput: { url: "https://example.com/b" },
    });
    const result1 = createFrame(3, 1300, "tool_result", {
      toolCallId: "call-A",
      toolOutput: "response A",
      exitCode: 0,
      isError: false,
    });
    const result2 = createFrame(4, 1400, "tool_result", {
      toolCallId: "call-B",
      toolOutput: "response B",
      exitCode: 0,
      isError: false,
    });

    const result = correlateTelemetryFrames([call1, call2, result1, result2]);
    expect(result).toHaveLength(2);

    expect(result[0].data.toolCallId).toBe("call-A");
    expect(result[0].data.toolOutput).toBe("response A");
    expect(result[0].data.durationMs).toBe(300);

    expect(result[1].data.toolCallId).toBe("call-B");
    expect(result[1].data.toolOutput).toBe("response B");
    expect(result[1].data.durationMs).toBe(350);
  });

  it("preserves intervening non-tool frames between tool_call and tool_result", () => {
    const call = createFrame(1, 1000, "tool_call", {
      toolCallId: "call-1",
      toolName: "bash",
      toolInput: { command: "sleep 1" },
    });
    const thinking = createFrame(2, 1200, "thinking", {
      content: "Waiting for background process...",
    });
    const toolRes = createFrame(3, 1500, "tool_result", {
      toolCallId: "call-1",
      toolOutput: "done",
      exitCode: 0,
      isError: false,
    });

    const result = correlateTelemetryFrames([call, thinking, toolRes]);
    expect(result).toHaveLength(2);

    expect(result[0].eventType).toBe("tool_call");
    expect(result[0].data.toolOutput).toBe("done");
    expect(result[0].data.durationMs).toBe(500);

    expect(result[1].eventType).toBe("thinking");
    expect(result[1].data.content).toBe("Waiting for background process...");
  });

  it("preserves orphaned tool_result frames with fallback rendering intact", () => {
    const orphanResult = createFrame(1, 1000, "tool_result", {
      toolCallId: "call-unknown",
      toolOutput: "orphaned output from missing call frame",
      exitCode: 1,
      isError: true,
    });

    const result = correlateTelemetryFrames([orphanResult]);
    expect(result).toHaveLength(1);
    expect(result[0].eventType).toBe("tool_result");
    expect(result[0].data.toolCallId).toBe("call-unknown");
    expect(result[0].data.toolOutput).toBe("orphaned output from missing call frame");
    expect(result[0].data.isError).toBe(true);
  });

  it("preserves in-flight tool_call frames that do not have a matching result yet", () => {
    const inFlightCall = createFrame(1, 1000, "tool_call", {
      toolCallId: "call-in-flight",
      toolName: "bash",
      toolInput: { command: "long-running-build" },
    });

    const result = correlateTelemetryFrames([inFlightCall]);
    expect(result).toHaveLength(1);
    expect(result[0].eventType).toBe("tool_call");
    expect(result[0].data.toolCallId).toBe("call-in-flight");
    expect(result[0].data.toolOutput).toBeNull();
    expect(result[0].data.exitCode).toBeNull();
    expect(result[0].data.durationMs).toBeNull();
  });

  it("handles frames without toolCallId gracefully without false correlation", () => {
    const callWithoutId = createFrame(1, 1000, "tool_call", {
      toolCallId: null,
      toolName: "legacy_tool",
    });
    const resultWithoutId = createFrame(2, 1100, "tool_result", {
      toolCallId: null,
      toolOutput: "legacy result",
    });

    const result = correlateTelemetryFrames([callWithoutId, resultWithoutId]);
    expect(result).toHaveLength(2);
    expect(result[0].eventType).toBe("tool_call");
    expect(result[1].eventType).toBe("tool_result");
  });

  it("correlates reverse-chronological frames (newest-first, as delivered to LogDrawer)", () => {
    const resultFrame = createFrame(2, 2000, "tool_result", {
      toolCallId: "call-rev",
      toolOutput: "reverse test passed",
      exitCode: 0,
      isError: false,
    });
    const callFrame = createFrame(1, 1200, "tool_call", {
      toolCallId: "call-rev",
      toolName: "bash",
      toolInput: { command: "bun test" },
    });

    // In reverse-chronological list: [resultFrame, callFrame]
    const result = correlateTelemetryFrames([resultFrame, callFrame]);

    // Merged into the tool_call frame in-place; standalone result frame omitted
    expect(result).toHaveLength(1);
    expect(result[0].eventType).toBe("tool_call");
    expect(result[0].data.toolName).toBe("bash");
    expect(result[0].data.toolOutput).toBe("reverse test passed");
    expect(result[0].data.durationMs).toBe(800);
  });

  it("propagates usage and rawHarnessCost from result frame if call frame lacked them", () => {
    const call = createFrame(1, 1000, "tool_call", {
      toolCallId: "call-usage",
      toolName: "query",
      usage: null,
      rawHarnessCost: null,
    });
    const res = createFrame(2, 1100, "tool_result", {
      toolCallId: "call-usage",
      toolOutput: "query result",
      usage: { input: 50, output: 25, cacheRead: 0, cacheWrite: 0, reasoning: 0, total: 75 },
      rawHarnessCost: 0.0025,
    });

    const result = correlateTelemetryFrames([call, res]);
    expect(result).toHaveLength(1);
    expect(result[0].data.usage).toEqual({
      input: 50,
      output: 25,
      cacheRead: 0,
      cacheWrite: 0,
      reasoning: 0,
      total: 75,
    });
    expect(result[0].data.rawHarnessCost).toBe(0.0025);
  });

  it("handles repeated toolCallIds across multiple calls and results", () => {
    const call1 = createFrame(1, 1000, "tool_call", {
      toolCallId: "call-dup",
      toolName: "bash",
      toolInput: { command: "cmd 1" },
    });
    const call2 = createFrame(2, 1050, "tool_call", {
      toolCallId: "call-dup",
      toolName: "bash",
      toolInput: { command: "cmd 2" },
    });
    const res1 = createFrame(3, 1200, "tool_result", {
      toolCallId: "call-dup",
      toolOutput: "output 1",
    });
    const res2 = createFrame(4, 1300, "tool_result", {
      toolCallId: "call-dup",
      toolOutput: "output 2",
    });

    const result = correlateTelemetryFrames([call1, call2, res1, res2]);
    expect(result).toHaveLength(2);
    expect(result[0].data.toolOutput).toBe("output 1");
    expect(result[1].data.toolOutput).toBe("output 2");
  });
});
