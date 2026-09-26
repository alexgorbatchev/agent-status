import { describe, it, expect } from "bun:test";
import { FILE_READ_TOOLS, isFileReadTool, stripEmbeddedLineNumbers } from "../stripEmbeddedLineNumbers";

describe("isFileReadTool", () => {
  it("returns true for known file read tool names case-insensitively", () => {
    const knownTools = [
      "read",
      "Read",
      "READ",
      "read_file",
      "ReadFile",
      "readfile",
      "FileRead",
      "fileread",
      "file_read",
      "view",
      "View",
      "cat",
      "read_file_or_dir",
    ];

    for (const tool of knownTools) {
      expect(isFileReadTool(tool)).toBe(true);
    }
  });

  it("returns false for non-file read tools and invalid values", () => {
    const nonReadTools = [
      "bash",
      "sh",
      "terminal",
      "psql",
      "grep",
      "find",
      "write",
      "edit",
      "",
      "   ",
      null,
      undefined,
    ];

    for (const tool of nonReadTools) {
      expect(isFileReadTool(tool)).toBe(false);
    }
  });

  it("exposes FILE_READ_TOOLS set", () => {
    expect(FILE_READ_TOOLS.has("read")).toBe(true);
    expect(FILE_READ_TOOLS.has("read_file")).toBe(true);
    expect(FILE_READ_TOOLS.has("cat")).toBe(true);
    expect(FILE_READ_TOOLS.has("view")).toBe(true);
  });
});

