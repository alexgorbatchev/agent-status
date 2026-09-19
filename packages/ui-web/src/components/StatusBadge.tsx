import React from "react";
import type { LifecycleState, ActivityState } from "../types";

interface StatusBadgeProps {
  lifecycle: LifecycleState;
  activity: ActivityState;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({ lifecycle, activity }) => {
  if (lifecycle === "RUNNING") {
    switch (activity) {
      case "BUSY":
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-900/60 text-blue-200 border border-blue-700/50">
            <span className="w-2 h-2 rounded-full bg-blue-400 animate-pulse" />
            Busy
          </span>
        );
      case "WAITING_QUESTION":
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-900/60 text-amber-200 border border-amber-600 animate-pulse">
            <span className="w-2 h-2 rounded-full bg-amber-400" />
            Waiting on Question
          </span>
        );
      case "ERROR":
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-900/60 text-red-200 border border-red-600 animate-pulse">
            <span className="w-2 h-2 rounded-full bg-red-400" />
            Error
          </span>
        );
      case "IDLE":
      default:
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-800 text-slate-300 border border-slate-700">
            <span className="w-2 h-2 rounded-full bg-slate-500" />
            Idle
          </span>
        );
    }
  }

  // EXITED
  switch (activity) {
    case "ERROR":
      return (
        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-950/40 text-red-400 border border-red-800/80">
          <span className="w-2 h-2 rounded-full bg-red-500" />
          Died on error
        </span>
      );
    case "WAITING_QUESTION":
      return (
        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-950/40 text-amber-400 border border-amber-800/80">
          <span className="w-2 h-2 rounded-full bg-amber-500" />
          Abandoned prompt
        </span>
      );
    case "IDLE":
    default:
      return (
        <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/50 text-emerald-300 border border-emerald-800/60">
          <span className="w-2 h-2 rounded-full bg-emerald-500" />
          Finished
        </span>
      );
  }
};
