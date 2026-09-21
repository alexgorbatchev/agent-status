import { diffLines, diffWordsWithSpace } from "diff";

export interface IWordToken {
  type: "add" | "del" | "same";
  value: string;
}

export interface IDiffLine {
  type: "add" | "del" | "ctx";
  oldLineNumber?: number;
  newLineNumber?: number;
  content: string;
  wordTokens?: IWordToken[];
}

export interface IDiffHunk {
  header?: string;
  lines: IDiffLine[];
}

export type FileDiffAction = "EDIT" | "WRITE";

export interface IFileModificationPayload {
  action: FileDiffAction;
  filePath: string;
  hunks?: IDiffHunk[];
  content?: string;
}

function splitLines(text: string): string[] {
  const lines = text.split("\n");
  if (lines.length > 0 && lines[lines.length - 1] === "") {
    lines.pop();
  }
  return lines;
}

function applyWordLevelHighlights(lines: IDiffLine[]): void {
  let i = 0;
  while (i < lines.length) {
    if (lines[i].type === "del") {
      const delIndices: number[] = [];
      while (i < lines.length && lines[i].type === "del") {
        delIndices.push(i);
        i++;
      }
      const addIndices: number[] = [];
      while (i < lines.length && lines[i].type === "add") {
        addIndices.push(i);
        i++;
      }

      const pairCount = Math.min(delIndices.length, addIndices.length);
      for (let p = 0; p < pairCount; p++) {
        const delLine = lines[delIndices[p]];
        const addLine = lines[addIndices[p]];

        const wordDiffs = diffWordsWithSpace(delLine.content, addLine.content);
        const delTokens: IWordToken[] = [];
        const addTokens: IWordToken[] = [];

        for (const token of wordDiffs) {
          if (token.added) {
            addTokens.push({ type: "add", value: token.value });
          } else if (token.removed) {
            delTokens.push({ type: "del", value: token.value });
          } else {
            delTokens.push({ type: "same", value: token.value });
            addTokens.push({ type: "same", value: token.value });
          }
        }

        delLine.wordTokens = delTokens;
        addLine.wordTokens = addTokens;
      }

      // Remaining unpaired deleted lines
      for (let p = pairCount; p < delIndices.length; p++) {
        lines[delIndices[p]].wordTokens = [{ type: "del", value: lines[delIndices[p]].content }];
      }
      // Remaining unpaired added lines
      for (let p = pairCount; p < addIndices.length; p++) {
        lines[addIndices[p]].wordTokens = [{ type: "add", value: lines[addIndices[p]].content }];
      }
    } else {
      i++;
    }
  }
}

function buildHunkFromStrings(oldStr: string, newStr: string, header?: string): IDiffHunk {
  const rawDiff = diffLines(oldStr, newStr);
  const lines: IDiffLine[] = [];
  let oldLine = 1;
  let newLine = 1;

  for (const part of rawDiff) {
    const partLines = splitLines(part.value);
    for (const lineText of partLines) {
      if (part.removed) {
        lines.push({
          type: "del",
          oldLineNumber: oldLine++,
          content: lineText,
        });
      } else if (part.added) {
        lines.push({
          type: "add",
          newLineNumber: newLine++,
          content: lineText,
        });
      } else {
        lines.push({
          type: "ctx",
          oldLineNumber: oldLine++,
          newLineNumber: newLine++,
          content: lineText,
        });
      }
    }
  }

  applyWordLevelHighlights(lines);

  return {
    header,
    lines,
  };
}

function parseUnifiedPatch(patchText: string, fallbackPath: string): IFileModificationPayload {
  const lines = patchText.split("\n");
  let filePath = fallbackPath;
  const hunks: IDiffHunk[] = [];
  let currentHunk: IDiffHunk | null = null;
  let oldLine = 1;
  let newLine = 1;

  for (const rawLine of lines) {
    if (rawLine.startsWith("diff --git ")) {
      const match = rawLine.match(/diff --git a\/(.*) b\/(.*)/);
      if (match && match[2]) {
        filePath = match[2].trim();
      }
      continue;
    }
    if (rawLine.startsWith("+++ b/")) {
      filePath = rawLine.slice(6).trim();
      continue;
    }
    if (rawLine.startsWith("--- ")) {
      continue;
    }

    if (rawLine.startsWith("@@ ")) {
      if (currentHunk) {
        applyWordLevelHighlights(currentHunk.lines);
        hunks.push(currentHunk);
      }
      currentHunk = {
        header: rawLine.trim(),
        lines: [],
      };

      const match = rawLine.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (match) {
        oldLine = Number.parseInt(match[1], 10);
        newLine = Number.parseInt(match[2], 10);
      }
      continue;
    }

    if (!currentHunk) {
      continue;
    }

    if (rawLine.startsWith("-")) {
      currentHunk.lines.push({
        type: "del",
        oldLineNumber: oldLine++,
        content: rawLine.slice(1),
      });
    } else if (rawLine.startsWith("+")) {
      currentHunk.lines.push({
        type: "add",
        newLineNumber: newLine++,
        content: rawLine.slice(1),
      });
    } else if (rawLine.startsWith(" ")) {
      currentHunk.lines.push({
        type: "ctx",
        oldLineNumber: oldLine++,
        newLineNumber: newLine++,
        content: rawLine.slice(1),
      });
    } else if (rawLine.startsWith("\\")) {
      // e.g. \ No newline at end of file
      continue;
    } else if (rawLine === "") {
      // Empty line within a hunk is treated as context
      currentHunk.lines.push({
        type: "ctx",
        oldLineNumber: oldLine++,
        newLineNumber: newLine++,
        content: "",
      });
    }
  }

  if (currentHunk) {
    applyWordLevelHighlights(currentHunk.lines);
    hunks.push(currentHunk);
  }

  return {
    action: "EDIT",
    filePath,
    hunks,
  };
}

