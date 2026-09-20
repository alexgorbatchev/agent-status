import React from "react";
import clsx from "clsx";
import type { IAgentCardProps } from "./types";
import type { HarnessType } from "../types";
import { StatusBadge } from "./StatusBadge";
import { IconWrapper } from "./IconWrapper";
import { GitBranch, FolderGit2, Bot, Layers, DollarSign, HelpCircle, AlertTriangle, Clock } from "lucide-react";
import { useRelativeTime } from "../hooks/useRelativeTime";
import { formatExactTime } from "../utils/agentUtils";

const HARNESS_LABELS: Record<HarnessType, string> = {
  "claude-code": "Claude Code",
  pi: "Pi",
  codex: "Codex",
  opencode: "OpenCode",
};

export function AgentCard({ agent, onOpenDrawer }: IAgentCardProps): React.JSX.Element {
  const { activity, environment, workspace, harness } = agent;

  const formattedCost = activity.cost ? `$${activity.cost.total.toFixed(4)}` : "—";
  const title = activity.title || "Untitled Agent Session";
  const action = activity.action || (activity.activeTool ? `Executing ${activity.activeTool}` : "Idle");
  const isFinished = activity.lifecycle === "EXITED";
  const relativeTime = useRelativeTime(activity.updatedAt);
  const exactTime = formatExactTime(activity.updatedAt);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onOpenDrawer?.(agent);
    }
  };

  return (
    <div
      data-testid="AgentCard"
      role="button"
      tabIndex={0}
      aria-label={`View details for ${title}`}
      onClick={() => onOpenDrawer?.(agent)}
      onKeyDown={handleKeyDown}
      className={clsx(
        "group relative flex flex-col justify-between p-4 rounded-xl bg-slate-900/80 border border-slate-800 hover:border-slate-700 transition-all cursor-pointer shadow-lg hover:shadow-slate-950/50",
        isFinished && "opacity-60 hover:opacity-100",
      )}
    >
      <div data-testid="AgentCard--body">
        {/* Header Row: Harness / Env & Status */}
        <div data-testid="AgentCard--header" className="flex items-center justify-between gap-2 mb-2">
          <div className="flex items-center gap-2 text-xs text-slate-400">
            <span className="px-2 py-0.5 rounded bg-slate-800 text-slate-300 font-mono">{environment.label}</span>
            <span className="text-slate-600">•</span>
            <span className="flex items-center gap-1 font-medium text-slate-300">
              <IconWrapper size="sm">
                <Bot size={14} />
              </IconWrapper>
              {HARNESS_LABELS[harness] || harness}
            </span>
          </div>
          <StatusBadge lifecycle={activity.lifecycle} activity={activity.activity} />
        </div>

        {/* Title */}
        <h3
          data-testid="AgentCard--title"
          className="text-sm font-semibold text-slate-100 group-hover:text-blue-400 transition-colors line-clamp-1 mb-1"
        >
          {title}
        </h3>

        {/* Action Line */}
        <p data-testid="AgentCard--action" className="text-xs text-slate-400 line-clamp-2 min-h-[2rem] mb-3">
          {action}
        </p>

        {/* Attention Details (Question or Error) */}
        {activity.activity === "WAITING_QUESTION" && activity.question && (
          <div
            data-testid="AgentCard--question"
            className="mb-3 p-2 rounded-lg bg-amber-950/40 border border-amber-800/60 text-xs text-amber-200 flex items-start gap-1.5"
          >
            <IconWrapper size="md" tone="warning" align="top">
              <HelpCircle size={16} />
            </IconWrapper>
            <span className="line-clamp-2">{activity.question}</span>
          </div>
        )}

        {activity.activity === "ERROR" && activity.lastError && (
          <div
            data-testid="AgentCard--error"
            className="mb-3 p-2 rounded-lg bg-red-950/40 border border-red-800/60 text-xs text-red-200 flex items-start gap-1.5"
          >
            <IconWrapper size="md" tone="error" align="top">
              <AlertTriangle size={16} />
            </IconWrapper>
            <span className="line-clamp-2 font-mono text-[11px]">{activity.lastError}</span>
          </div>
        )}

        {/* Metadata Badges */}
        <div data-testid="AgentCard--badges" className="flex flex-wrap items-center gap-2 text-xs text-slate-400 mb-3">
          <div className="flex items-center gap-1 bg-slate-800/50 px-2 py-0.5 rounded border border-slate-700/50">
            <IconWrapper size="xs" tone="muted">
              <GitBranch size={12} />
            </IconWrapper>
            <span className="font-mono text-[11px] text-slate-300">{workspace.branch || "HEAD"}</span>
          </div>

          {workspace.isWorktree && workspace.relPath && (
            <div className="flex items-center gap-1 bg-slate-800/50 px-2 py-0.5 rounded border border-slate-700/50">
              <IconWrapper size="xs" tone="muted">
                <FolderGit2 size={12} />
              </IconWrapper>
              <span className="font-mono text-[11px] text-slate-300">{workspace.relPath}</span>
            </div>
          )}

          {activity.subagentCount > 0 && (
            <div className="flex items-center gap-1 bg-purple-950/40 border border-purple-800/50 text-purple-300 px-2 py-0.5 rounded text-[11px]">
              <IconWrapper size="xs" tone="purple">
                <Layers size={12} />
              </IconWrapper>
              <span>
                {activity.subagentCount} subagent{activity.subagentCount > 1 ? "s" : ""}
              </span>
            </div>
          )}

          <div
            data-testid="AgentCard--last-active"
            className="flex items-center gap-1 bg-slate-800/50 px-2 py-0.5 rounded border border-slate-700/50"
            title={`Last active: ${exactTime}`}
          >
            <IconWrapper size="xs" tone="muted">
              <Clock size={12} />
            </IconWrapper>
            <span className="font-mono text-[11px] text-slate-300">{relativeTime}</span>
          </div>
        </div>
      </div>

      {/* Footer: Usage & Spend */}
      <div
        data-testid="AgentCard--footer"
        className="flex items-center justify-between pt-2 border-t border-slate-800 text-xs text-slate-400"
      >
        <div className="flex items-center gap-1 font-mono text-emerald-400">
          <IconWrapper size="sm">
            <DollarSign size={14} />
          </IconWrapper>
          <span>{formattedCost}</span>
        </div>

        <div className="text-[11px] text-slate-500 font-mono">{activity.usage.total.toLocaleString()} tokens</div>
      </div>
    </div>
  );
}
