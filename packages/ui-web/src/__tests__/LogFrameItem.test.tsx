import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import { LogFrameItem } from "../components/LogFrameItem";
import {
  fixture_telemetryFrame,
  fixture_telemetryFrameSubagent,
  fixture_telemetryFrameUserPrompt,
  fixture_telemetryFrameToolResult,
  fixture_telemetryFrameThinkingAnsi,
} from "./fixtures";

describe("LogFrameItem Component", () => {
  it("renders tool_call frame with tool execution block", () => {
    const html = renderToString(<LogFrameItem frame={fixture_telemetryFrame} />);

    expect(html).toContain('data-testid="LogFrameItem"');
    expect(html).toContain('data-testid="LogFrameItem--type"');
    expect(html).toContain("tool_call");
    expect(html).toContain('data-testid="LogFrameItem--content"');
    expect(html).toContain("Executing test suite to verify changes.");
    expect(html).toContain('data-testid="ToolExecutionBlock"');
    expect(html).toContain("Bash");
    expect(html).toContain("bun test");
    expect(html).toContain("1 pass, 0 fail");
  });

  it("renders tool_result frame as tool_result (not user prompt) and displays tool output", () => {
    const html = renderToString(<LogFrameItem frame={fixture_telemetryFrameToolResult} />);

    expect(html).toContain('data-testid="LogFrameItem"');
    expect(html).toContain('data-testid="LogFrameItem--type"');
    // Even if role is "user" (e.g. from Claude Code harness logs), tool_result is not a user prompt
    expect(html).toContain("tool_result");
    expect(html).not.toContain("USER PROMPT");
    // Should render the ToolExecutionBlock with output containing preserved line breaks
    expect(html).toContain('data-testid="ToolExecutionBlock--output"');
    expect(html).toContain("PASS packages/ui-web/src/App.test.tsx");
    expect(html).toContain("Test Suites: 2 passed, 2 total");
  });

  it("renders user prompt frame with USER PROMPT header and content", () => {
    const html = renderToString(<LogFrameItem frame={fixture_telemetryFrameUserPrompt} />);

    expect(html).toContain('data-testid="LogFrameItem"');
    expect(html).toContain('data-testid="LogFrameItem--type"');
    expect(html).toContain("USER PROMPT");
    expect(html).toContain('data-testid="LogFrameItem--content"');
    expect(html).toContain("Please build the new telemetry component.");
  });

  it("renders subagent badge when frame has subagentId", () => {
    const html = renderToString(<LogFrameItem frame={fixture_telemetryFrameSubagent} />);

    expect(html).toContain('data-testid="LogFrameItem--subagent"');
    expect(html).toContain("Subagent: subagent-worker-1");
  });

  it("renders thinking frame with ANSI colors and markdown intact", () => {
    const html = renderToString(<LogFrameItem frame={fixture_telemetryFrameThinkingAnsi} />);

    expect(html).toContain('data-testid="LogFrameItem"');
    expect(html).toContain('data-testid="LogFrameItem--type"');
    expect(html).toContain("thinking");

    // Assert ANSI color codes are rendered as styled elements
    expect(html).toContain('style="color:rgb(138, 190, 183)"');
    expect(html).toContain("Thinking:");
    expect(html).toContain('style="color:rgb(128, 128, 128)"');
    expect(html).toContain("Okay, here&#x27;s my interpretation");

    // Assert markdown elements are rendered
    expect(html).toContain("<strong>");
    expect(html).toContain("Running the Checks");

    // Raw ANSI codes must not appear in output
    expect(html).not.toContain("38;2;138;190;183m");
    expect(html).not.toContain("[39m");
  });
});
