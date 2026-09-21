import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import { LogFrameItem } from "../components/LogFrameItem";
import type { ITelemetryFrame } from "../types/types";
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

  it("preserves each new line in markdown content as line break", () => {
    const frameWithNewlines = {
      ...fixture_telemetryFrame,
      data: {
        ...fixture_telemetryFrame.data,
        content: "First line of log\nSecond line of log\nThird line of log",
      },
    };
    const html = renderToString(<LogFrameItem frame={frameWithNewlines} />);

    expect(html).toContain('data-testid="LogFrameItem--content"');
    expect(html).toContain("First line of log");
    expect(html).toContain("Second line of log");
    expect(html).toContain("Third line of log");
    expect(html).toContain("<br/>");
  });

  it("applies min-w-0, max-w-full, and overflow-hidden to frame container", () => {
    const html = renderToString(<LogFrameItem frame={fixture_telemetryFrame} />);

    expect(html).toMatch(/data-testid="LogFrameItem"[^>]*class="[^"]*min-w-0[^"]*"/);
    expect(html).toMatch(/data-testid="LogFrameItem"[^>]*class="[^"]*max-w-full[^"]*"/);
    expect(html).toMatch(/data-testid="LogFrameItem"[^>]*class="[^"]*overflow-hidden[^"]*"/);
  });

  it("configures markdown code components with min-w-0, max-w-full, and overflow-wrap", () => {
    const markdownPayload = {
      ...fixture_telemetryFrame,
      data: {
        ...fixture_telemetryFrame.data,
        content: "```sh\necho 'very_long_unbroken_code_string_testing_containment'\n```",
      },
    };
    const html = renderToString(<LogFrameItem frame={markdownPayload} />);

    expect(html).toMatch(
      /<pre[^>]*class="[^"]*min-w-0[^"]*max-w-full[^"]*whitespace-pre-wrap[^"]*\[overflow-wrap:anywhere\][^"]*break-all[^"]*"/,
    );
    expect(html).toMatch(
      /<code[^>]*class="[^"]*min-w-0[^"]*max-w-full[^"]*\[overflow-wrap:anywhere\][^"]*break-all[^"]*"/,
    );
  });

  it("renders markdown table in an overflow containment wrapper", () => {
    const tableMarkdownPayload = {
      ...fixture_telemetryFrame,
      data: {
        ...fixture_telemetryFrame.data,
        content: "| Service | Status |\n|---|---|\n| API | Operational |\n| DB | Connected |",
      },
    };
    const html = renderToString(<LogFrameItem frame={tableMarkdownPayload} />);

    expect(html).toContain('class="min-w-0 max-w-full overflow-x-auto my-2"');
    expect(html).toMatch(/<table[^>]*class="[^"]*min-w-0[^"]*max-w-full[^"]*\[overflow-wrap:anywhere\][^"]*"/);
    expect(html).toContain("Service");
    expect(html).toContain("Operational");
    expect(html).toContain("Connected");
  });

  it("renders icons and styles for run_started, turn_end, and error event types", () => {
    const runStartedFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      eventType: "run_started",
    };
    const turnEndFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      eventType: "turn_end",
    };
    const errorFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      eventType: "error",
    };

    const runStartedHtml = renderToString(<LogFrameItem frame={runStartedFrame} />);
    expect(runStartedHtml).toContain("run_started");

    const turnEndHtml = renderToString(<LogFrameItem frame={turnEndFrame} />);
    expect(turnEndHtml).toContain("turn_end");

    const errorHtml = renderToString(<LogFrameItem frame={errorFrame} />);
    expect(errorHtml).toContain("error");

    const runExitedFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      eventType: "run_exited",
    };
    const runExitedHtml = renderToString(<LogFrameItem frame={runExitedFrame} />);
    expect(runExitedHtml).toContain("run_exited");
  });

  it("renders question prompt with min-w-0 and max-w-full containment", () => {
    const questionFrame = {
      ...fixture_telemetryFrame,
      data: {
        ...fixture_telemetryFrame.data,
        content: null,
        question: "```sh\necho 'unbroken_question_code_block'\n```",
      },
    };
    const html = renderToString(<LogFrameItem frame={questionFrame} />);

    expect(html).toContain('data-testid="LogFrameItem--question"');
    expect(html).toMatch(/data-testid="LogFrameItem--question"[^>]*class="[^"]*min-w-0[^"]*"/);
    expect(html).toMatch(/data-testid="LogFrameItem--question"[^>]*class="[^"]*max-w-full[^"]*"/);
    expect(html).toMatch(
      /<pre[^>]*class="[^"]*min-w-0[^"]*max-w-full[^"]*\[overflow-wrap:anywhere\][^"]*break-all[^"]*"/,
    );
  });

  it("passes exitCode and isError from frame.data to ToolExecutionBlock", () => {
    const errorFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      data: {
        ...fixture_telemetryFrame.data,
        toolName: "Bash",
        toolOutput: "Command failed",
        exitCode: 1,
        isError: true,
      },
    };
    const html = renderToString(<LogFrameItem frame={errorFrame} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--status"');
    expect(html).toContain("exit 1");
    expect(html).toContain('data-testid="ToolExecutionBlock--stream-type"');
    expect(html).toContain("STDERR");
  });
});
