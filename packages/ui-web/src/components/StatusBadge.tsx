import React from "react";
import type { IStatusBadgeProps, IStatusBadgeConfig } from "./types";
import type { LifecycleState, ActivityState } from "../types";
import { IconWrapper } from "./IconWrapper";
import { AlertTriangle } from "lucide-react";

const STATUS_CONFIGS: Record<LifecycleState, Record<ActivityState, IStatusBadgeConfig>> = {
  RUNNING: {
    BUSY: {
      label: "Working",
      badgeClass: "bg-blue-900/60 text-blue-200 border border-blue-700/50",
      indicatorClass: "bg-blue-400 animate-pulse",
    },
    WAITING_QUESTION: {
      label: "Waiting on Question",
      badgeClass: "bg-amber-900/60 text-amber-200 border border-amber-600 animate-pulse",
      indicatorClass: "bg-amber-400",
    },
    ERROR: {
      label: "Error",
      badgeClass: "bg-red-900/60 text-red-200 border border-red-600 animate-pulse",
      indicatorClass: "bg-red-400",
    },
    IDLE: {
      label: "Idle",
      badgeClass: "bg-slate-800 text-slate-300 border border-slate-700",
      indicatorClass: "bg-slate-500",
    },
  },
  EXITED: {
    ERROR: {
      label: "Died on error",
      badgeClass: "bg-red-950/40 text-red-400 border border-red-800/80",
      indicatorClass: "bg-red-500",
    },
    WAITING_QUESTION: {
      label: "Abandoned prompt",
      badgeClass: "bg-amber-950/40 text-amber-400 border border-amber-800/80",
      indicatorClass: "bg-amber-500",
    },
    IDLE: {
      label: "Finished",
      badgeClass: "bg-emerald-950/50 text-emerald-300 border border-emerald-800/60",
      indicatorClass: "bg-emerald-500",
    },
    BUSY: {
      label: "Finished",
      badgeClass: "bg-emerald-950/50 text-emerald-300 border border-emerald-800/60",
      indicatorClass: "bg-emerald-500",
    },
  },
};

export function StatusBadge({
  lifecycle,
  activity,
  stallTier,
  toolDuration,
  isSessionStalled,
  silenceDuration,
  hasUnseenCompletion,
}: IStatusBadgeProps): React.JSX.Element {
  if (lifecycle === "RUNNING" && activity === "IDLE" && Boolean(hasUnseenCompletion)) {
    return (
      <span
        data-testid="StatusBadge"
        role="status"
        data-lifecycle={lifecycle}
        data-semantic-status="success"
        className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/60 text-emerald-300 border border-emerald-600/80"
      >
        <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-emerald-400" />
        <span data-testid="StatusBadge--label">Completed</span>
      </span>
    );
  }

  const isToolStalledOrWarning =
    lifecycle === "RUNNING" && activity === "BUSY" && (stallTier === "warning" || stallTier === "stalled");

  if (isToolStalledOrWarning) {
    const durationLabel = toolDuration ? ` • ${toolDuration}` : "";
    return (
      <span
        data-testid="StatusBadge"
        role="status"
        data-lifecycle={lifecycle}
        data-stall-tier={stallTier}
        data-semantic-status="warning"
        className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-950/60 text-amber-300 border border-amber-600/80 animate-pulse"
      >
        <IconWrapper size="xs" tone="warning">
          <AlertTriangle size={12} />
        </IconWrapper>
        <span data-testid="StatusBadge--label">Working{durationLabel}</span>
      </span>
    );
  }

  const isSilenceStalled = lifecycle === "RUNNING" && activity === "BUSY" && Boolean(isSessionStalled);
  if (isSilenceStalled) {
    const durationLabel = silenceDuration ? ` (${silenceDuration})` : "";
    return (
      <span
        data-testid="StatusBadge"
        role="status"
        data-lifecycle={lifecycle}
        data-stall-type="silence"
        data-semantic-status="warning"
        className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-950/60 text-amber-300 border border-amber-600/80"
      >
        <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-amber-400" />
        <span data-testid="StatusBadge--label">Stalled{durationLabel}</span>
      </span>
    );
  }

  const config = STATUS_CONFIGS[lifecycle]?.[activity] ?? STATUS_CONFIGS.RUNNING.IDLE;
  const semanticStatus =
    activity === "ERROR"
      ? "error"
      : activity === "WAITING_QUESTION"
        ? "warning"
        : lifecycle === "EXITED"
          ? "success"
          : activity === "BUSY"
            ? "info"
            : "neutral";

  return (
    <span
      data-testid="StatusBadge"
      role="status"
      data-lifecycle={lifecycle}
      data-semantic-status={semanticStatus}
      className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium ${config.badgeClass}`}
    >
      <span data-testid="StatusBadge--indicator" className={`w-2 h-2 rounded-full ${config.indicatorClass}`} />
      <span data-testid="StatusBadge--label">{config.label}</span>
    </span>
  );
}
