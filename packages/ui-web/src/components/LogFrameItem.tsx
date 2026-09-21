import React, { useMemo } from "react";
import type { ILogFrameItemProps } from "./types";
import { Brain, Wrench, AlertTriangle, User, Play, CheckCircle2 } from "lucide-react";
import Markdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import { ToolExecutionBlock } from "./ToolExecutionBlock";
import { TokenUsageBar } from "./TokenUsageBar";
import { rehypeAnsi } from "../utils/rehypeAnsi";
import { preprocessMarkdownAnsi } from "../utils/ansiUtils";

interface IMarkdownPreProps extends React.ComponentPropsWithoutRef<"pre"> {
  node?: unknown;
}

interface IMarkdownCodeProps extends React.ComponentPropsWithoutRef<"code"> {
  node?: unknown;
}

interface IMarkdownTableProps extends React.ComponentPropsWithoutRef<"table"> {
  node?: unknown;
}

const markdownComponents = {
  pre: ({ node: _node, className, ...props }: IMarkdownPreProps) => (
    <pre
      className={`min-w-0 max-w-full overflow-x-auto [overflow-wrap:anywhere] break-all ${className ?? ""}`}
      {...props}
    />
  ),
  code: ({ node: _node, className, ...props }: IMarkdownCodeProps) => (
    <code className={`min-w-0 max-w-full [overflow-wrap:anywhere] break-all ${className ?? ""}`} {...props} />
  ),
  table: ({ node: _node, className, ...props }: IMarkdownTableProps) => (
    <div className="min-w-0 max-w-full overflow-x-auto my-2">
      <table className={`min-w-0 max-w-full [overflow-wrap:anywhere] ${className ?? ""}`} {...props} />
    </div>
  ),
};

export function LogFrameItem({ frame }: ILogFrameItemProps): React.JSX.Element {
  const isSubagent = Boolean(frame.subagentId);
  const isUser = (frame.data.role === "user" || frame.eventType === "turn_start") && frame.eventType !== "tool_result";

  const renderIcon = (): React.JSX.Element | null => {
    if (isUser) {
      return (
        <span className="w-3.5 h-3.5 text-blue-400 flex items-center justify-center">
          <User size={14} />
        </span>
      );
    }
    switch (frame.eventType) {
      case "thinking":
        return (
          <span className="w-3.5 h-3.5 text-blue-400 flex items-center justify-center">
            <Brain size={14} />
          </span>
        );
      case "tool_call":
      case "tool_result":
        return (
          <span className="w-3.5 h-3.5 text-amber-400 flex items-center justify-center">
            <Wrench size={14} />
          </span>
        );
      case "error":
        return (
          <span className="w-3.5 h-3.5 text-red-400 flex items-center justify-center">
            <AlertTriangle size={14} />
          </span>
        );
      case "run_started":
        return (
          <span className="w-3.5 h-3.5 text-emerald-400 flex items-center justify-center">
            <Play size={14} />
          </span>
        );
      case "turn_end":
        return (
          <span className="w-3.5 h-3.5 text-slate-400 flex items-center justify-center">
            <CheckCircle2 size={14} />
          </span>
        );
      default:
        return null;
    }
  };

  const containerClasses = isUser
    ? "bg-blue-950/20 border-blue-800/40"
    : isSubagent
      ? "bg-purple-950/20 border-purple-900/40 ml-4"
      : "bg-slate-900/80 border-slate-800";

  const remarkPlugins = useMemo(() => [remarkBreaks], []);
  const rehypePlugins = useMemo(() => [rehypeAnsi], []);
  const processedContent = useMemo(
    () => (frame.data.content ? preprocessMarkdownAnsi(frame.data.content) : null),
    [frame.data.content],
  );
  const processedQuestion = useMemo(
    () => (frame.data.question ? preprocessMarkdownAnsi(frame.data.question) : null),
    [frame.data.question],
  );

  return (
    <div
      data-testid="LogFrameItem"
      className={`p-3 rounded-lg border transition-all min-w-0 max-w-full overflow-hidden ${containerClasses}`}
    >
      <div className="flex items-center justify-between gap-2 mb-1 text-[11px] text-slate-400 font-mono">
        <div className="flex items-center gap-1.5">
          {renderIcon()}
          {isSubagent && (
            <span
              data-testid="LogFrameItem--subagent"
              className="px-1.5 py-0.5 rounded bg-purple-900/50 text-purple-300 text-[10px]"
            >
              {`Subagent: ${frame.subagentId}`}
            </span>
          )}
          <span data-testid="LogFrameItem--type" className="font-semibold text-slate-300 uppercase">
            {isUser ? "USER PROMPT" : frame.eventType}
          </span>
        </div>
        <span data-testid="LogFrameItem--timestamp" className="text-slate-500">
          {new Date(frame.timestamp).toLocaleTimeString()}
        </span>
      </div>

      {processedContent && (
        <div
          data-testid="LogFrameItem--content"
          className="text-slate-300 prose prose-invert max-w-none text-xs leading-relaxed break-words space-y-1 min-w-0 max-w-full [overflow-wrap:anywhere]"
        >
          <Markdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={markdownComponents}>
            {processedContent}
          </Markdown>
        </div>
      )}

      {processedQuestion && (
        <div
          data-testid="LogFrameItem--question"
          className="mt-1 p-2 rounded bg-amber-950/30 border border-amber-800/40 text-amber-200 min-w-0 max-w-full"
        >
          <span className="font-semibold text-[11px] block mb-1">Question:</span>
          <Markdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={markdownComponents}>
            {processedQuestion}
          </Markdown>
        </div>
      )}

      {(frame.data.toolName || frame.data.toolOutput || frame.data.toolInput) && (
        <ToolExecutionBlock
          toolName={frame.data.toolName}
          toolInput={frame.data.toolInput}
          toolOutput={frame.data.toolOutput}
        />
      )}

      {frame.data.usage && <TokenUsageBar usage={frame.data.usage} />}
    </div>
  );
}