describe("stripEmbeddedLineNumbers", () => {
  it("returns unchanged text when output is empty", () => {
    expect(stripEmbeddedLineNumbers("")).toEqual({
      cleanedOutput: "",
      startLine: 1,
      lines: [],
    });
    expect(stripEmbeddedLineNumbers("", "read")).toEqual({
      cleanedOutput: "",
      startLine: 1,
      lines: [],
    });
  });

  it("returns unchanged text when output has no newlines", () => {
    expect(stripEmbeddedLineNumbers("single line of output", "read")).toEqual({
      cleanedOutput: "single line of output",
      startLine: 1,
      lines: ["single line of output"],
    });
  });

  it("returns unchanged text when output has trailing newline but fewer than 2 content lines", () => {
    expect(stripEmbeddedLineNumbers("single line\n", "read")).toEqual({
      cleanedOutput: "single line\n",
      startLine: 1,
      lines: ["single line", ""],
    });
  });

  it("returns unchanged text when output has no embedded line numbers", () => {
    const raw = "line one\nline two\nline three";
    expect(stripEmbeddedLineNumbers(raw, "read")).toEqual({
      cleanedOutput: raw,
      startLine: 1,
      lines: ["line one", "line two", "line three"],
    });
  });

  it("accepts options object with toolName", () => {
    const raw = '1\tpackage installer\n2\t\n3\timport (\n4\t    "context"\n5\t)';
    const result = stripEmbeddedLineNumbers(raw, { toolName: "read" });

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe('package installer\n\nimport (\n    "context"\n)');
  });

  it("strips tab-separated line numbers and returns starting line number", () => {
    const raw = '1\tpackage installer\n2\t\n3\timport (\n4\t    "context"\n5\t)';
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe('package installer\n\nimport (\n    "context"\n)');
    expect(result.lines).toEqual(["package installer", "", "import (", '    "context"', ")"]);
  });

  it("strips tab-separated line numbers with offset start line", () => {
    const raw =
      "39\thost.mu.Lock()\n40\thost.requests = append(host.requests, cargoHostRequest{\n41\t    path: r.URL.Path,\n42\t})";
    const result = stripEmbeddedLineNumbers(raw, "ReadFile");

    expect(result.startLine).toBe(39);
    expect(result.cleanedOutput).toBe(
      "host.mu.Lock()\nhost.requests = append(host.requests, cargoHostRequest{\n    path: r.URL.Path,\n})",
    );
  });

  it("strips tab-separated line numbers with empty lines that contain only line number", () => {
    const raw = "1\tfirst line\n2\n3\tthird line";
    const result = stripEmbeddedLineNumbers(raw, "read_file");

    expect(result.startLine).toBe(1);
    expect(result.lines).toEqual(["first line", "", "third line"]);
    expect(result.cleanedOutput).toBe("first line\n\nthird line");
  });

  it("preserves tab lines when broken format interrupts sequential numbers", () => {
    const raw = "1\tfirst line\nnot a tab line\n3\tthird line";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  it("preserves tab lines when numbers are not sequential", () => {
    const raw = "1\tfirst line\n3\tthird line";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  it("preserves trailing newline for tab-separated output", () => {
    const raw = "1\tfirst\n2\tsecond\n";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe("first\nsecond\n");
    expect(result.lines).toEqual(["first", "second", ""]);
  });

  it("strips pipe-separated line numbers", () => {
    const raw = '10 | func main() {\n11 |     println("hello")\n12 | }';
    const result = stripEmbeddedLineNumbers(raw, "view");

    expect(result.startLine).toBe(10);
    expect(result.cleanedOutput).toBe('func main() {\n    println("hello")\n}');
  });

  it("preserves pipe lines when broken format interrupts sequential numbers", () => {
    const raw = "10 | first\nnot a pipe line\n12 | third";
    const result = stripEmbeddedLineNumbers(raw, "view");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  it("preserves pipe lines when numbers are not sequential", () => {
    const raw = "10 | first\n12 | third";
    const result = stripEmbeddedLineNumbers(raw, "view");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  it("preserves trailing newline for pipe-separated output", () => {
    const raw = "10 | first\n11 | second\n";
    const result = stripEmbeddedLineNumbers(raw, "view");

    expect(result.startLine).toBe(10);
    expect(result.cleanedOutput).toBe("first\nsecond\n");
    expect(result.lines).toEqual(["first", "second", ""]);
  });

  it("strips space-separated line numbers and preserves relative indentation", () => {
    const raw = [
      "39            host.mu.Lock()",
      "40            host.requests = append(host.requests, cargoHostRequest{",
      "41                path:          r.URL.Path,",
      '42                userAgent:     r.Header.Get("User-Agent"),',
      "43            })",
    ].join("\n");

    const result = stripEmbeddedLineNumbers(raw, "cat");

    expect(result.startLine).toBe(39);
    expect(result.lines[0]).toBe("host.mu.Lock()");
    expect(result.lines[1]).toBe("host.requests = append(host.requests, cargoHostRequest{");
    expect(result.lines[2]).toBe("    path:          r.URL.Path,");
    expect(result.lines[3]).toBe('    userAgent:     r.Header.Get("User-Agent"),');
    expect(result.lines[4]).toBe("})");
  });

  it("strips space-separated line numbers with empty lines that contain only line number", () => {
    const raw = "  1   first line\n  2\n  3   third line";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.lines).toEqual(["first line", "", "third line"]);
    expect(result.cleanedOutput).toBe("first line\n\nthird line");
  });

  it("updates minSepSpaces when separation varies across lines", () => {
    const raw = "  1    first line with four spaces\n  2  second line with two spaces";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.lines[0]).toBe("  first line with four spaces");
    expect(result.lines[1]).toBe("second line with two spaces");
  });

  it("does not strip space-separated lines when minSepSpaces is less than 2", () => {
    const raw = "1 first\n2 second";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  it("preserves space-separated lines when broken format interrupts sequential numbers", () => {
    const raw = "  1   first line\nnot a space numbered line\n  3   third line";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  it("preserves space-separated lines when numbers are not sequential", () => {
    const raw = "  1   first line\n  3   third line";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  it("preserves trailing newline for space-separated output", () => {
    const raw = "  1   first\n  2   second\n";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe("first\nsecond\n");
    expect(result.lines).toEqual(["first", "second", ""]);
  });

  it("does not strip non-sequential numbers (e.g. log timestamps or bullet points)", () => {
    const raw = "10:00:00 started\n10:00:01 progress\n10:00:05 finished";
    const result = stripEmbeddedLineNumbers(raw, "read");

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });

  describe("negative probes and tool gating", () => {
    it("does not strip TSV row ids when tool is bash or unknown", () => {
      const tsv = "1\talice\n2\tbob\n3\tcarol";
      expect(stripEmbeddedLineNumbers(tsv, "bash").cleanedOutput).toBe(tsv);
      expect(stripEmbeddedLineNumbers(tsv, { toolName: "bash" }).cleanedOutput).toBe(tsv);
      expect(stripEmbeddedLineNumbers(tsv).cleanedOutput).toBe(tsv);
    });

    it("does not strip psql table output when tool is psql or unknown", () => {
      const psql = " 1 | alice\n 2 | bob\n 3 | carol";
      expect(stripEmbeddedLineNumbers(psql, "psql").cleanedOutput).toBe(psql);
      expect(stripEmbeddedLineNumbers(psql, { toolName: "psql" }).cleanedOutput).toBe(psql);
      expect(stripEmbeddedLineNumbers(psql).cleanedOutput).toBe(psql);
    });

    it("does not strip shell history when tool is bash or unknown", () => {
      const history = "  1  git status\n  2  ls -la\n  3  cargo test";
      expect(stripEmbeddedLineNumbers(history, "bash").cleanedOutput).toBe(history);
      expect(stripEmbeddedLineNumbers(history).cleanedOutput).toBe(history);
    });

    it("does not strip numbered table data when tool is bash or unknown", () => {
      const table = "1  apples   10\n2  pears    20";
      expect(stripEmbeddedLineNumbers(table, "bash").cleanedOutput).toBe(table);
      expect(stripEmbeddedLineNumbers(table).cleanedOutput).toBe(table);
    });
  });
});
