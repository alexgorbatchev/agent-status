import React from "react";
import type { IAttentionBannerProps } from "./types";
import type { IAgentState } from "../types";
import { IconWrapper } from "./IconWrapper";
import { AlertCircle, HelpCircle, AlertTriangle, ArrowRight } from "lucide-react";
import { isAgentRequiringAttention } from "../utils/agentUtils";

export function AttentionBanner({ agents, onSelectAgent }: IAttentionBannerProps): React.JSX.Element | null {
  // Tier 1: ACT NOW (Running agents with WAITING_QUESTION or ERROR)
  const actNowAgents = agents.filter(isAgentRequiringAttention);

  if (actNowAgents.length === 0) return null;

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>, agent: IAgentState) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onSelectAgent(agent);
    }
  };

  const titleText = `Act Now — ${actNowAgents.length} ${
    actNowAgents.length === 1 ? "Agent Requires" : "Agents Require"
  } Attention`;

  return (
    <div
      data-testid="AttentionBanner"
      role="region"
      aria-label={titleText}
      className="mb-6 p-4 rounded-xl bg-amber-950/30 border border-amber-800/80 shadow-lg shadow-amber-950/20"
    >
      <div data-testid="AttentionBanner--header" className="flex items-center gap-2 mb-3">
        <IconWrapper size="lg" tone="warning">
          <AlertCircle size={20} />
        </IconWrapper>
        <h2 data-testid="AttentionBanner--title" className="text-sm font-bold tracking-wide uppercase text-amber-300">
          {titleText}
        </h2>
      </div>

      <div data-testid="AttentionBanner--grid" className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {actNowAgents.map((agent) => {
          const isQuestion = agent.activity.activity === "WAITING_QUESTION";
          const title = agent.activity.title || "Untitled Agent";
          return (
            <div
              key={agent.id}
              role="button"
              tabIndex={0}
              aria-label={`View agent needing attention: ${title}`}
              data-testid="AttentionBanner--agent-card"
              onClick={() => onSelectAgent(agent)}
              onKeyDown={(event) => handleKeyDown(event, agent)}
              className="flex items-start justify-between gap-3 p-3 rounded-lg bg-slate-900/90 border border-amber-900/60 hover:border-amber-600 transition-all cursor-pointer group"
            >
              <div className="flex items-start gap-2.5">
                {isQuestion ? (
                  <IconWrapper size="md" tone="warning" align="top">
                    <HelpCircle size={16} />
                  </IconWrapper>
                ) : (
                  <IconWrapper size="md" tone="error" align="top">
                    <AlertTriangle size={16} />
                  </IconWrapper>
                )}
                <div>
                  <div className="flex items-center gap-2 text-xs font-semibold text-slate-200 group-hover:text-amber-300 transition-colors">
                    <span data-testid="AttentionBanner--agent-title">{title}</span>
                    <span
                      data-testid="AttentionBanner--agent-env"
                      role="status"
                      className="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-400 font-mono"
                    >
                      {agent.environment.label}
                    </span>
                  </div>
                  <p
                    data-testid="AttentionBanner--agent-message"
                    role="alert"
                    data-callout-type={isQuestion ? "warning" : "error"}
                    className="text-xs text-slate-400 line-clamp-1 mt-0.5 font-mono"
                  >
                    {isQuestion
                      ? agent.activity.question || "Waiting on user question input"
                      : agent.activity.lastError || "Agent encountered an execution error"}
                  </p>
                </div>
              </div>

              <span className="text-slate-500 group-hover:text-amber-400 transition-colors self-center">
                <IconWrapper size="md">
                  <ArrowRight size={16} />
                </IconWrapper>
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
