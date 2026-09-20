import React from "react";
import type { IProjectGroupProps } from "./types";
import { AgentCard } from "./AgentCard";
import { IconWrapper } from "./IconWrapper";
import { FolderGit2, DollarSign } from "lucide-react";

export function ProjectGroup({ projectName, projectId, agents, onSelectAgent }: IProjectGroupProps): React.JSX.Element {
  const projectSpend = agents.reduce((acc, a) => acc + (a.activity.cost?.total || 0), 0);
  const activeCount = agents.filter((a) => a.activity.lifecycle === "RUNNING").length;

  return (
    <div data-testid="ProjectGroup" className="mb-8">
      {/* Project Header */}
      <div
        data-testid="ProjectGroup--header"
        className="flex items-center justify-between gap-4 mb-3 pb-2 border-b border-slate-800/80"
      >
        <div className="flex items-center gap-2">
          <IconWrapper size="md" tone="primary">
            <FolderGit2 size={16} />
          </IconWrapper>
          <h2 data-testid="ProjectGroup--title" className="text-sm font-bold text-slate-100">
            {projectName}
          </h2>
          <span className="text-xs text-slate-500 font-mono">({projectId})</span>
          {activeCount > 0 && (
            <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60">
              {activeCount} active
            </span>
          )}
        </div>

        <div data-testid="ProjectGroup--spend" className="flex items-center gap-1 text-xs font-mono text-emerald-400">
          <IconWrapper size="sm">
            <DollarSign size={14} />
          </IconWrapper>
          <span>${projectSpend.toFixed(3)}</span>
        </div>
      </div>

      {/* Agents Grid */}
      <div data-testid="ProjectGroup--grid" className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {agents.map((agent) => (
          <AgentCard key={agent.id} agent={agent} onOpenDrawer={onSelectAgent} />
        ))}
      </div>
    </div>
  );
}