export function isFileModification(toolName?: string | null, toolInput?: Record<string, unknown> | null): boolean {
  if (!toolInput) {
    return false;
  }

  const normalizedTool = (toolName ?? "").toLowerCase();

  // Explicit tool name match
  const isNamedFileTool =
    normalizedTool === "fileedit" ||
    normalizedTool === "filewrite" ||
    normalizedTool === "edit" ||
    normalizedTool === "write" ||
    normalizedTool === "write_file" ||
    normalizedTool === "writefile" ||
    normalizedTool === "apply_patch" ||
    normalizedTool === "applypatch" ||
    normalizedTool === "str_replace_editor";

  // Check characteristic properties
  const hasPath =
    typeof toolInput.path === "string" ||
    typeof toolInput.filePath === "string" ||
    typeof toolInput.file_path === "string";

  const hasEditsArray = Array.isArray(toolInput.edits);
  const hasOldNew =
    typeof toolInput.old_string === "string" ||
    typeof toolInput.oldText === "string" ||
    typeof toolInput.old_str === "string";
  const hasPatch = typeof toolInput.patch === "string" || typeof toolInput.diff === "string";
  const hasContent = typeof toolInput.content === "string";

  if (isNamedFileTool && (hasPath || hasPatch || hasContent || hasEditsArray || hasOldNew)) {
    return true;
  }

  // Signature based match even if toolName is unrecognized
  if (hasPath && (hasEditsArray || hasOldNew || hasPatch || hasContent)) {
    return true;
  }

  if (hasPatch && (toolInput.patch as string).includes("@@")) {
    return true;
  }

  return false;
}

export function parseFileModification(
  toolName?: string | null,
  toolInput?: Record<string, unknown> | null,
): IFileModificationPayload | null {
  if (!toolInput || !isFileModification(toolName, toolInput)) {
    return null;
  }

  const normalizedTool = (toolName ?? "").toLowerCase();
  const filePath = String(toolInput.path || toolInput.filePath || toolInput.file_path || "untitled");

  // 1. Unified patch (Codex apply_patch or patch/diff input)
  const patchContent =
    typeof toolInput.patch === "string" ? toolInput.patch : typeof toolInput.diff === "string" ? toolInput.diff : null;

  if (patchContent && (patchContent.includes("@@") || patchContent.includes("diff --git"))) {
    return parseUnifiedPatch(patchContent, filePath);
  }

  // 2. Write operations (FileWrite, write, write_file, or content without edits)
  const isWriteTool =
    normalizedTool === "filewrite" ||
    normalizedTool === "write" ||
    normalizedTool === "write_file" ||
    normalizedTool === "writefile";
  const hasEdits =
    Array.isArray(toolInput.edits) ||
    typeof toolInput.old_string === "string" ||
    typeof toolInput.oldText === "string" ||
    typeof toolInput.old_str === "string";

  if ((isWriteTool || !hasEdits) && typeof toolInput.content === "string") {
    return {
      action: "WRITE",
      filePath,
      content: toolInput.content,
    };
  }

  // 3. Pi edits array format: edits: Array<{ oldText: string, newText: string }>
  if (Array.isArray(toolInput.edits) && toolInput.edits.length > 0) {
    const hunks: IDiffHunk[] = [];
    const edits = toolInput.edits as Array<Record<string, unknown>>;
    for (let idx = 0; idx < edits.length; idx++) {
      const edit = edits[idx];
      const oldText = typeof edit.oldText === "string" ? edit.oldText : "";
      const newText = typeof edit.newText === "string" ? edit.newText : "";
      const header = edits.length > 1 ? `Hunk ${idx + 1} of ${edits.length}` : undefined;
      hunks.push(buildHunkFromStrings(oldText, newText, header));
    }
    return {
      action: "EDIT",
      filePath,
      hunks,
    };
  }

  // 4. Claude Code / str_replace_editor format: old_string / new_string
  const oldString =
    typeof toolInput.old_string === "string"
      ? toolInput.old_string
      : typeof toolInput.oldText === "string"
        ? toolInput.oldText
        : typeof toolInput.old_str === "string"
          ? toolInput.old_str
          : "";

  const newString =
    typeof toolInput.new_string === "string"
      ? toolInput.new_string
      : typeof toolInput.newText === "string"
        ? toolInput.newText
        : typeof toolInput.new_str === "string"
          ? toolInput.new_str
          : "";

  return {
    action: "EDIT",
    filePath,
    hunks: [buildHunkFromStrings(oldString, newString)],
  };
}
