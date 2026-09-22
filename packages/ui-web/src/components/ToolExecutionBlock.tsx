import React, { useState } from "react";
import { Terminal, Copy, Check } from "lucide-react";
import type { IToolExecutionBlockProps } from "./types";
import { AnsiText } from "./AnsiText";
import { DiffViewer } from "./DiffViewer";
import { IconWrapper } from "./IconWrapper";
import { isFileModification, parseFileModification } from "../utils/diffUtils";

const SHELL_TOOLS = new Set(["bash", "sh", "exec", "run_command", "shell", "terminal"]);
const PARAM_TOOLS = new Set([
  "grep",
  "find",
  "glob",
  "search_files",
  "file_search",
  "ripgrep",
  "web_search",
  "fetch_content",
  "fetch",
]);

function getShellCommand(toolName?: string | null, toolInput?: Record<string, unknown> | null): string | null {
  if (!toolName || !toolInput) return null;
  if (!SHELL_TOOLS.has(toolName.toLowerCase())) return null;

  if (typeof toolInput.command === "string") return toolInput.command;
  if (typeof toolInput.cmd === "string") return toolInput.cmd;
  if (typeof toolInput.script === "string") return toolInput.script;

  return null;
}

function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  if (ms < 60000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }
  const mins = Math.floor(ms / 60000);
  const secs = Math.floor((ms % 60000) / 1000);
  return `${mins}m ${secs}s`;
}

