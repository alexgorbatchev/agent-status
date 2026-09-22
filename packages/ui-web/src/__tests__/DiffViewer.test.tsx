import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import { DiffViewer } from "../components/DiffViewer";

describe("DiffViewer Component", () => {
  it("renders target header with path and EDIT badge", () => {
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "EDIT",
          filePath: "src/components/LogDrawer.tsx",
          hunks: [
            {
              header: "@@ -1,3 +1,3 @@",
              lines: [
                { type: "del", oldLineNumber: 1, content: "export function Old()" },
                { type: "add", newLineNumber: 1, content: "export function New()" },
              ],
            },
          ],
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer"');
    expect(html).toContain('data-testid="DiffViewer--path"');
    expect(html).toContain("src/components/LogDrawer.tsx");
    expect(html).toContain('data-testid="DiffViewer--badge"');
    expect(html).toContain("EDIT");
    expect(html).toContain('data-testid="DiffViewer--copy-btn"');
  });

  it("renders single-column unified diff with additions and deletions and prefix symbols", () => {
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "EDIT",
          filePath: "internal/service.go",
          hunks: [
            {
              lines: [
                { type: "ctx", oldLineNumber: 10, newLineNumber: 10, content: "func Run() {" },
                { type: "del", oldLineNumber: 11, content: "\treturn false" },
                { type: "add", newLineNumber: 11, content: "\treturn true" },
                { type: "ctx", oldLineNumber: 12, newLineNumber: 12, content: "}" },
              ],
            },
          ],
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer--del-line"');
    expect(html).toContain('data-testid="DiffViewer--add-line"');
    expect(html).toContain("-");
    expect(html).toContain("+");
    expect(html).toContain("return false");
    expect(html).toContain("return true");
  });

  it("renders additions and deletions with background colors and white font", () => {
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "EDIT",
          filePath: "main.go",
          hunks: [
            {
              lines: [
                { type: "del", oldLineNumber: 1, content: "const timeout = 10" },
                { type: "add", newLineNumber: 1, content: "const timeout = 20" },
              ],
            },
          ],
        }}
      />,
    );

    const delLine = html.match(/<div[^>]*data-testid="DiffViewer--del-line"[^>]*>/)?.[0] ?? "";
    const addLine = html.match(/<div[^>]*data-testid="DiffViewer--add-line"[^>]*>/)?.[0] ?? "";

    expect(delLine).toContain("bg-red-800/80");
    expect(delLine).toContain("text-white");
    expect(delLine).not.toContain("border-l-2");

    expect(addLine).toContain("bg-emerald-800/80");
    expect(addLine).toContain("text-white");
    expect(addLine).not.toContain("border-l-2");
  });

  it("renders word-level inline difference highlighting within altered lines", () => {
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "EDIT",
          filePath: "main.go",
          hunks: [
            {
              lines: [
                {
                  type: "del",
                  oldLineNumber: 1,
                  content: "const timeout = 10",
                  wordTokens: [
                    { type: "same", value: "const timeout = " },
                    { type: "del", value: "10" },
                  ],
                },
                {
                  type: "add",
                  newLineNumber: 1,
                  content: "const timeout = 20",
                  wordTokens: [
                    { type: "same", value: "const timeout = " },
                    { type: "add", value: "20" },
                  ],
                },
              ],
            },
          ],
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer--word-del"');
    expect(html).toContain('data-testid="DiffViewer--word-add"');
    expect(html).toContain("10");
    expect(html).toContain("20");

    const wordDel = html.match(/<span[^>]*data-testid="DiffViewer--word-del"[^>]*>/)?.[0] ?? "";
    const wordAdd = html.match(/<span[^>]*data-testid="DiffViewer--word-add"[^>]*>/)?.[0] ?? "";
    expect(wordDel).toContain("bg-black/30");
    expect(wordAdd).toContain("bg-black/30");
  });

  it("renders multi-hunk diffs sequentially with hunk headers", () => {
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "EDIT",
          filePath: "types.ts",
          hunks: [
            {
              header: "@@ -5,3 +5,4 @@",
              lines: [
                { type: "del", oldLineNumber: 5, content: "interface Foo" },
                { type: "add", newLineNumber: 5, content: "interface Bar" },
              ],
            },
            {
              header: "@@ -20,2 +21,3 @@",
              lines: [{ type: "add", newLineNumber: 21, content: "type Baz = string" }],
            },
          ],
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer--hunk-header"');
    expect(html).toContain("@@ -5,3 +5,4 @@");
    expect(html).toContain("@@ -20,2 +21,3 @@");
    expect(html).toContain("interface Bar");
    expect(html).toContain("type Baz = string");
  });

  it("renders WRITE mode as unescaped code viewer with line numbers and copy button", () => {
    const writeContent = 'import React from "react";\n\nexport const App = () => <div>Hello</div>;';
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "WRITE",
          filePath: "src/App.tsx",
          content: writeContent,
        }}
      />,
    );

    expect(html).toContain('data-testid="DiffViewer"');
    expect(html).toContain('data-testid="DiffViewer--badge"');
    expect(html).toContain("WRITE");
    expect(html).toContain('data-testid="DiffViewer--write-content"');
    expect(html).toContain("src/App.tsx");
    expect(html).toContain("import React from &quot;react&quot;;");
    expect(html).not.toContain("\\n");
  });

  it("renders empty file indicator when write content is empty", () => {
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "WRITE",
          filePath: "empty.txt",
          content: "",
        }}
      />,
    );

    expect(html).toContain("(Empty file)");
  });

  it("renders no differences detected when hunks array is empty", () => {
    const html = renderToString(
      <DiffViewer
        payload={{
          action: "EDIT",
          filePath: "no-change.ts",
          hunks: [],
        }}
      />,
    );

    expect(html).toContain("No differences detected");
  });
});
