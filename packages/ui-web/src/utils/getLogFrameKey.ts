import type { ITelemetryFrame } from "../types";

/**
 * Returns a stable unique React key for a telemetry frame.
 * The key is constructed from the frame's immutable properties (agentId, seq, timestamp,
 * eventType, subagentId, toolCallId) and intentionally omits array indices to ensure DOM
 * element identity is preserved when frames are prepended during live streaming.
 */
export function getLogFrameKey(frame: ITelemetryFrame): string {
  const toolCallId = frame.data?.toolCallId ? `-${frame.data.toolCallId}` : "";
  const subagent = frame.subagentId ? `-${frame.subagentId}` : "";
  const agentId = frame.agentId || "agent";
  return `${agentId}-${frame.seq}-${frame.timestamp}-${frame.eventType}${subagent}${toolCallId}`;
}
