import React, { useMemo } from "react";
import type { AgentSelectCallback, AgentToggleHideCallback, IProjectGroupProps } from "./types";
import { AgentCard } from "./AgentCard";
import { calculateTotalSpend, filterActiveAgents, formatCurrency } from "../utils/agentUtils";
import { groupAgentsByCategory, CATEGORY_ORDER, groupAgentsByProjectHierarchy } from "../utils/groupingUtils";
import { FolderGit2, DollarSign } from "lucide-react";
import type { IAgentState } from "../types";

function renderCategoryGrids(
  agents: IAgentState[],
  projectName: string,
  hideWorktreeBadge: boolean,
  onSelectAgent: AgentSelectCallback,
  onToggleHide?: AgentToggleHideCallback,
  gridTestId = "ProjectGroup--grid",
): React.JSX.Element {
  const grouped = groupAgentsByCategory(agents);
  const visibleCategories = CATEGORY_ORDER.filter((cat) => grouped[cat].length > 0);

  return (
    <div data-testid={gridTestId} className="flex flex-col gap-8">
      {visibleCategories.map((category) => (
        <div
          key={category}
          data-testid={`ProjectGroup--grid-${category}`}
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

export function ProjectGroup({
  projectName,
  projectId,
  agents,
  treeGroup,
  onSelectAgent,
  onToggleHide,
}: IProjectGroupProps): React.JSX.Element {
  const tree = useMemo(() => {
    if (treeGroup) {
      return treeGroup;
    }
    const hierarchy = groupAgentsByProjectHierarchy(agents);
    return (
      hierarchy.find((item) => item.id === projectId) ??
      hierarchy[0] ?? {
        id: projectId,
        name: projectName,
        rootAgents: agents.filter((a) => !a.workspace.isWorktree),
        worktrees: [],
        allAgents: agents,
      }
    );
  }, [treeGroup, agents, projectId, projectName]);

  const spend = calculateTotalSpend(tree.allAgents);
  const activeCount = filterActiveAgents(tree.allAgents).length;
  const hasWorktrees = tree.worktrees.length > 0;
  const rootBranch = tree.rootAgents[0]?.workspace.branch || "main";

  return (
    <div data-testid="ProjectGroup" className="mb-8">
      {/* Project Family Header */}
      <div
        data-testid="ProjectGroup--header"
        className="flex items-center justify-between gap-4 mb-3 pb-2 border-b border-slate-800/80"
      >
        <div className="flex items-center gap-2">
          <span className="w-4 h-4 text-blue-400 flex items-center justify-center">
            <FolderGit2 size={16} />
          </span>
          <h2 data-testid="ProjectGroup--title" className="text-sm font-bold text-slate-100">
            {projectName}
          </h2>
          <span data-testid="ProjectGroup--subtitle" className="text-xs text-slate-500 font-mono">
            {`(${projectId})`}
          </span>
          <span
            data-testid="ProjectGroup--active-badge"
            className={`px-2 py-0.5 rounded-full text-[10px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60 ${
              activeCount > 0 ? "visible" : "invisible select-none"
            }`}
            aria-hidden={activeCount === 0 ? "true" : undefined}
          >
            {`${activeCount} active`}
          </span>
        </div>

        <div data-testid="ProjectGroup--spend" className="flex items-center gap-1 text-xs font-mono text-emerald-400">
          <span className="w-3.5 h-3.5 flex items-center justify-center">
            <DollarSign size={14} />
          </span>
          <span>{formatCurrency(spend, 3)}</span>
        </div>
      </div>

      {/* Hierarchical Structure: Main Checkout & Nested Worktrees */}
      {!hasWorktrees ? (
        renderCategoryGrids(
          tree.rootAgents.length > 0 ? tree.rootAgents : tree.allAgents,
          projectName,
          false,
          onSelectAgent,
          onToggleHide,
          "ProjectGroup--grid",
        )
      ) : (
        <div className="flex flex-col gap-6">
          {/* Main Checkout Root Branch */}
          {tree.rootAgents.length > 0 && (
            <div data-testid="ProjectGroup--root-section" className="flex flex-col">
              <div
                data-testid="ProjectGroup--root-branch"
                className="flex items-center gap-2 mb-3 text-xs text-slate-400"
              >
                <span className="font-mono text-slate-500 select-none">├─</span>
                <span className="px-1.5 py-0.5 rounded text-[11px] font-mono font-medium bg-slate-800/80 text-slate-200 border border-slate-700/60">
                  {`[${rootBranch}]`}
                </span>
                <span className="text-xs text-slate-500 font-mono">
                  {`(${tree.rootAgents.length} ${tree.rootAgents.length === 1 ? "session" : "sessions"})`}
                </span>
              </div>
              <div className="ml-2 pl-4 border-l border-slate-800">
                {renderCategoryGrids(
                  tree.rootAgents,
                  projectName,
                  false,
                  onSelectAgent,
                  onToggleHide,
                  "ProjectGroup--root-grid",
                )}
              </div>
            </div>
          )}

          {/* Linked Worktree Branches */}
          {tree.worktrees.map((wt, index) => {
            const isLast = index === tree.worktrees.length - 1;
            const branchLabel = wt.branch || "HEAD";
            return (
              <div key={wt.worktreeKey} data-testid="ProjectGroup--worktree-section" className="flex flex-col">
                <div
                  data-testid="ProjectGroup--worktree-header"
                  className="flex items-center gap-2 mb-3 text-xs text-slate-400"
                >
                  <span className="font-mono text-slate-500 select-none">{isLast ? "└─" : "├─"}</span>
                  <span className="px-1.5 py-0.5 rounded text-[11px] font-mono font-medium bg-indigo-950/60 text-indigo-300 border border-indigo-800/60">
                    {`[worktree: ${branchLabel}]`}
                  </span>
                  {wt.relPath && <span className="text-xs text-slate-400 font-mono">{`(${wt.relPath})`}</span>}
                  <span className="text-xs text-slate-500 font-mono">
                    {`(${wt.agents.length} ${wt.agents.length === 1 ? "session" : "sessions"})`}
                  </span>
                </div>
                <div className={`ml-2 pl-4 ${isLast ? "border-l border-transparent" : "border-l border-slate-800"}`}>
                  {renderCategoryGrids(
                    wt.agents,
                    projectName,
                    true,
                    onSelectAgent,
                    onToggleHide,
                    "ProjectGroup--worktree-grid",
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
