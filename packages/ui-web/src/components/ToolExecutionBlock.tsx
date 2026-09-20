import React from "react";
import type { IToolExecutionBlockProps } from "./types";

export function ToolExecutionBlock({ toolName, toolInput, toolOutput }: IToolExecutionBlockProps): React.JSX.Element {
  return (
    <div data-testid="ToolExecutionBlock" className="mt-1 font-mono">
      <span data-testid="ToolExecutionBlock--name" className="text-amber-300 font-semibold">
        {toolName}
      </span>
      {toolInput && (
        <pre
          data-testid="ToolExecutionBlock--input"
          className="mt-1 p-2 rounded bg-slate-950 text-slate-400 text-[10px] overflow-x-auto"
        >
          {JSON.stringify(toolInput, null, 2)}
        </pre>
      )}
      {toolOutput && (
        <div
          data-testid="ToolExecutionBlock--output"
          className="mt-1 p-2 rounded bg-slate-950/80 text-slate-300 font-mono text-[10px] overflow-x-auto max-h-40"
        >
          {toolOutput}
        </div>
      )}
    </div>
  );
}
