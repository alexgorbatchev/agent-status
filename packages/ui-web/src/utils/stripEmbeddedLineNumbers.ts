export interface IStrippedOutput {
  cleanedOutput: string;
  startLine: number;
  lines: string[];
}

export interface IStripEmbeddedLineNumbersOptions {
  toolName?: string | null;
}

export type StripEmbeddedLineNumbersInput = string | null | undefined | IStripEmbeddedLineNumbersOptions;

export const FILE_READ_TOOLS: ReadonlySet<string> = new Set([
  "read",
  "read_file",
  "readfile",
  "fileread",
  "file_read",
  "view",
  "cat",
  "read_file_or_dir",
]);

export function isFileReadTool(toolName?: string | null): boolean {
  if (!toolName) return false;
  return FILE_READ_TOOLS.has(toolName.trim().toLowerCase());
}

/**
 * Detects whether tool output text already has sequential embedded line numbers
 * (e.g. from file read tools like `read`, `view`, `cat -n`), strips them from the
 * content to prevent duplicate line number columns, and returns the starting line number.
 *
 * Stripping only applies to known file-read tools. For non-file-read tools (e.g. bash,
 * psql, grep) or unknown tools, output is preserved as-is so leading numeric columns
 * (IDs, row numbers, history numbers) are not stripped.
 */
export function stripEmbeddedLineNumbers(
  output: string,
  toolNameOrOptions?: StripEmbeddedLineNumbersInput,
): IStrippedOutput {
  const toolName = typeof toolNameOrOptions === "string" ? toolNameOrOptions : toolNameOrOptions?.toolName;

  if (!isFileReadTool(toolName)) {
    const lines = output ? output.split("\n") : [];
    return {
      cleanedOutput: output || "",
      startLine: 1,
      lines,
    };
  }
  if (!output || !output.includes("\n")) {
    const lines = output ? [output] : [];
    return {
      cleanedOutput: output || "",
      startLine: 1,
      lines,
    };
  }

  const allLines = output.split("\n");
  const hasTrailingEmpty = allLines.length > 1 && allLines[allLines.length - 1] === "";
  const lines = hasTrailingEmpty ? allLines.slice(0, -1) : allLines;

  if (lines.length < 2) {
    return {
      cleanedOutput: output,
      startLine: 1,
      lines: allLines,
    };
  }

  // 1. Check for tab-separated line numbers: `^\s*(\d+)\t(.*)$`
  const tabRegex = /^\s*(\d+)\t(.*)$/;
  const firstTabMatch = lines[0]?.match(tabRegex);
  if (firstTabMatch && firstTabMatch[1]) {
    const startNum = parseInt(firstTabMatch[1], 10);
    let isSequential = true;
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]!;
      const match = line.match(tabRegex);
      if (!match || !match[1]) {
        const numOnly = line.match(/^\s*(\d+)\s*$/);
        if (numOnly && numOnly[1] && parseInt(numOnly[1], 10) === startNum + i) {
          continue;
        }
        isSequential = false;
        break;
      }
      if (parseInt(match[1], 10) !== startNum + i) {
        isSequential = false;
        break;
      }
    }
    if (isSequential) {
      const cleaned = lines.map((line) => {
        const match = line.match(tabRegex);
        if (match) return match[2] ?? "";
        const numOnly = line.match(/^\s*(\d+)\s*$/);
        if (numOnly) return "";
        return line;
      });
      if (hasTrailingEmpty) cleaned.push("");
      return {
        cleanedOutput: cleaned.join("\n"),
        startLine: startNum,
        lines: cleaned,
      };
    }
  }

  // 2. Check for pipe-separated line numbers: `^\s*(\d+)\s*\|\s?(.*)$`
  const pipeRegex = /^\s*(\d+)\s*\|\s?(.*)$/;
  const firstPipeMatch = lines[0]?.match(pipeRegex);
  if (firstPipeMatch && firstPipeMatch[1]) {
    const startNum = parseInt(firstPipeMatch[1], 10);
    let isSequential = true;
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]!;
      const match = line.match(pipeRegex);
      if (!match || !match[1]) {
        isSequential = false;
        break;
      }
      if (parseInt(match[1], 10) !== startNum + i) {
        isSequential = false;
        break;
      }
    }
    if (isSequential) {
      const cleaned = lines.map((line) => {
        const match = line.match(pipeRegex);
        return match ? (match[2] ?? "") : line;
      });
      if (hasTrailingEmpty) cleaned.push("");
      return {
        cleanedOutput: cleaned.join("\n"),
        startLine: startNum,
        lines: cleaned,
      };
    }
  }

  // 3. Check for space-separated line numbers: `^(\s*)(\d+)([ ]+)(.*)$`
  const spaceRegex = /^(\s*)(\d+)([ ]+)(.*)$/;
  const firstSpaceMatch = lines[0]?.match(spaceRegex);
  if (firstSpaceMatch && firstSpaceMatch[2]) {
    const startNum = parseInt(firstSpaceMatch[2], 10);
    let isSequential = true;
    let minSepSpaces = firstSpaceMatch[3]?.length ?? 0;

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]!;
      const match = line.match(spaceRegex);
      if (!match || !match[2]) {
        const numOnly = line.match(/^\s*(\d+)\s*$/);
        if (numOnly && numOnly[1] && parseInt(numOnly[1], 10) === startNum + i) {
          continue;
        }
        isSequential = false;
        break;
      }
      if (parseInt(match[2], 10) !== startNum + i) {
        isSequential = false;
        break;
      }
      const sepLen = match[3]?.length ?? 0;
      if (match[4] && match[4].length > 0 && sepLen < minSepSpaces) {
        minSepSpaces = sepLen;
      }
    }

    if (isSequential && minSepSpaces >= 2) {
      const cleaned = lines.map((line) => {
        const match = line.match(spaceRegex);
        if (!match) return "";
        const spacesAfterNum = match[3] ?? "";
        const remainingSpaces = spacesAfterNum.slice(minSepSpaces);
        return remainingSpaces + (match[4] ?? "");
      });
      if (hasTrailingEmpty) cleaned.push("");
      return {
        cleanedOutput: cleaned.join("\n"),
        startLine: startNum,
        lines: cleaned,
      };
    }
  }

  return {
    cleanedOutput: output,
    startLine: 1,
    lines: allLines,
  };
}
