import React from "react";
import type { IHeaderProps } from "./types";
import { IconWrapper } from "./IconWrapper";
import {
  calculateTotalSpend,
  filterActiveAgents,
  formatCurrency,
  isAgentRequiringAttention,
} from "../utils/agentUtils";
import { Bot, AlertCircle, DollarSign, Activity, Server, HelpCircle } from "lucide-react";
import { clsx } from "clsx";

export function Header({
  agents,
  environments,
  isConnected,
  isActiveFilter,
  onToggleActiveFilter,
  onOpenHelp,
}: IHeaderProps): React.JSX.Element {
  const activeAgents = filterActiveAgents(agents);
  const attentionAgents = agents.filter((a) => isAgentRequiringAttention(a));

  const totalSpend = calculateTotalSpend(agents);
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
        <button
          type="button"
          data-testid="Header--active-agents"
          aria-pressed={Boolean(isActiveFilter)}
          aria-label={isActiveFilter ? "Disable active agents filter" : "Filter by active agents"}
          onClick={onToggleActiveFilter}
          className={clsx(
            "flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-xs font-medium transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500",
            isActiveFilter
              ? "bg-blue-950/70 border-blue-500/70 text-blue-300 ring-1 ring-blue-500/40"
              : "bg-slate-900 border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700",
          )}
        >
          <IconWrapper size="md" tone="primary">
            <Activity size={16} />
          </IconWrapper>
          <span className={isActiveFilter ? "text-blue-300" : "text-slate-400"}>Active:</span>
          <span className={`font-semibold ${isActiveFilter ? "text-blue-200" : "text-slate-200"}`}>
            {activeAgents.length}
          </span>
        </button>

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
          <span className="font-semibold">{formatCurrency(totalSpend)}</span>
        </div>

        {/* State Legend Help Button */}
        {onOpenHelp && (
          <button
            type="button"
            aria-label="Open state legend help dialog"
            data-testid="Header--help-button"
            onClick={onOpenHelp}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700 transition-colors cursor-pointer"
          >
            <IconWrapper size="md" tone="muted">
              <HelpCircle size={16} />
            </IconWrapper>
            <span>Legend</span>
          </button>
        )}
      </div>
    </header>
  );
}
