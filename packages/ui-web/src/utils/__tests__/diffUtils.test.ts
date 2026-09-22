import { describe, it, expect } from "bun:test";
import { isFileModification, parseFileModification } from "../diffUtils";

describe("diffUtils", () => {
  describe("isFileModification", () => {
    it("returns false for null or undefined inputs", () => {
      expect(isFileModification(null, null)).toBe(false);
      expect(isFileModification("Bash", null)).toBe(false);
      expect(isFileModification("Bash", { command: "ls" })).toBe(false);
    });

    it("returns true for Claude Code FileEdit", () => {
      expect(
        isFileModification("FileEdit", {
          path: "foo.ts",
          old_string: "const a = 1;",
          new_string: "const a = 2;",
        }),
      ).toBe(true);
    });

    it("returns true for Pi edit tool", () => {
      expect(
        isFileModification("edit", {
          path: "bar.py",
          edits: [{ oldText: "print(1)", newText: "print(2)" }],
        }),
      ).toBe(true);
    });

    it("returns true for file write tools", () => {
      expect(isFileModification("FileWrite", { path: "a.txt", content: "hello" })).toBe(true);
      expect(isFileModification("write", { path: "b.txt", content: "hello" })).toBe(true);
      expect(isFileModification("write_file", { file_path: "c.txt", content: "hello" })).toBe(true);
    });

    it("returns true for Codex apply_patch", () => {
      expect(
        isFileModification("apply_patch", {
          patch: "diff --git a/x b/x\n@@ -1 +1 @@\n-1\n+2",
        }),
      ).toBe(true);
    });

    it("returns true for payload with path and edits even if toolName is unrecognized", () => {
      expect(
        isFileModification("custom_updater", {
          path: "app.go",
          edits: [{ oldText: "a", newText: "b" }],
        }),
      ).toBe(true);
    });

    it("returns true if fileModification is provided even without toolName or toolInput", () => {
      expect(
        isFileModification(null, null, {
          action: "WRITE",
          filePath: "out.log",
          content: "ready",
        }),
      ).toBe(true);
    });

    it("returns true when patch contains unified diff @@ header", () => {
      expect(
        isFileModification(null, {
          patch: "@@ -1,2 +1,2 @@\n-a\n+b",
        }),
      ).toBe(true);
    });
  });

  describe("parseFileModification", () => {
    it("returns null for non-file modification inputs", () => {
      expect(parseFileModification("Bash", { command: "cat foo" })).toBeNull();
      expect(parseFileModification(null, null)).toBeNull();
    });

    it("parses Claude Code FileEdit and computes line differences and word highlights", () => {
      const parsed = parseFileModification("FileEdit", {
        path: "src/calc.ts",
        old_string: "function sum(a, b) {\n  return a - b;\n}",
        new_string: "function sum(a, b) {\n  return a + b;\n}",
      });

      expect(parsed).not.toBeNull();
      expect(parsed?.action).toBe("EDIT");
      expect(parsed?.filePath).toBe("src/calc.ts");
      expect(parsed?.hunks).toHaveLength(1);

      const lines = parsed?.hunks?.[0]?.lines ?? [];
      const delLine = lines.find((l) => l.type === "del");
      const addLine = lines.find((l) => l.type === "add");

      expect(delLine).toBeDefined();
      expect(delLine?.content).toBe("  return a - b;");
      expect(addLine).toBeDefined();
      expect(addLine?.content).toBe("  return a + b;");

      // Check word tokens
      const delMinus = delLine?.wordTokens?.find((t) => t.type === "del");
      const addPlus = addLine?.wordTokens?.find((t) => t.type === "add");
      expect(delMinus?.value).toBe("-");
      expect(addPlus?.value).toBe("+");
    });

    it("parses Pi multiple edits as sequential multi-hunks", () => {
      const parsed = parseFileModification("edit", {
        path: "pkg/core.go",
        edits: [
          {
            oldText: "package core\n\nvar Version = 1",
            newText: "package core\n\nvar Version = 2",
          },
          {
            oldText: "func Run() bool {\n  return false\n}",
            newText: "func Run() bool {\n  return true\n}",
          },
        ],
      });

      expect(parsed).not.toBeNull();
      expect(parsed?.action).toBe("EDIT");
      expect(parsed?.filePath).toBe("pkg/core.go");
      expect(parsed?.hunks).toHaveLength(2);
      expect(parsed?.hunks?.[0]?.header).toBe("Hunk 1 of 2");
      expect(parsed?.hunks?.[1]?.header).toBe("Hunk 2 of 2");
    });

    it("parses Codex unified git patch format with line numbers", () => {
      const patch = [
        "diff --git a/internal/config.go b/internal/config.go",
        "--- a/internal/config.go",
        "+++ b/internal/config.go",
        "@@ -10,3 +10,4 @@ func Load() error {",
        " const port = 8080",
        "-const debug = false",
        "+const debug = true",
        "+const timeout = 30",
        " return nil",
      ].join("\n");

      const parsed = parseFileModification("apply_patch", { patch });

      expect(parsed).not.toBeNull();
      expect(parsed?.action).toBe("EDIT");
      expect(parsed?.filePath).toBe("internal/config.go");
      expect(parsed?.hunks).toHaveLength(1);

      const hunk = parsed?.hunks?.[0];
      expect(hunk?.header).toBe("@@ -10,3 +10,4 @@ func Load() error {");

      const lines = hunk?.lines ?? [];
      const delLine = lines.find((l) => l.type === "del");
      const addLines = lines.filter((l) => l.type === "add");

      expect(delLine?.oldLineNumber).toBe(11);
      expect(delLine?.content).toBe("const debug = false");

      expect(addLines[0]?.newLineNumber).toBe(11);
      expect(addLines[0]?.content).toBe("const debug = true");
      expect(addLines[1]?.newLineNumber).toBe(12);
      expect(addLines[1]?.content).toBe("const timeout = 30");
    });

    it("parses write operations into WRITE action payload", () => {
      const parsed = parseFileModification("FileWrite", {
        path: "notes.txt",
        content: "First line\nSecond line\n",
      });

      expect(parsed).not.toBeNull();
      expect(parsed?.action).toBe("WRITE");
      expect(parsed?.filePath).toBe("notes.txt");
      expect(parsed?.content).toBe("First line\nSecond line\n");
      expect(parsed?.hunks).toBeUndefined();
    });

    it("prioritizes fileModification from telemetry data without inspecting toolInput", () => {
      const norm = {
        action: "EDIT" as const,
        filePath: "pkg/service.go",
        hunks: [
          {
            header: "@@ -1,2 +1,2 @@",
            lines: [
              { type: "del" as const, oldLineNumber: 1, content: "old code" },
              { type: "add" as const, newLineNumber: 1, content: "new code" },
            ],
          },
        ],
      };
      const parsed = parseFileModification("unknown_tool", null, norm);
      expect(parsed).not.toBeNull();
      expect(parsed?.action).toBe("EDIT");
      expect(parsed?.filePath).toBe("pkg/service.go");
      expect(parsed?.hunks).toHaveLength(1);
      expect(parsed?.hunks?.[0]?.lines[0]?.content).toBe("old code");
    });

    it("parses Claude Code MultiEdit format with old_string and new_string in edits array", () => {
      const parsed = parseFileModification("MultiEdit", {
        file_path: "src/server.ts",
        edits: [
          {
            old_string: "const port = 3000;\n",
            new_string: "const port = 4000;\n",
          },
          {
            old_string: "const host = 'localhost';\n",
            new_string: "const host = '0.0.0.0';\n",
          },
        ],
      });
      expect(parsed).not.toBeNull();
      expect(parsed?.action).toBe("EDIT");
      expect(parsed?.filePath).toBe("src/server.ts");
      expect(parsed?.hunks).toHaveLength(2);
      expect(parsed?.hunks?.[0]?.lines.find((l) => l.type === "del")?.content).toBe("const port = 3000;");
      expect(parsed?.hunks?.[0]?.lines.find((l) => l.type === "add")?.content).toBe("const port = 4000;");
    });
  });
});
