import React from "react";
import type { IHeaderProps } from "./types";
import { IconWrapper } from "./IconWrapper";
import { Bot, AlertCircle, DollarSign, Activity, Server } from "lucide-react";

export function Header({ agents, environments, isConnected }: IHeaderProps): React.JSX.Element {
  const activeAgents = agents.filter((a) => a.activity.lifecycle === "RUNNING");
  const attentionAgents = agents.filter(
    (a) =>
      a.activity.lifecycle === "RUNNING" &&
      (a.activity.activity === "WAITING_QUESTION" || a.activity.activity === "ERROR"),
  );

  const totalSpend = agents.reduce((acc, a) => acc + (a.activity.cost?.total || 0), 0);
  const liveEnvs = environments.filter((e) => !e.isStale).length;

  return (
    <header
      data-testid="Header"
      className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 p-4 border-b border-slate-800 bg-slate-950/60 backdrop-blur"
    >
      <div data-testid="Header--brand" className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-blue-600/20 border border-blue-500/30 text-blue-400 flex items-center justify-center">
          <Bot size={20} />
        </div>
        <div>
          <h1 data-testid="Header--title" className="text-lg font-bold text-slate-100 flex items-center gap-2">
            Agent Status
            <span className={`w-2 h-2 rounded-full ${isConnected ? "bg-emerald-400" : "bg-red-400 animate-pulse"}`} />
          </h1>
          <p className="text-xs text-slate-400">Multi-host coding agent live fleet observability</p>
        </div>
      </div>

      <div data-testid="Header--stats" className="flex flex-wrap items-center gap-4 text-xs">
        {/* Active Agents */}
        <div
          data-testid="Header--active-agents"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800"
        >
          <IconWrapper size="md" tone="primary">
            <Activity size={16} />
          </IconWrapper>
          <span className="text-slate-400">Active:</span>
          <span className="font-semibold text-slate-200">{activeAgents.length}</span>
        </div>

        {/* Attention Needed */}
        {attentionAgents.length > 0 && (
          <div
            data-testid="Header--attention-agents"
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-950/40 border border-amber-800/60 text-amber-300 animate-pulse"
          >
            <IconWrapper size="md" tone="warning">
              <AlertCircle size={16} />
            </IconWrapper>
            <span>Needs Attention:</span>
            <span className="font-bold">{attentionAgents.length}</span>
          </div>
        )}

        {/* Environments */}
        <div
          data-testid="Header--environments"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800"
        >
          <IconWrapper size="md" tone="muted">
            <Server size={16} />
          </IconWrapper>
          <span className="text-slate-400">Environments:</span>
          <span className="font-semibold text-slate-200">
            {liveEnvs}/{environments.length}
          </span>
        </div>

        {/* Total Fleet Spend */}
        <div
          data-testid="Header--total-spend"
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 font-mono text-emerald-400"
        >
          <IconWrapper size="md" tone="success">
            <DollarSign size={16} />
          </IconWrapper>
          <span className="font-semibold">${totalSpend.toFixed(2)}</span>
        </div>
      </div>
    </header>
  );
}
