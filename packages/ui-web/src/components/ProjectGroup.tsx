import React, { useMemo } from "react";
import type { IProjectGroupProps } from "./types";
import { calculateTotalSpend, filterActiveAgents } from "../utils/agentUtils";
import { groupAgentsByProjectHierarchy } from "../utils/groupingUtils";
import { FolderGit2 } from "lucide-react";
import { AgentGroupHeader } from "./AgentGroupHeader";
import { AgentCategoryGrids } from "./AgentCategoryGrids";

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
      hierarchy.find((item) => item.id === projectId) ?? {
        id: projectId,
        name: projectName,
        rootAgents: [],
        worktrees: [],
        allAgents: [],
      }
    );
  }, [treeGroup, agents, projectId, projectName]);

  const spend = calculateTotalSpend(tree.allAgents);
  const activeCount = filterActiveAgents(tree.allAgents).length;
  const hasWorktrees = tree.worktrees.length > 0;
  const rootBranch = tree.rootAgents[0]?.workspace.branch || "HEAD";
  const prefix = "ProjectGroup";

  return (
    <div data-testid="ProjectGroup" className="mb-8">
      {/* Project Family Header */}
      <AgentGroupHeader
        testId={prefix}
        icon={<FolderGit2 size={16} />}
        iconColorClass="text-blue-400"
        title={projectName}
        subtitle={`(${projectId})`}
        activeCount={activeCount}
        spend={spend}
      />

      {/* Hierarchical Structure: Main Checkout & Nested Worktrees */}
      {!hasWorktrees ? (
        <AgentCategoryGrids
          agents={tree.rootAgents.length > 0 ? tree.rootAgents : tree.allAgents}
          gridTestId="ProjectGroup--grid"
          projectName={projectName}
          hideWorktreeBadge={false}
          onSelectAgent={onSelectAgent}
          onToggleHide={onToggleHide}
        />
      ) : (
        <div className="flex flex-col gap-6">
          {/* Main Checkout Root Branch */}
          {tree.rootAgents.length > 0 && (
            <div data-testid="ProjectGroup--root-section" className="flex flex-col">
              <h3
                data-testid="ProjectGroup--root-branch"
                className="flex items-center gap-2 mb-3 text-xs font-normal text-slate-400"
              >
                <span className="font-mono text-slate-500 select-none" aria-hidden="true">
                  ├─
                </span>
                <span className="px-1.5 py-0.5 rounded text-[11px] font-mono font-medium bg-slate-800/80 text-slate-200 border border-slate-700/60">
                  {`[${rootBranch}]`}
                </span>
                <span className="text-xs text-slate-500 font-mono">
                  {`(${tree.rootAgents.length} ${tree.rootAgents.length === 1 ? "session" : "sessions"})`}
                </span>
              </h3>
              <div className="ml-2 pl-4 border-l border-slate-800">
                <AgentCategoryGrids
                  agents={tree.rootAgents}
                  gridTestId="ProjectGroup--root-grid"
                  projectName={projectName}
                  hideWorktreeBadge={false}
                  onSelectAgent={onSelectAgent}
                  onToggleHide={onToggleHide}
                />
              </div>
            </div>
          )}

          {/* Linked Worktree Branches */}
          {tree.worktrees.map((wt, index) => {
            const isLast = index === tree.worktrees.length - 1;
            const branchLabel = wt.branch || "HEAD";
            return (
              <div key={wt.worktreeKey} data-testid="ProjectGroup--worktree-section" className="flex flex-col">
                <h3
                  data-testid="ProjectGroup--worktree-header"
                  className="flex items-center gap-2 mb-3 text-xs font-normal text-slate-400"
                >
                  <span className="font-mono text-slate-500 select-none" aria-hidden="true">
                    {isLast ? "└─" : "├─"}
                  </span>
                  <span className="px-1.5 py-0.5 rounded text-[11px] font-mono font-medium bg-indigo-950/60 text-indigo-300 border border-indigo-800/60">
                    {`[worktree: ${branchLabel}]`}
                  </span>
                  {wt.relPath && <span className="text-xs text-slate-400 font-mono">{`(${wt.relPath})`}</span>}
                  <span className="text-xs text-slate-500 font-mono">
                    {`(${wt.agents.length} ${wt.agents.length === 1 ? "session" : "sessions"})`}
                  </span>
                </h3>
                <div className={`ml-2 pl-4 ${isLast ? "border-l border-transparent" : "border-l border-slate-800"}`}>
                  <AgentCategoryGrids
                    agents={wt.agents}
                    gridTestId="ProjectGroup--worktree-grid"
                    projectName={projectName}
                    hideWorktreeBadge={true}
                    onSelectAgent={onSelectAgent}
                    onToggleHide={onToggleHide}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
