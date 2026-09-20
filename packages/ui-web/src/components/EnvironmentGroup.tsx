import React from "react";
import type { IEnvironmentGroupProps } from "./types";
import { AgentGroupSection } from "./AgentGroupSection";
import { Server } from "lucide-react";

export function EnvironmentGroup({ environment, agents, onSelectAgent }: IEnvironmentGroupProps): React.JSX.Element {
  const isStale = environment.isStale;
  const staleBadge = (
    <span
      className={`px-2 py-0.5 rounded-full text-[10px] font-semibold ${
        isStale
          ? "bg-red-950 text-red-300 border border-red-800/60"
          : "bg-emerald-950 text-emerald-300 border border-emerald-800/60"
      }`}
    >
      {isStale ? "Stale" : "Live"}
    </span>
  );

  return (
    <AgentGroupSection
      testId="EnvironmentGroup"
      icon={<Server size={16} />}
      iconColorClass="text-purple-400"
      title={environment.label}
      subtitle={`(${environment.user}@${environment.hostname})`}
      badges={staleBadge}
      agents={agents}
      onSelectAgent={onSelectAgent}
    />
  );
}
