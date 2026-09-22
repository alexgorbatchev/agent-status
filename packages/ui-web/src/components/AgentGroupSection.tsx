import React, { useMemo } from "react";
import type { IAgentGroupSectionProps } from "./types";
import { AgentCard } from "./AgentCard";
import { calculateTotalSpend, filterActiveAgents, formatCurrency } from "../utils/agentUtils";
import { groupAgentsByCategory, CATEGORY_ORDER } from "../utils/groupingUtils";
import { DollarSign } from "lucide-react";

export function AgentGroupSection({
  testId,
  icon,
  iconColorClass,
  title,
  subtitle,
  badges,
  agents,
  onSelectAgent,
  onToggleHide,
}: IAgentGroupSectionProps): React.JSX.Element {
  const spend = calculateTotalSpend(agents);
  const activeCount = filterActiveAgents(agents).length;

  const iconContainerClass = iconColorClass
    ? `w-4 h-4 ${iconColorClass} flex items-center justify-center`
    : "w-4 h-4 flex items-center justify-center";

  const prefix = testId || "AgentGroupSection";

  const grouped = useMemo(() => groupAgentsByCategory(agents), [agents]);
  const visibleCategories = CATEGORY_ORDER.filter((cat) => grouped[cat].length > 0);

  return (
    <div data-testid="AgentGroupSection" className="mb-8">
      {/* Section Header */}
      <div
        data-testid={`${prefix}--header`}
        className="flex items-center justify-between gap-4 mb-3 pb-2 border-b border-slate-800/80"
      >
        <div className="flex items-center gap-2">
          <span className={iconContainerClass}>{icon}</span>
          <h2 data-testid={`${prefix}--title`} className="text-sm font-bold text-slate-100">
            {title}
          </h2>
          <span className="text-xs text-slate-500 font-mono">{subtitle}</span>
          {badges}
          {activeCount > 0 && (
            <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60">
              {`${activeCount} active`}
            </span>
          )}
        </div>

        <div data-testid={`${prefix}--spend`} className="flex items-center gap-1 text-xs font-mono text-emerald-400">
          <span className="w-3.5 h-3.5 flex items-center justify-center">
            <DollarSign size={14} />
          </span>
          <span>{formatCurrency(spend, 3)}</span>
        </div>
      </div>

      {/* Agents Grid Sections */}
      <div data-testid={`${prefix}--grid`} className="flex flex-col gap-8">
        {visibleCategories.map((category) => (
          <div
            key={category}
            data-testid={`${prefix}--grid-${category}`}
            className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
          >
            {grouped[category].map((agent) => (
              <AgentCard key={agent.id} agent={agent} onOpenDrawer={onSelectAgent} onToggleHide={onToggleHide} />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
