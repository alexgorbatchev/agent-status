import React from "react";
import type { IAgentGroupHeaderProps } from "./types";
import { DollarSign } from "lucide-react";
import { formatCurrency } from "../utils/agentUtils";

export function AgentGroupHeader({
  testId,
  icon,
  iconColorClass,
  title,
  subtitle,
  badges,
  activeCount,
  spend,
}: IAgentGroupHeaderProps): React.JSX.Element {
  const prefix = testId || "AgentGroupHeader";
  const iconContainerClass = iconColorClass
    ? `w-4 h-4 ${iconColorClass} flex items-center justify-center`
    : "w-4 h-4 flex items-center justify-center";

  return (
    <div
      data-testid={testId ? `${testId}--header` : "AgentGroupHeader"}
      className="flex items-center justify-between gap-4 mb-3 pb-2 border-b border-slate-800/80"
    >
      <div className="flex items-center gap-2">
        <span className={iconContainerClass}>{icon}</span>
        <h2 data-testid={`${prefix}--title`} className="text-sm font-bold text-slate-100">
          {title}
        </h2>
        {subtitle && (
          <span data-testid={`${prefix}--subtitle`} className="text-xs text-slate-500 font-mono">
            {subtitle}
          </span>
        )}
        {badges}
        <span
          data-testid={`${prefix}--active-badge`}
          className={`px-2 py-0.5 rounded-full text-[10px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60 ${
            activeCount > 0 ? "visible" : "invisible select-none"
          }`}
          aria-hidden={activeCount === 0 ? "true" : undefined}
        >
          {`${activeCount} active`}
        </span>
      </div>

      <div data-testid={`${prefix}--spend`} className="flex items-center gap-1 text-xs font-mono text-emerald-400">
        <span className="w-3.5 h-3.5 flex items-center justify-center">
          <DollarSign size={14} />
        </span>
        <span>{formatCurrency(spend, 3)}</span>
      </div>
    </div>
  );
}
