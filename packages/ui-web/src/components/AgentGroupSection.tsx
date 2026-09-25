import React from "react";
import type { IAgentGroupSectionProps } from "./types";
import { calculateTotalSpend, filterActiveAgents } from "../utils/agentUtils";
import { AgentGroupHeader } from "./AgentGroupHeader";
import { AgentCategoryGrids } from "./AgentCategoryGrids";

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
  const prefix = testId || "AgentGroupSection";

  return (
    <div data-testid="AgentGroupSection" className="mb-8">
      <AgentGroupHeader
        testId={prefix}
        icon={icon}
        iconColorClass={iconColorClass}
        title={title}
        subtitle={subtitle}
        badges={badges}
        activeCount={activeCount}
        spend={spend}
      />
      <AgentCategoryGrids
        agents={agents}
        gridTestId={`${prefix}--grid`}
        onSelectAgent={onSelectAgent}
        onToggleHide={onToggleHide}
      />
    </div>
  );
}
