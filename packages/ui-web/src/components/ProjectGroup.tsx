import React from "react";
import type { IAgentState } from "../types";
import { AgentCard } from "./AgentCard";
import { FolderGit2, DollarSign } from "lucide-react";

interface ProjectGroupProps {
  projectName: string;
  projectId: string;
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
}

export const ProjectGroup: React.FC<ProjectGroupProps> = ({
  projectName,
  projectId,
  agents,
  onSelectAgent,
}) => {
  const projectSpend = agents.reduce((acc, a) => acc + (a.activity.cost?.total || 0), 0);
  const activeCount = agents.filter((a) => a.activity.lifecycle === "RUNNING").length;

  return (
    <div className="mb-8">
      {/* Project Header */}
      <div className="flex items-center justify-between gap-4 mb-3 pb-2 border-b border-slate-800/80">
        <div className="flex items-center gap-2">
          <FolderGit2 className="w-4 h-4 text-blue-400" />
          <h2 className="text-sm font-bold text-slate-100">{projectName}</h2>
          <span className="text-xs text-slate-500 font-mono">({projectId})</span>
          {activeCount > 0 && (
            <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60">
              {activeCount} active
            </span>
          )}
        </div>

        <div className="flex items-center gap-1 text-xs font-mono text-emerald-400">
          <DollarSign className="w-3.5 h-3.5" />
          <span>${projectSpend.toFixed(3)}</span>
        </div>
      </div>

      {/* Agents Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {agents.map((agent) => (
          <AgentCard key={agent.id} agent={agent} onOpenDrawer={onSelectAgent} />
        ))}
      </div>
    </div>
  );
};
