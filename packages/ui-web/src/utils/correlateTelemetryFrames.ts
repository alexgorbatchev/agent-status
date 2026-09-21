import type { ITelemetryFrame } from "../types";

/**
 * Correlates decoupled telemetry frames by pairing `tool_call` and `tool_result` events
 * using their matching `toolCallId`.
 *
 * In agent-status, parsers emit `tool_call` and `tool_result` as separate asynchronous events
 * without duplicating bulky command or file inputs across records.
 *
 * This function merges matched `tool_result` data into the originating `tool_call` frame in-place:
 * - Attaches `toolOutput`, `exitCode`, `isError`, and calculated `durationMs` to the tool call frame.
 * - Omits the redundant standalone `tool_result` frame from the returned list.
 * - Retains orphaned/unmatched `tool_result` frames as-is to preserve fallback rendering.
 * - Preserves non-tool frames in their original relative positions.
 */
export function correlateTelemetryFrames(frames: readonly ITelemetryFrame[]): ITelemetryFrame[] {
  if (frames.length === 0) {
    return [];
  }

  // Group tool_call and tool_result frames by toolCallId
  const callsById = new Map<string, ITelemetryFrame[]>();
  const resultsById = new Map<string, ITelemetryFrame[]>();

  for (const frame of frames) {
    const toolCallId = frame.data.toolCallId;
    if (!toolCallId) {
      continue;
    }

    if (frame.eventType === "tool_call") {
      const list = callsById.get(toolCallId);
      if (list) {
        list.push(frame);
      } else {
        callsById.set(toolCallId, [frame]);
      }
    } else if (frame.eventType === "tool_result") {
      const list = resultsById.get(toolCallId);
      if (list) {
        list.push(frame);
      } else {
        resultsById.set(toolCallId, [frame]);
      }
    }
  }

  // Pair each tool_call with the closest matching tool_result
  const callToResultMap = new Map<ITelemetryFrame, ITelemetryFrame>();
  const matchedResults = new Set<ITelemetryFrame>();

  for (const [toolCallId, calls] of callsById.entries()) {
    const results = resultsById.get(toolCallId);
    if (!results || results.length === 0) {
      continue;
    }

    const availableResults = [...results];
    for (const call of calls) {
      if (availableResults.length === 0) {
        break;
      }

      // Find the result closest in time to the call
      let bestIndex = 0;
      let minDiff = Infinity;

      for (let i = 0; i < availableResults.length; i++) {
        const diff = Math.abs(availableResults[i].timestamp - call.timestamp);
        if (diff < minDiff) {
          minDiff = diff;
          bestIndex = i;
        }
      }

      const matchedResult = availableResults[bestIndex];
      availableResults.splice(bestIndex, 1);
      callToResultMap.set(call, matchedResult);
      matchedResults.add(matchedResult);
    }
  }

  // Assemble output array preserving order of frames
  const correlated: ITelemetryFrame[] = [];

  for (const frame of frames) {
    if (frame.eventType === "tool_call") {
      const matchingResult = callToResultMap.get(frame);
      if (matchingResult) {
        const durationMs = Math.max(0, Math.abs(matchingResult.timestamp - frame.timestamp));
        correlated.push({
          ...frame,
          data: {
            ...frame.data,
            toolOutput: matchingResult.data.toolOutput ?? frame.data.toolOutput,
            exitCode: matchingResult.data.exitCode ?? frame.data.exitCode,
            isError: matchingResult.data.isError ?? frame.data.isError,
            durationMs,
            usage: matchingResult.data.usage ?? frame.data.usage,
            rawHarnessCost: matchingResult.data.rawHarnessCost ?? frame.data.rawHarnessCost,
          },
        });
      } else {
        correlated.push(frame);
      }
    } else if (frame.eventType === "tool_result") {
      if (matchedResults.has(frame)) {
        // Redundant; already merged into matching tool_call frame in-place
        continue;
      }
      // Orphaned tool_result without matching call; retain fallback rendering
      correlated.push(frame);
    } else {
      correlated.push(frame);
    }
  }

  return correlated;
}
