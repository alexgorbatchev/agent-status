import React, { useState } from "react";
import { Copy, Check, FileCode } from "lucide-react";
import { IconWrapper } from "./IconWrapper";
import type { IFileModificationPayload } from "../utils/diffUtils";

export interface IDiffViewerProps {
  payload: IFileModificationPayload;
}

export function DiffViewer({ payload }: IDiffViewerProps): React.JSX.Element {
  const [hasCopied, setHasCopied] = useState<boolean>(false);

  const handleCopy = async (): Promise<void> => {
    try {
      const textToCopy =
        payload.action === "WRITE" && typeof payload.content === "string" ? payload.content : payload.filePath;

      if (typeof navigator !== "undefined" && navigator.clipboard) {
        await navigator.clipboard.writeText(textToCopy);
        setHasCopied(true);
        setTimeout(() => setHasCopied(false), 2000);
      }
    } catch {
      // Clipboard copy failed or unavailable in headless environment
    }
  };

  const isWrite = payload.action === "WRITE";
  const badgeColorClass = isWrite
    ? "bg-emerald-950/80 text-emerald-400 border-emerald-800/60"
    : "bg-blue-950/80 text-blue-400 border-blue-800/60";

  return (
    <div
      data-testid="DiffViewer"
      className="mt-1 rounded-md border border-slate-800 bg-slate-950 overflow-hidden font-mono text-xs min-w-0 max-w-full"
    >
      {/* Target Header */}
      <div className="flex items-center justify-between px-3 py-1.5 bg-slate-900/90 border-b border-slate-800 text-slate-300 min-w-0">
        <div className="flex items-center space-x-2 min-w-0 flex-1 mr-2">
          <IconWrapper size="xs" tone="muted">
            <FileCode />
          </IconWrapper>
          <span
            data-testid="DiffViewer--badge"
            className={`px-1.5 py-0.5 rounded text-[10px] font-semibold border ${badgeColorClass} shrink-0`}
          >
            {payload.action}
          </span>
          <span
            data-testid="DiffViewer--path"
            title={payload.filePath}
            className="text-slate-200 font-mono text-[11px] truncate"
          >
            {payload.filePath}
          </span>
        </div>

        <button
          type="button"
          data-testid="DiffViewer--copy-btn"
          onClick={handleCopy}
          aria-label={isWrite ? "Copy file content" : "Copy file path"}
          title={isWrite ? "Copy file content" : "Copy file path"}
          className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors shrink-0 flex items-center space-x-1 text-[10px]"
        >
          {hasCopied ? (
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

      {/* Main Content View */}
      {isWrite ? (
        <div
          data-testid="DiffViewer--write-content"
          className="overflow-x-auto max-h-72 p-2 bg-slate-950 text-slate-300 text-[11px] font-mono whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full divide-y divide-transparent"
        >
          {payload.content ? (
            payload.content.split("\n").map((line, idx) => (
              <div key={`write-line-${idx}`} className="flex items-start hover:bg-slate-900/30">
                <span className="w-8 shrink-0 text-right select-none pr-2 text-slate-600 text-[10px]">{idx + 1}</span>
                <span className="flex-1 whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full">
                  {line}
                </span>
              </div>
            ))
          ) : (
            <span className="text-slate-500 italic p-2 block">(Empty file)</span>
          )}
        </div>
      ) : (
        <div className="overflow-x-auto max-h-80 bg-slate-950 min-w-0 max-w-full">
          {payload.hunks && payload.hunks.length > 0 ? (
            payload.hunks.map((hunk, hIdx) => (
              <div key={`hunk-${hIdx}`} className="border-b border-slate-800/40 last:border-b-0">
                {hunk.header && (
                  <div
                    data-testid="DiffViewer--hunk-header"
                    className="bg-slate-900/70 text-slate-400 text-[10px] font-mono px-3 py-1 border-y border-slate-800/60 select-none"
                  >
                    {hunk.header}
                  </div>
                )}
                <div className="divide-y divide-slate-900/20">
                  {hunk.lines.map((line, lIdx) => {
                    if (line.type === "del") {
                      return (
                        <div
                          key={`line-${lIdx}`}
                          data-testid="DiffViewer--del-line"
                          className="flex items-start bg-red-950/40 text-red-300 hover:bg-red-950/60 border-l-2 border-red-500 py-0.5 px-1 font-mono text-[11px]"
                        >
                          <span className="w-8 shrink-0 text-right select-none pr-2 text-red-400/60 text-[10px]">
                            {line.oldLineNumber ?? ""}
                          </span>
                          <span className="w-8 shrink-0 select-none" />
                          <span className="w-4 shrink-0 text-center select-none text-red-400 font-semibold">-</span>
                          <span className="flex-1 whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full">
                            {line.wordTokens
                              ? line.wordTokens.map((token, tIdx) => {
                                  if (token.type === "del") {
                                    return (
                                      <span
                                        key={`t-${tIdx}`}
                                        data-testid="DiffViewer--word-del"
                                        className="bg-red-500/30 text-red-200 rounded px-0.5"
                                      >
                                        {token.value}
                                      </span>
                                    );
                                  }
                                  return <span key={`t-${tIdx}`}>{token.value}</span>;
                                })
                              : line.content}
                          </span>
                        </div>
                      );
                    }

                    if (line.type === "add") {
                      return (
                        <div
                          key={`line-${lIdx}`}
                          data-testid="DiffViewer--add-line"
                          className="flex items-start bg-emerald-950/40 text-emerald-300 hover:bg-emerald-950/60 border-l-2 border-emerald-500 py-0.5 px-1 font-mono text-[11px]"
                        >
                          <span className="w-8 shrink-0 select-none" />
                          <span className="w-8 shrink-0 text-right select-none pr-2 text-emerald-400/60 text-[10px]">
                            {line.newLineNumber ?? ""}
                          </span>
                          <span className="w-4 shrink-0 text-center select-none text-emerald-400 font-semibold">+</span>
                          <span className="flex-1 whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full">
                            {line.wordTokens
                              ? line.wordTokens.map((token, tIdx) => {
                                  if (token.type === "add") {
                                    return (
                                      <span
                                        key={`t-${tIdx}`}
                                        data-testid="DiffViewer--word-add"
                                        className="bg-emerald-500/30 text-emerald-200 rounded px-0.5"
                                      >
                                        {token.value}
                                      </span>
                                    );
                                  }
                                  return <span key={`t-${tIdx}`}>{token.value}</span>;
                                })
                              : line.content}
                          </span>
                        </div>
                      );
                    }

                    return (
                      <div
                        key={`line-${lIdx}`}
                        data-testid="DiffViewer--ctx-line"
                        className="flex items-start text-slate-400 hover:bg-slate-900/40 border-l-2 border-transparent py-0.5 px-1 font-mono text-[11px]"
                      >
                        <span className="w-8 shrink-0 text-right select-none pr-2 text-slate-600 text-[10px]">
                          {line.oldLineNumber ?? ""}
                        </span>
                        <span className="w-8 shrink-0 text-right select-none pr-2 text-slate-600 text-[10px]">
                          {line.newLineNumber ?? ""}
                        </span>
                        <span className="w-4 shrink-0 text-center select-none text-slate-600">&nbsp;</span>
                        <span className="flex-1 whitespace-pre-wrap break-all [overflow-wrap:anywhere] min-w-0 max-w-full">
                          {line.content}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ))
          ) : (
            <div className="p-3 text-slate-500 italic text-[11px]">No differences detected</div>
          )}
        </div>
      )}
    </div>
  );
}
