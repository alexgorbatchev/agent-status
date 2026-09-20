import React from "react";
import type { IStatusBadgeProps } from "./types";

export function StatusBadge({ lifecycle, activity }: IStatusBadgeProps): React.JSX.Element {
  if (lifecycle === "RUNNING") {
    switch (activity) {
      case "BUSY":
        return (
          <span
            data-testid="StatusBadge"
            className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-900/60 text-blue-200 border border-blue-700/50"
          >
            <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-blue-400 animate-pulse" />
            <span data-testid="StatusBadge--label">Busy</span>
          </span>
        );
      case "WAITING_QUESTION":
        return (
          <span
            data-testid="StatusBadge"
            className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-900/60 text-amber-200 border border-amber-600 animate-pulse"
          >
            <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-amber-400" />
            <span data-testid="StatusBadge--label">Waiting on Question</span>
          </span>
        );
      case "ERROR":
        return (
          <span
            data-testid="StatusBadge"
            className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-900/60 text-red-200 border border-red-600 animate-pulse"
          >
            <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-red-400" />
            <span data-testid="StatusBadge--label">Error</span>
          </span>
        );
      case "IDLE":
      default:
        return (
          <span
            data-testid="StatusBadge"
            className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-800 text-slate-300 border border-slate-700"
          >
            <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-slate-500" />
            <span data-testid="StatusBadge--label">Idle</span>
          </span>
        );
    }
  }

  // EXITED
  switch (activity) {
    case "ERROR":
      return (
        <span
          data-testid="StatusBadge"
          className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-950/40 text-red-400 border border-red-800/80"
        >
          <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-red-500" />
          <span data-testid="StatusBadge--label">Died on error</span>
        </span>
      );
    case "WAITING_QUESTION":
      return (
        <span
          data-testid="StatusBadge"
          className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-950/40 text-amber-400 border border-amber-800/80"
        >
          <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-amber-500" />
          <span data-testid="StatusBadge--label">Abandoned prompt</span>
        </span>
      );
    case "IDLE":
    default:
      return (
        <span
          data-testid="StatusBadge"
          className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/50 text-emerald-300 border border-emerald-800/60"
        >
          <span data-testid="StatusBadge--indicator" className="w-2 h-2 rounded-full bg-emerald-500" />
          <span data-testid="StatusBadge--label">Finished</span>
        </span>
      );
  }
}
