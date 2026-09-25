import React, { useMemo } from "react";
import type { IAgentCategoryGridsProps } from "./types";
import { AgentCard } from "./AgentCard";
import { groupAgentsByCategory, CATEGORY_ORDER } from "../utils/groupingUtils";

export function AgentCategoryGrids({
  agents,
  gridTestId,
  projectName,
  hideWorktreeBadge = false,
  onSelectAgent,
  onToggleHide,
}: IAgentCategoryGridsProps): React.JSX.Element {
  const prefix = gridTestId || "AgentCategoryGrids";
  const grouped = useMemo(() => groupAgentsByCategory(agents), [agents]);
  const visibleCategories = CATEGORY_ORDER.filter((cat) => grouped[cat].length > 0);

  return (
    <div data-testid={gridTestId ? gridTestId : "AgentCategoryGrids"} className="flex flex-col gap-8">
      {visibleCategories.map((category) => (
        <div
          key={category}
          data-testid={`${prefix}-${category}`}
          className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
        >
          {grouped[category].map((agent) => (
            <AgentCard
              key={agent.id}
              agent={agent}
              projectName={projectName}
              hideWorktreeBadge={hideWorktreeBadge}
              onOpenDrawer={onSelectAgent}
              onToggleHide={onToggleHide}
            />
          ))}
        </div>
      ))}
    </div>
  );
}
