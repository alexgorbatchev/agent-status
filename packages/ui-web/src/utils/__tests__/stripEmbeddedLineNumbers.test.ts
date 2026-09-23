import { describe, it, expect } from "bun:test";
import { stripEmbeddedLineNumbers } from "../stripEmbeddedLineNumbers";

describe("stripEmbeddedLineNumbers", () => {
  it("returns unchanged output for empty or single-line strings", () => {
    expect(stripEmbeddedLineNumbers("")).toEqual({
      cleanedOutput: "",
      startLine: 1,
      lines: [],
    });

    expect(stripEmbeddedLineNumbers("single line of output")).toEqual({
      cleanedOutput: "single line of output",
      startLine: 1,
      lines: ["single line of output"],
    });
  });

  it("returns unchanged output for regular non-numbered multiline text", () => {
    const raw = "line one\nline two\nline three";
    expect(stripEmbeddedLineNumbers(raw)).toEqual({
      cleanedOutput: raw,
      startLine: 1,
      lines: ["line one", "line two", "line three"],
    });
  });

  it("strips tab-separated line numbers and returns starting line number", () => {
    const raw = '1\tpackage installer\n2\t\n3\timport (\n4\t    "context"\n5\t)';
    const result = stripEmbeddedLineNumbers(raw);

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe('package installer\n\nimport (\n    "context"\n)');
    expect(result.lines).toEqual(["package installer", "", "import (", '    "context"', ")"]);
  });

  it("strips tab-separated line numbers with offset start line", () => {
    const raw =
      "39\thost.mu.Lock()\n40\thost.requests = append(host.requests, cargoHostRequest{\n41\t    path: r.URL.Path,\n42\t})";
    const result = stripEmbeddedLineNumbers(raw);

    expect(result.startLine).toBe(39);
    expect(result.cleanedOutput).toBe(
      "host.mu.Lock()\nhost.requests = append(host.requests, cargoHostRequest{\n    path: r.URL.Path,\n})",
    );
  });

  it("strips space-separated line numbers and preserves relative indentation", () => {
    const raw = [
      "39            host.mu.Lock()",
      "40            host.requests = append(host.requests, cargoHostRequest{",
      "41                path:          r.URL.Path,",
      '42                userAgent:     r.Header.Get("User-Agent"),',
      "43            })",
    ].join("\n");

    const result = stripEmbeddedLineNumbers(raw);

    expect(result.startLine).toBe(39);
    expect(result.lines[0]).toBe("host.mu.Lock()");
    expect(result.lines[1]).toBe("host.requests = append(host.requests, cargoHostRequest{");
    expect(result.lines[2]).toBe("    path:          r.URL.Path,");
    expect(result.lines[3]).toBe('    userAgent:     r.Header.Get("User-Agent"),');
    expect(result.lines[4]).toBe("})");
  });

  it("strips pipe-separated line numbers", () => {
    const raw = '10 | func main() {\n11 |     println("hello")\n12 | }';
    const result = stripEmbeddedLineNumbers(raw);

    expect(result.startLine).toBe(10);
    expect(result.cleanedOutput).toBe('func main() {\n    println("hello")\n}');
  });

  it("does not strip non-sequential numbers (e.g. log timestamps or bullet points)", () => {
    const raw = "10:00:00 started\n10:00:01 progress\n10:00:05 finished";
    const result = stripEmbeddedLineNumbers(raw);

    expect(result.startLine).toBe(1);
    expect(result.cleanedOutput).toBe(raw);
  });
});
