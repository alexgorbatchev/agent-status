import React from "react";
import type { IAgentState, IEnvironmentInfo } from "../types";
import { Bot, AlertCircle, DollarSign, Activity, Server } from "lucide-react";

interface HeaderProps {
  agents: IAgentState[];
  environments: IEnvironmentInfo[];
  isConnected: boolean;
}

export const Header: React.FC<HeaderProps> = ({ agents, environments, isConnected }) => {
  const activeAgents = agents.filter((a) => a.activity.lifecycle === "RUNNING");
  const attentionAgents = agents.filter(
    (a) =>
      a.activity.lifecycle === "RUNNING" &&
      (a.activity.activity === "WAITING_QUESTION" || a.activity.activity === "ERROR")
  );

  const totalSpend = agents.reduce((acc, a) => acc + (a.activity.cost?.total || 0), 0);
  const liveEnvs = environments.filter((e) => !e.isStale).length;

  return (
    <header className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 p-4 border-b border-slate-800 bg-slate-950/60 backdrop-blur">
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-blue-600/20 border border-blue-500/30 text-blue-400">
          <Bot className="w-5 h-5" />
        </div>
        <div>
          <h1 className="text-lg font-bold text-slate-100 flex items-center gap-2">
            Agent Status
            <span
              className={`w-2 h-2 rounded-full ${
                isConnected ? "bg-emerald-400" : "bg-red-400 animate-pulse"
              }`}
            />
          </h1>
          <p className="text-xs text-slate-400">
            Multi-host coding agent live fleet observability
          </p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-4 text-xs">
        {/* Active Agents */}
        <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800">
          <Activity className="w-4 h-4 text-blue-400" />
          <span className="text-slate-400">Active:</span>
          <span className="font-semibold text-slate-200">{activeAgents.length}</span>
        </div>

        {/* Attention Needed */}
        {attentionAgents.length > 0 && (
          <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-950/40 border border-amber-800/60 text-amber-300 animate-pulse">
            <AlertCircle className="w-4 h-4 text-amber-400" />
            <span>Needs Attention:</span>
            <span className="font-bold">{attentionAgents.length}</span>
          </div>
        )}

        {/* Environments */}
        <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800">
          <Server className="w-4 h-4 text-slate-400" />
          <span className="text-slate-400">Environments:</span>
          <span className="font-semibold text-slate-200">
            {liveEnvs}/{environments.length}
          </span>
        </div>

        {/* Total Fleet Spend */}
        <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 font-mono text-emerald-400">
          <DollarSign className="w-4 h-4 text-emerald-400" />
          <span className="font-semibold">${totalSpend.toFixed(2)}</span>
        </div>
      </div>
    </header>
  );
};
