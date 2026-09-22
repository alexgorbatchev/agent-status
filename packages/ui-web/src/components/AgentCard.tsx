import React from "react";
import clsx from "clsx";
import type { IAgentCardProps } from "./types";
import type { HarnessType } from "../types";
import { StatusBadge } from "./StatusBadge";
import { IconWrapper } from "./IconWrapper";
import {
  GitBranch,
  FolderGit2,
  Bot,
  Layers,
  DollarSign,
  HelpCircle,
  AlertTriangle,
  Clock,
  Eye,
  EyeOff,
} from "lucide-react";
import { useRelativeTime } from "../hooks/useRelativeTime";
import {
  formatExactTime,
  getToolExecutionDuration,
  getToolStallTier,
  formatDuration,
  isSessionStalled,
} from "../utils/agentUtils";

const HARNESS_LABELS: Record<HarnessType, string> = {
  "claude-code": "Claude Code",
  pi: "Pi",
  codex: "Codex",
  opencode: "OpenCode",
};

function resolveCurrentTime(): number {
  return Date.now();
}

export function AgentCard({ agent, onOpenDrawer, onToggleHide }: IAgentCardProps): React.JSX.Element {
  const { activity, environment, workspace, harness } = agent;

  const formattedCost = activity.cost ? `$${activity.cost.total.toFixed(4)}` : "—";
  const title = activity.title || "Untitled Agent Session";
  const action = activity.action || (activity.activeTool ? `Executing ${activity.activeTool}` : "Idle");
  const isFinished = activity.lifecycle === "EXITED";
  const now = resolveCurrentTime();
  const relativeTime = useRelativeTime(activity.updatedAt);
  const exactTime = formatExactTime(activity.updatedAt);

  const durationMs = getToolExecutionDuration(activity, now);
  const stallTier = getToolStallTier(durationMs);
  const formattedDuration = durationMs !== null ? formatDuration(durationMs) : null;
  const isStalled = stallTier === "stalled";

  const isRunningBusy = activity.lifecycle === "RUNNING" && activity.activity === "BUSY";
  const sessionStalled = isRunningBusy && isSessionStalled(activity.updatedAt, now);
  const silenceMs = Math.max(0, now - activity.updatedAt);
  const silenceMinutes = Math.floor(silenceMs / 60000);
  const silenceDuration = `${silenceMinutes}m`;

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.target !== event.currentTarget) {
      return;
    }
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
        "group relative flex flex-col justify-between p-4 rounded-xl bg-slate-900/80 border transition-all cursor-pointer shadow-lg hover:shadow-slate-950/50",
        isStalled || sessionStalled
          ? "border-amber-500/80 shadow-amber-950/40"
          : "border-slate-800 hover:border-slate-700",
        agent.isHidden && "border-dashed border-slate-700/60 opacity-60 hover:opacity-100",
        !agent.isHidden && isFinished && "opacity-60 hover:opacity-100",
      )}
    >
      <div data-testid="AgentCard--body">
        {/* Header Row: Harness & Status */}
        <div data-testid="AgentCard--header" className="flex items-center justify-between gap-2 mb-2">
          <div className="flex items-center gap-2 text-xs text-slate-400">
            <span
              data-testid="AgentCard--harness-badge"
              role="status"
              aria-label={HARNESS_LABELS[harness] || harness}
              className="flex items-center gap-1 font-medium text-slate-300"
            >
              <IconWrapper size="sm">
                <Bot size={14} />
              </IconWrapper>
              {HARNESS_LABELS[harness] || harness}
            </span>
            {onToggleHide && (
              <button
                type="button"
                data-testid={agent.isHidden ? "AgentCard--unhide-button" : "AgentCard--hide-button"}
                title={agent.isHidden ? "Unhide agent session" : "Hide agent session"}
                aria-label={agent.isHidden ? "Unhide agent session" : "Hide agent session"}
                onClick={(event: React.MouseEvent<HTMLButtonElement>) => {
                  event.stopPropagation();
                  onToggleHide(agent);
                }}
                onKeyDown={(event: React.KeyboardEvent<HTMLButtonElement>) => {
                  event.stopPropagation();
                }}
                className="p-1 rounded hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors focus:outline-none focus:ring-1 focus:ring-slate-500 cursor-pointer"
              >
                <IconWrapper size="xs" tone="muted">
                  {agent.isHidden ? <Eye size={13} /> : <EyeOff size={13} />}
                </IconWrapper>
              </button>
            )}
          </div>
          <div className="flex items-center gap-1.5">
            {agent.isHidden && (
              <span
                data-testid="AgentCard--hidden-badge"
                role="status"
                className="px-1.5 py-0.5 rounded bg-slate-800 text-slate-400 text-[10px] font-medium border border-slate-700 font-mono"
              >
                Hidden
              </span>
            )}
            <StatusBadge
              lifecycle={activity.lifecycle}
              activity={activity.activity}
              stallTier={stallTier}
              toolDuration={formattedDuration ?? undefined}
              isSessionStalled={sessionStalled}
              silenceDuration={sessionStalled ? silenceDuration : undefined}
            />
          </div>
        </div>

        {/* Title */}
        <h3
          data-testid="AgentCard--title"
          className="text-sm font-semibold text-slate-100 group-hover:text-blue-400 transition-colors line-clamp-1 mb-1"
        >
          {title}
        </h3>

        {/* Action Line */}
        <p
          data-testid="AgentCard--action"
          data-action
          className="text-xs text-slate-400 line-clamp-2 min-h-[2rem] mb-3"
        >
          {action}
        </p>

        {/* Attention Details (Question or Error) */}
        {activity.activity === "WAITING_QUESTION" && activity.question && (
          <div
            data-testid="AgentCard--question"
            role="alert"
            data-callout-type="warning"
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
            role="alert"
            data-callout-type="error"
            className="mb-3 p-2 rounded-lg bg-red-950/40 border border-red-800/60 text-xs text-red-200 flex items-start gap-1.5"
          >
            <IconWrapper size="md" tone="error" align="top">
              <AlertTriangle size={16} />
            </IconWrapper>
            <span className="line-clamp-2 font-mono text-[11px]">{activity.lastError}</span>
          </div>
        )}

        {(stallTier === "warning" || stallTier === "stalled") && formattedDuration && (
          <div
            data-testid="AgentCard--stall-warning"
            role="alert"
            data-callout-type="warning"
            className="mb-3 p-2 rounded-lg bg-amber-950/40 border border-amber-800/60 text-xs text-amber-200 flex flex-col gap-1"
          >
            <div className="flex items-center gap-1.5 font-medium">
              <IconWrapper size="md" tone="warning" align="top">
                <AlertTriangle size={16} />
              </IconWrapper>
              <span>Command running for {formattedDuration} without output or return</span>
            </div>
            {activity.action && (
              <div className="font-mono text-[11px] text-amber-300/80 truncate pl-5">Command: {activity.action}</div>
            )}
          </div>
        )}

        {/* Metadata Badges */}
        <div data-testid="AgentCard--badges" className="flex flex-wrap items-center gap-2 text-xs text-slate-400 mb-3">
          <div
            data-testid="AgentCard--env-badge"
            data-badge-type="env"
            role="status"
            aria-label={environment.label}
            className="flex items-center gap-1 bg-slate-800/50 px-2 py-0.5 rounded border border-slate-700/50"
          >
            <span className="font-mono text-[11px] text-slate-300">{environment.label}</span>
          </div>

          <div
            data-testid="AgentCard--branch-badge"
            data-badge-type="branch"
            role="status"
            aria-label={`branch: ${workspace.branch || "HEAD"}`}
            className="flex items-center gap-1 bg-slate-800/50 px-2 py-0.5 rounded border border-slate-700/50"
          >
            <IconWrapper size="xs" tone="muted">
              <GitBranch size={12} />
            </IconWrapper>
            <span className="font-mono text-[11px] text-slate-300">{workspace.branch || "HEAD"}</span>
          </div>

          {workspace.isWorktree && workspace.relPath && (
            <div
              data-testid="AgentCard--worktree-badge"
              data-badge-type="worktree"
              role="status"
              aria-label={`worktree: ${workspace.relPath}`}
              className="flex items-center gap-1 bg-slate-800/50 px-2 py-0.5 rounded border border-slate-700/50"
            >
              <IconWrapper size="xs" tone="muted">
                <FolderGit2 size={12} />
              </IconWrapper>
              <span className="font-mono text-[11px] text-slate-300">{workspace.relPath}</span>
            </div>
          )}

          {activity.subagentCount > 0 && (
            <div
              data-testid="AgentCard--subagents-badge"
              data-badge-type="subagents"
              role="status"
              aria-label={`${activity.subagentCount} subagent${activity.subagentCount > 1 ? "s" : ""}`}
              className="flex items-center gap-1 bg-purple-950/40 border border-purple-800/50 text-purple-300 px-2 py-0.5 rounded text-[11px]"
            >
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
            role="status"
            aria-label={
              sessionStalled ? `silent: ${silenceDuration}` : isRunningBusy ? `active: ${relativeTime}` : relativeTime
            }
            className="flex items-center gap-1 bg-slate-800/50 px-2 py-0.5 rounded border border-slate-700/50"
            title={`Last active: ${exactTime}`}
          >
            <IconWrapper size="xs" tone={sessionStalled ? "warning" : "muted"}>
              <Clock size={12} />
            </IconWrapper>
            {sessionStalled ? (
              <span className="font-mono text-[11px] text-amber-300">Silent for {silenceDuration}</span>
            ) : isRunningBusy ? (
              <span className="font-mono text-[11px] text-slate-300">Active {relativeTime}</span>
            ) : (
              <span className="font-mono text-[11px] text-slate-300">{relativeTime}</span>
            )}
          </div>

          {stallTier === "notice" && formattedDuration && (
            <div
              data-testid="AgentCard--stall-notice"
              data-badge-type="stall-notice"
              role="status"
              aria-label={`${activity.activeTool} running for ${formattedDuration}`}
              className="flex items-center gap-1 bg-amber-950/40 border border-amber-800/50 text-amber-300 px-2 py-0.5 rounded text-[11px]"
            >
              <IconWrapper size="xs" tone="warning">
                <Clock size={12} />
              </IconWrapper>
              <span>
                {activity.activeTool} running for {formattedDuration}
              </span>
            </div>
          )}

          {(stallTier === "warning" || stallTier === "stalled") && formattedDuration && (
            <div
              data-testid="AgentCard--stall-duration"
              data-badge-type="stall-duration"
              role="status"
              aria-label={`${formattedDuration} on tool`}
              className="flex items-center gap-1 bg-amber-950/40 border border-amber-800/50 text-amber-300 px-2 py-0.5 rounded text-[11px]"
            >
              <IconWrapper size="xs" tone="warning">
                <Clock size={12} />
              </IconWrapper>
              <span>{formattedDuration} on tool</span>
            </div>
          )}
        </div>
      </div>

      {/* Footer: Usage & Spend */}
      <div
        data-testid="AgentCard--footer"
        className="flex items-center justify-between pt-2 border-t border-slate-800 text-xs text-slate-400"
      >
        <div
          data-testid="AgentCard--cost"
          data-metric="cost"
          aria-label={formattedCost}
          className="flex items-center gap-1 font-mono text-emerald-400"
        >
          <IconWrapper size="sm">
            <DollarSign size={14} />
          </IconWrapper>
          <span>{formattedCost}</span>
        </div>

        <div
          data-testid="AgentCard--usage"
          data-metric="usage"
          aria-label={`${activity.usage.total.toLocaleString()} tokens`}
          className="text-[11px] text-slate-500 font-mono"
        >
          {activity.usage.total.toLocaleString()} tokens
        </div>
      </div>
    </div>
  );
}
