import React from "react";
import type { IToolExecutionBlockProps } from "./types";
import { AnsiText } from "./AnsiText";
import { DiffViewer } from "./DiffViewer";
import { isFileModification, parseFileModification } from "../utils/diffUtils";

export function ToolExecutionBlock({ toolName, toolInput, toolOutput }: IToolExecutionBlockProps): React.JSX.Element {
  const fileDiffPayload = isFileModification(toolName, toolInput) ? parseFileModification(toolName, toolInput) : null;

  return (
    <div data-testid="ToolExecutionBlock" className="mt-1 font-mono min-w-0 max-w-full overflow-hidden">
      {toolName && (
        <span data-testid="ToolExecutionBlock--name" className="text-amber-300 font-semibold">
          {toolName}
        </span>
      )}
      {fileDiffPayload ? (
        <DiffViewer payload={fileDiffPayload} />
      ) : (
        toolInput && (
          <pre
            data-testid="ToolExecutionBlock--input"
            className="mt-1 p-2 rounded bg-slate-950 text-slate-400 text-[10px] overflow-x-auto whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full"
          >
            {JSON.stringify(toolInput, null, 2)}
          </pre>
        )
      )}
      {toolOutput && (
        <pre
          data-testid="ToolExecutionBlock--output"
          className="mt-1 p-2 rounded bg-slate-950/80 text-slate-300 font-mono text-[10px] overflow-x-auto max-h-40 whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full"
        >
          <AnsiText text={toolOutput} />
        </pre>
      )}
    </div>
  );
}