export function ToolExecutionBlock({
  toolName,
  toolInput,
  fileModification,
  toolOutput,
  exitCode,
  isError,
  toolCallId: _toolCallId,
  durationMs,
}: IToolExecutionBlockProps): React.JSX.Element {
  const [hasCopiedCommand, setHasCopiedCommand] = useState<boolean>(false);
  const [hasCopiedOutput, setHasCopiedOutput] = useState<boolean>(false);

  const fileDiffPayload = isFileModification(toolName, toolInput, fileModification)
    ? parseFileModification(toolName, toolInput, fileModification)
    : null;
  const shellCommand = getShellCommand(toolName, toolInput);
  const isParamTool = Boolean(toolName && PARAM_TOOLS.has(toolName.toLowerCase()));

  const hasStatus = (exitCode !== undefined && exitCode !== null) || (isError !== undefined && isError !== null);
  const isFailure = isError === true || (exitCode !== null && exitCode !== undefined && exitCode !== 0);

  const statusText =
    exitCode !== null && exitCode !== undefined ? `exit ${exitCode}` : isFailure ? "failed" : "success";

  const statusBadgeClass = isFailure
    ? "bg-red-950/80 text-red-400 border-red-800/60"
    : "bg-emerald-950/80 text-emerald-400 border-emerald-800/60";

  const handleCopyCommand = async (): Promise<void> => {
    if (!shellCommand) return;
    try {
      if (typeof navigator !== "undefined" && navigator.clipboard) {
        await navigator.clipboard.writeText(shellCommand);
        setHasCopiedCommand(true);
        setTimeout(() => setHasCopiedCommand(false), 2000);
      }
    } catch {
      // Clipboard copy unavailable
    }
  };

  const handleCopyOutput = async (): Promise<void> => {
    if (!toolOutput) return;
    try {
      if (typeof navigator !== "undefined" && navigator.clipboard) {
        await navigator.clipboard.writeText(toolOutput);
        setHasCopiedOutput(true);
        setTimeout(() => setHasCopiedOutput(false), 2000);
      }
    } catch {
      // Clipboard copy unavailable
    }
  };

  const outputLines = toolOutput ? toolOutput.split("\n") : [];
  const lineCount = outputLines.length;
  const streamType = isFailure ? "STDERR" : "OUTPUT";

  const renderDurationBadge = (): React.JSX.Element | null => {
    if (durationMs === undefined || durationMs === null) return null;
    return (
      <span
        data-testid="ToolExecutionBlock--duration"
        className="px-1.5 py-0.5 rounded text-[10px] bg-slate-800/80 text-slate-300 border border-slate-700/60 font-mono"
      >
        {formatDuration(durationMs)}
      </span>
    );
  };

  const renderStatusBadge = (): React.JSX.Element | null => {
    if (!hasStatus) return null;
    return (
      <span
        data-testid="ToolExecutionBlock--status"
        className={`px-1.5 py-0.5 rounded text-[10px] font-semibold border ${statusBadgeClass} font-mono`}
      >
        {statusText}
      </span>
    );
  };

  return (
    <div data-testid="ToolExecutionBlock" className="mt-1 font-mono min-w-0 max-w-full overflow-hidden">
      {/* File modification handling */}
      {fileDiffPayload ? (
        <>
          {toolName && (
            <div className="flex items-center justify-between mb-1 min-w-0">
              <span data-testid="ToolExecutionBlock--name" className="text-amber-300 font-semibold">
                {toolName}
              </span>
              <div className="flex items-center space-x-1.5 shrink-0">
                {renderDurationBadge()}
                {renderStatusBadge()}
              </div>
            </div>
          )}
          <DiffViewer payload={fileDiffPayload} />
        </>
      ) : shellCommand !== null ? (
        /* Shell terminal view */
        <div
          data-testid="ToolExecutionBlock--terminal"
          className="rounded-md border border-slate-800 bg-slate-950 overflow-hidden min-w-0 max-w-full"
        >
          {/* Terminal Header */}
          <div className="flex items-center justify-between px-2.5 py-1.5 bg-slate-900/90 border-b border-slate-800 text-slate-300 min-w-0">
            <div className="flex items-center space-x-2 min-w-0">
              <IconWrapper size="xs" tone="muted">
                <Terminal />
              </IconWrapper>
              <span data-testid="ToolExecutionBlock--name" className="text-amber-300 font-semibold text-[11px]">
                {toolName}
              </span>
            </div>
            <div className="flex items-center space-x-2 shrink-0">
              {renderDurationBadge()}
              {renderStatusBadge()}
              <button
                type="button"
                data-testid="ToolExecutionBlock--copy-command-btn"
                onClick={handleCopyCommand}
                aria-label="Copy command"
                title="Copy command"
                className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors flex items-center space-x-1 text-[10px]"
              >
                {hasCopiedCommand ? (
                  <>
                    <IconWrapper size="xs" tone="success">
                      <Check />
                    </IconWrapper>
                    <span className="text-emerald-400 text-[10px]">Copied!</span>
                  </>
                ) : (
                  <>
                    <IconWrapper size="xs" tone="muted">
                      <Copy />
                    </IconWrapper>
                    <span className="text-slate-400 text-[10px]">Copy</span>
                  </>
                )}
              </button>
            </div>
          </div>
          {/* Terminal Command */}
          <pre
            data-testid="ToolExecutionBlock--input"
            className="p-2.5 bg-slate-950 text-slate-200 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full"
          >
            <span className="text-emerald-400 select-none mr-1.5 font-bold" aria-hidden="true">
              $
            </span>
            {shellCommand}
          </pre>
        </div>
      ) : (
        /* Standard tool input / parameters */
        <>
          {(toolName || hasStatus || durationMs !== undefined) && (
            <div className="flex items-center justify-between mb-1 min-w-0">
              {toolName ? (
                <span data-testid="ToolExecutionBlock--name" className="text-amber-300 font-semibold">
                  {toolName}
                </span>
              ) : (
                <span />
              )}
              <div className="flex items-center space-x-1.5 shrink-0">
                {renderDurationBadge()}
                {renderStatusBadge()}
              </div>
            </div>
          )}

          {toolInput &&
            (isParamTool ? (
              <div data-testid="ToolExecutionBlock--params" className="flex flex-wrap gap-1.5 mt-1 min-w-0 max-w-full">
                {Object.entries(toolInput).map(([key, val]) => (
                  <span
                    key={key}
                    className="inline-flex items-center px-2 py-0.5 rounded bg-slate-900 border border-slate-800 text-[11px] font-mono text-slate-300"
                  >
                    <span className="text-slate-400 mr-1.5 font-semibold">{key}:</span>
                    <span className="text-slate-200">{typeof val === "string" ? val : JSON.stringify(val)}</span>
                  </span>
                ))}
              </div>
            ) : (
              <pre
                data-testid="ToolExecutionBlock--input"
                className="mt-1 p-2 rounded bg-slate-950 text-slate-400 text-[10px] overflow-x-auto whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full"
              >
                {JSON.stringify(toolInput, null, 2)}
              </pre>
            ))}
        </>
      )}

      {/* Enhanced Output Viewer */}
      {toolOutput && (
        <div className="mt-1.5 rounded border border-slate-800/80 bg-slate-950/60 overflow-hidden min-w-0 max-w-full">
          <div
            data-testid="ToolExecutionBlock--output-header"
            className="flex items-center justify-between px-2.5 py-1 bg-slate-900/80 border-b border-slate-800 text-slate-400 text-[10px]"
          >
            <div className="flex items-center space-x-2">
              <span
                data-testid="ToolExecutionBlock--stream-type"
                className="px-1.5 py-0.5 rounded font-semibold font-mono uppercase bg-slate-800 text-slate-300 text-[9px]"
              >
                {streamType}
              </span>
              <span
                data-testid="ToolExecutionBlock--line-count"
                className="px-1.5 py-0.5 rounded bg-slate-800/70 text-slate-400 font-mono text-[9px]"
              >
                {`${lineCount} ${lineCount === 1 ? "line" : "lines"}`}
              </span>
            </div>
            <button
              type="button"
              data-testid="ToolExecutionBlock--copy-output-btn"
              onClick={handleCopyOutput}
              aria-label="Copy output"
              title="Copy output"
              className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors flex items-center space-x-1 text-[10px]"
            >
              {hasCopiedOutput ? (
                <>
                  <IconWrapper size="xs" tone="success">
                    <Check />
                  </IconWrapper>
                  <span className="text-emerald-400 text-[10px]">Copied!</span>
                </>
              ) : (
                <>
                  <IconWrapper size="xs" tone="muted">
                    <Copy />
                  </IconWrapper>
                  <span className="text-slate-400 text-[10px]">Copy</span>
                </>
              )}
            </button>
          </div>

          <div
            data-testid="ToolExecutionBlock--output-scroll-container"
            className="flex min-w-0 max-w-full overflow-auto max-h-64"
          >
            {lineCount > 1 && (
              <div
                data-testid="ToolExecutionBlock--line-gutter"
                aria-hidden="true"
                className="select-none py-2 pl-2.5 pr-2 text-right text-slate-600 bg-slate-950 border-r border-slate-800/80 shrink-0 font-mono text-[10px] leading-relaxed sticky left-0 z-10"
              >
                {outputLines.map((_, idx) => (
                  <div key={idx}>{idx + 1}</div>
                ))}
              </div>
            )}
            <pre
              data-testid="ToolExecutionBlock--output"
              className="flex-1 p-2 rounded-r bg-slate-950/80 text-slate-300 font-mono text-[10px] whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full leading-relaxed"
            >
              <AnsiText text={toolOutput} />
            </pre>
          </div>
        </div>
      )}
    </div>
  );
}
