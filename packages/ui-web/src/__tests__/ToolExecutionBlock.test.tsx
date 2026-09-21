import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import { ToolExecutionBlock } from "../components/ToolExecutionBlock";

describe("ToolExecutionBlock Component", () => {
  it("renders tool name, input, and output", () => {
    const html = renderToString(
      <ToolExecutionBlock toolName="Bash" toolInput={{ command: "bun test" }} toolOutput="1 pass, 0 fail" />,
    );

    expect(html).toContain('data-testid="ToolExecutionBlock"');
    expect(html).toContain('data-testid="ToolExecutionBlock--name"');
    expect(html).toContain("Bash");
    expect(html).toContain('data-testid="ToolExecutionBlock--input"');
    expect(html).toContain("bun test");
    expect(html).toContain('data-testid="ToolExecutionBlock--output"');
    expect(html).toContain("1 pass, 0 fail");
  });

  it("renders multi-line output with pre tag and whitespace-pre-wrap preserving line breaks", () => {
    const multilineOutput = "Line 1: starting build\nLine 2: compilation finished\nLine 3: 0 errors";
    const html = renderToString(<ToolExecutionBlock toolName="Build" toolOutput={multilineOutput} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--output"');
    // Output must be rendered inside a pre element with whitespace-pre-wrap to avoid collapsing into a wall of text
    expect(html).toContain("<pre");
    expect(html).toContain("whitespace-pre-wrap");
    expect(html).toContain(multilineOutput);
  });

  it("renders output when toolName is not provided", () => {
    const output = "Output from anonymous tool result\nSecond line";
    const html = renderToString(<ToolExecutionBlock toolOutput={output} />);

    expect(html).toContain('data-testid="ToolExecutionBlock"');
    expect(html).not.toContain('data-testid="ToolExecutionBlock--name"');
    expect(html).toContain('data-testid="ToolExecutionBlock--output"');
    expect(html).toContain(output);
  });

  it("renders nothing for input and output when they are not provided", () => {
    const html = renderToString(<ToolExecutionBlock toolName="ReadFile" />);

    expect(html).toContain('data-testid="ToolExecutionBlock"');
    expect(html).toContain("ReadFile");
    expect(html).not.toContain('data-testid="ToolExecutionBlock--input"');
    expect(html).not.toContain('data-testid="ToolExecutionBlock--output"');
  });

  it("renders output with ANSI color escape codes as styled spans", () => {
    const ansiOutput = "\u001b[32mPASS\u001b[39m test.ts\n\u001b[31mFAIL\u001b[39m error.ts";
    const html = renderToString(<ToolExecutionBlock toolName="Bash" toolOutput={ansiOutput} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--output"');
    expect(html).toContain('style="color:rgb(34, 197, 94)"');
    expect(html).toContain("PASS");
    expect(html).toContain('style="color:rgb(239, 68, 68)"');
    expect(html).toContain("FAIL");
    expect(html).not.toContain("32m");
    expect(html).not.toContain("31m");
  });
});
