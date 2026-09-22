import { describe, it, expect, mock } from "bun:test";
import assert from "node:assert";
import React from "react";
import { renderToString } from "react-dom/server";
import { ToolExecutionBlock } from "../components/ToolExecutionBlock";
import { setupMockDomEnvironment, mountComponent } from "./helpers";

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

  it("applies overflow-wrap, break-all, min-w-0, and max-w-full to pre elements on unbroken payloads", () => {
    const unbrokenInput = { payload: "a".repeat(500) };
    const unbrokenOutput = "x".repeat(500);
    const html = renderToString(
      <ToolExecutionBlock toolName="RunCommand" toolInput={unbrokenInput} toolOutput={unbrokenOutput} />,
    );

    // Input pre element must have overflow breaking styles and containment
    expect(html).toMatch(/data-testid="ToolExecutionBlock--input"[^>]*class="[^"]*break-all[^"]*"/);
    expect(html).toMatch(/data-testid="ToolExecutionBlock--input"[^>]*class="[^"]*\[overflow-wrap:anywhere\][^"]*"/);
    expect(html).toMatch(/data-testid="ToolExecutionBlock--input"[^>]*class="[^"]*min-w-0[^"]*"/);
    expect(html).toMatch(/data-testid="ToolExecutionBlock--input"[^>]*class="[^"]*max-w-full[^"]*"/);
    expect(html).not.toMatch(/data-testid="ToolExecutionBlock--input"[^>]*class="[^"]*break-words[^"]*"/);

    // Output pre element must have overflow breaking styles and containment
    expect(html).toMatch(/data-testid="ToolExecutionBlock--output"[^>]*class="[^"]*break-all[^"]*"/);
    expect(html).toMatch(/data-testid="ToolExecutionBlock--output"[^>]*class="[^"]*\[overflow-wrap:anywhere\][^"]*"/);
    expect(html).toMatch(/data-testid="ToolExecutionBlock--output"[^>]*class="[^"]*min-w-0[^"]*"/);
    expect(html).toMatch(/data-testid="ToolExecutionBlock--output"[^>]*class="[^"]*max-w-full[^"]*"/);
    expect(html).not.toMatch(/data-testid="ToolExecutionBlock--output"[^>]*class="[^"]*break-words[^"]*"/);
  });

  it("constrains scroll container with max-h-64 and overflow-auto so gutter and output scroll together without empty space", () => {
    const multilineOutput = Array.from({ length: 57 }, (_, i) => `Line ${i + 1}`).join("\n");
    const html = renderToString(<ToolExecutionBlock toolName="bash" toolOutput={multilineOutput} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--output-scroll-container"');
    expect(html).toMatch(/data-testid="ToolExecutionBlock--output-scroll-container"[^>]*class="[^"]*max-h-64[^"]*"/);
    expect(html).toMatch(
      /data-testid="ToolExecutionBlock--output-scroll-container"[^>]*class="[^"]*overflow-auto[^"]*"/,
    );
    // pre element must not independently cap max-h-64 causing empty space next to a taller gutter
    expect(html).not.toMatch(/data-testid="ToolExecutionBlock--output"[^>]*class="[^"]*max-h-64[^"]*"/);
    // pre element must not retain independent horizontal scrollbar that detaches from outer container
    expect(html).not.toMatch(/data-testid="ToolExecutionBlock--output"[^>]*class="[^"]*overflow-x-auto[^"]*"/);
    // line gutter must be sticky with solid opaque background to prevent text bleed-through
    expect(html).toMatch(/data-testid="ToolExecutionBlock--line-gutter"[^>]*class="[^"]*sticky left-0[^"]*"/);
    expect(html).toMatch(/data-testid="ToolExecutionBlock--line-gutter"[^>]*class="[^"]*bg-slate-950[^"]*"/);
  });

  it("renders DiffViewer instead of raw JSON pre for Claude Code FileEdit", () => {
    const html = renderToString(
      <ToolExecutionBlock
        toolName="FileEdit"
        toolInput={{
          path: "src/utils/math.ts",
          old_string: "export const add = (a, b) => a - b;",
          new_string: "export const add = (a, b) => a + b;",
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer"');
    expect(html).toContain('data-testid="DiffViewer--path"');
    expect(html).toContain("src/utils/math.ts");
    expect(html).toContain('data-testid="DiffViewer--badge"');
    expect(html).toContain("EDIT");
    expect(html).not.toContain('data-testid="ToolExecutionBlock--input"');
  });

  it("renders DiffViewer instead of raw JSON pre for Pi edit tool", () => {
    const html = renderToString(
      <ToolExecutionBlock
        toolName="edit"
        toolInput={{
          path: "packages/ui-web/App.tsx",
          edits: [
            {
              oldText: "const a = 1;",
              newText: "const a = 2;",
            },
          ],
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer"');
    expect(html).toContain('data-testid="DiffViewer--path"');
    expect(html).toContain("packages/ui-web/App.tsx");
    expect(html).not.toContain('data-testid="ToolExecutionBlock--input"');
  });

  it("renders DiffViewer instead of raw JSON pre for FileWrite", () => {
    const html = renderToString(
      <ToolExecutionBlock
        toolName="FileWrite"
        toolInput={{
          path: "README.md",
          content: "# Project\n\nWelcome to our project.",
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer"');
    expect(html).toContain('data-testid="DiffViewer--badge"');
    expect(html).toContain("WRITE");
    expect(html).toContain('data-testid="DiffViewer--path"');
    expect(html).toContain("README.md");
    expect(html).not.toContain('data-testid="ToolExecutionBlock--input"');
  });

  it("renders status badge for clean exit (exitCode 0)", () => {
    const html = renderToString(<ToolExecutionBlock toolName="Bash" exitCode={0} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--status"');
    expect(html).toContain("exit 0");
    expect(html).toContain("text-emerald-400");
  });

  it("renders status badge for non-zero exit code (failure)", () => {
    const html = renderToString(<ToolExecutionBlock toolName="Bash" exitCode={127} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--status"');
    expect(html).toContain("exit 127");
    expect(html).toContain("text-red-400");
  });

  it("renders status badge for isError true without explicit exit code", () => {
    const html = renderToString(<ToolExecutionBlock toolName="CustomTool" isError={true} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--status"');
    expect(html).toContain("failed");
    expect(html).toContain("text-red-400");
  });

  it("renders terminal view with $ prompt and copy button for shell tools", () => {
    const multilineCmd = "git status\nbun test";
    const html = renderToString(<ToolExecutionBlock toolName="bash" toolInput={{ command: multilineCmd }} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--terminal"');
    expect(html).toContain('data-testid="ToolExecutionBlock--copy-command-btn"');
    expect(html).toContain("$");
    expect(html).toContain("git status");
    expect(html).toContain("bun test");
  });

  it("renders structured parameter pills for search and navigation tools", () => {
    const html = renderToString(
      <ToolExecutionBlock toolName="grep" toolInput={{ pattern: "TODO", path: "src/components", maxResults: 10 }} />,
    );

    expect(html).toContain('data-testid="ToolExecutionBlock--params"');
    expect(html).toContain("pattern");
    expect(html).toContain("TODO");
    expect(html).toContain("path");
    expect(html).toContain("src/components");
    expect(html).toContain("maxResults");
    expect(html).toContain("10");
  });

  it("renders enhanced output viewer with line count badge, stream label, copy button, and line gutter", () => {
    const output = "first line\nsecond line\nthird line";
    const html = renderToString(<ToolExecutionBlock toolName="bash" toolOutput={output} exitCode={0} />);

    expect(html).toContain('data-testid="ToolExecutionBlock--output-header"');
    expect(html).toContain('data-testid="ToolExecutionBlock--line-count"');
    expect(html).toContain("3 lines");
    expect(html).toContain('data-testid="ToolExecutionBlock--stream-type"');
    expect(html).toContain("OUTPUT");
    expect(html).toContain('data-testid="ToolExecutionBlock--copy-output-btn"');
    expect(html).toContain('data-testid="ToolExecutionBlock--line-gutter"');
    expect(html).toContain("1");
    expect(html).toContain("2");
    expect(html).toContain("3");
  });

  it("renders stream label as STDERR when isError is true or exitCode is non-zero", () => {
    const html = renderToString(
      <ToolExecutionBlock toolName="bash" toolOutput="Command failed: exit 1" exitCode={1} isError={true} />,
    );

    expect(html).toContain('data-testid="ToolExecutionBlock--stream-type"');
    expect(html).toContain("STDERR");
  });

  it("copies shell command to clipboard when copy-command button is clicked", async () => {
    let copiedText = "";
    const originalClipboard = navigator.clipboard;
    Object.assign(navigator, {
      clipboard: {
        writeText: mock(async (text: string) => {
          copiedText = text;
        }),
      },
    });

    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const { container, unmount } = await mountComponent(
        <ToolExecutionBlock toolName="bash" toolInput={{ command: "bun test --coverage" }} />,
        mockDoc,
      );

      const copyBtn = container.querySelector('[data-testid="ToolExecutionBlock--copy-command-btn"]');
      assert(copyBtn);
      copyBtn.click();

      expect(copiedText).toBe("bun test --coverage");
      await unmount();
    } finally {
      cleanup();
      Object.assign(navigator, { clipboard: originalClipboard });
    }
  });

  it("copies output text to clipboard when copy-output button is clicked", async () => {
    let copiedText = "";
    const originalClipboard = navigator.clipboard;
    Object.assign(navigator, {
      clipboard: {
        writeText: mock(async (text: string) => {
          copiedText = text;
        }),
      },
    });

    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const { container, unmount } = await mountComponent(
        <ToolExecutionBlock toolName="bash" toolOutput="Build complete in 12ms" />,
        mockDoc,
      );

      const copyBtn = container.querySelector('[data-testid="ToolExecutionBlock--copy-output-btn"]');
      assert(copyBtn);
      copyBtn.click();

      expect(copiedText).toBe("Build complete in 12ms");
      await unmount();
    } finally {
      cleanup();
      Object.assign(navigator, { clipboard: originalClipboard });
    }
  });

  it("renders duration badge formatted as milliseconds, seconds, or minutes", () => {
    const htmlMs = renderToString(<ToolExecutionBlock toolName="bash" durationMs={450} />);
    expect(htmlMs).toContain('data-testid="ToolExecutionBlock--duration"');
    expect(htmlMs).toContain("450ms");

    const htmlSecs = renderToString(<ToolExecutionBlock toolName="bash" durationMs={2500} />);
    expect(htmlSecs).toContain('data-testid="ToolExecutionBlock--duration"');
    expect(htmlSecs).toContain("2.5s");

    const htmlMins = renderToString(<ToolExecutionBlock toolName="bash" durationMs={65000} />);
    expect(htmlMins).toContain('data-testid="ToolExecutionBlock--duration"');
    expect(htmlMins).toContain("1m 5s");
  });

  it("extracts command from cmd and script fields for shell tools", () => {
    const htmlCmd = renderToString(<ToolExecutionBlock toolName="sh" toolInput={{ cmd: "echo 123" }} />);
    expect(htmlCmd).toContain('data-testid="ToolExecutionBlock--terminal"');
    expect(htmlCmd).toContain("echo 123");

    const htmlScript = renderToString(<ToolExecutionBlock toolName="exec" toolInput={{ script: "node app.js" }} />);
    expect(htmlScript).toContain('data-testid="ToolExecutionBlock--terminal"');
    expect(htmlScript).toContain("node app.js");
  });

  it("renders status badge when toolName is absent but isError is provided", () => {
    const htmlSuccess = renderToString(<ToolExecutionBlock isError={false} />);
    expect(htmlSuccess).toContain('data-testid="ToolExecutionBlock--status"');
    expect(htmlSuccess).toContain("success");

    const htmlFailure = renderToString(<ToolExecutionBlock isError={true} />);
    expect(htmlFailure).toContain('data-testid="ToolExecutionBlock--status"');
    expect(htmlFailure).toContain("failed");
  });

  it("renders DiffViewer directly from fileModification prop without toolInput", () => {
    const html = renderToString(
      <ToolExecutionBlock
        fileModification={{
          action: "WRITE",
          filePath: "docs/spec.md",
          content: "# Specification\n",
        }}
      />,
    );
    expect(html).toContain('data-testid="DiffViewer"');
    expect(html).toContain("docs/spec.md");
    expect(html).toContain("WRITE");
  });
});
