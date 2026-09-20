import React from "react";
import type { IEnvironmentGroupProps } from "./types";
import { AgentCard } from "./AgentCard";
import { calculateTotalSpend, filterActiveAgents, formatCurrency } from "../utils/agentUtils";
import { Server, DollarSign } from "lucide-react";

export function EnvironmentGroup({ environment, agents, onSelectAgent }: IEnvironmentGroupProps) {
  const envSpend = calculateTotalSpend(agents);
  const activeCount = filterActiveAgents(agents).length;

  return (
    <div data-testid="EnvironmentGroup" className="mb-8">
      {/* Environment Header */}
      <div
        data-testid="EnvironmentGroup--header"
        className="flex items-center justify-between gap-4 mb-3 pb-2 border-b border-slate-800/80"
      >
        <div className="flex items-center gap-2">
          <span className="w-4 h-4 text-purple-400 flex items-center justify-center">
            <Server size={16} />
          </span>
          <h2 data-testid="EnvironmentGroup--title" className="text-sm font-bold text-slate-100">
            {environment.label}
          </h2>
          <span className="text-xs text-slate-500 font-mono">
            ({environment.user}@{environment.hostname})
          </span>
          <span
            className={`px-2 py-0.5 rounded-full text-[10px] font-semibold ${
              environment.isStale
                ? "bg-red-950 text-red-300 border border-red-800/60"
                : "bg-emerald-950 text-emerald-300 border border-emerald-800/60"
            }`}
          >
            {environment.isStale ? "Stale" : "Live"}
          </span>
          {activeCount > 0 && (
            <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-blue-950 text-blue-300 border border-blue-800/60">
              {activeCount} active
            </span>
          )}
        </div>

        <div
          data-testid="EnvironmentGroup--spend"
          className="flex items-center gap-1 text-xs font-mono text-emerald-400"
        >
          <span className="w-3.5 h-3.5 flex items-center justify-center">
            <DollarSign size={14} />
          </span>
          <span>{formatCurrency(envSpend, 3)}</span>
        </div>
      </div>

      {/* Agents Grid */}
      <div data-testid="EnvironmentGroup--grid" className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {agents.map((agent) => (
          <AgentCard key={agent.id} agent={agent} onOpenDrawer={onSelectAgent} />
        ))}
      </div>
    </div>
  );
}
