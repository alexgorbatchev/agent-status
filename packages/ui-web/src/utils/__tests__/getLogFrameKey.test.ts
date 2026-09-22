import { describe, expect, it } from "bun:test";
import { fixture_telemetryFrame } from "./fixtures";
import type { ITelemetryFrame } from "../../types";
import { getLogFrameKey } from "../getLogFrameKey";

describe("getLogFrameKey", () => {
  it("generates a deterministic key without array indices", () => {
    const frame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      agentId: "env-1:pi:session-1",
      seq: 42,
      timestamp: 1700000000000,
      eventType: "tool_call",
      subagentId: null,
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call_abc123",
      },
    };

    const key = getLogFrameKey(frame);
    expect(key).toBe("env-1:pi:session-1-42-1700000000000-tool_call-call_abc123");
    // Verify key remains identical regardless of calls
    expect(getLogFrameKey(frame)).toBe(key);
  });

  it("includes subagentId when present", () => {
    const frame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      agentId: "env-1:claude-code:session-1",
      seq: 10,
      timestamp: 1700000001000,
      eventType: "turn_start",
      subagentId: "sub-worker-9",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: null,
      },
    };

    const key = getLogFrameKey(frame);
    expect(key).toBe("env-1:claude-code:session-1-10-1700000001000-turn_start-sub-worker-9");
  });

  it("handles missing toolCallId and null subagentId", () => {
    const frame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      agentId: "env-2:codex:session-2",
      seq: 5,
      timestamp: 1700000002000,
      eventType: "turn_end",
      subagentId: null,
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: null,
      },
    };

    const key = getLogFrameKey(frame);
    expect(key).toBe("env-2:codex:session-2-5-1700000002000-turn_end");
  });

  it("falls back to 'agent' when agentId is missing or empty", () => {
    const frame = {
      ...fixture_telemetryFrame,
      agentId: "" as unknown as string,
      seq: 1,
      timestamp: 1700000003000,
      eventType: "user_prompt",
      subagentId: null,
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: null,
      },
    } as unknown as ITelemetryFrame;

    const key = getLogFrameKey(frame);
    expect(key).toBe("agent-1-1700000003000-user_prompt");
  });
});
