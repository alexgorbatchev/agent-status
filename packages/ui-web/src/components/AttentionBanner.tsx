import React from "react";
import type { IAttentionBannerProps } from "./types";
import { isAgentRequiringAttention } from "../utils/agentUtils";
import { AlertCircle, HelpCircle, AlertTriangle, ArrowRight } from "lucide-react";
import { isAgentRequiringAttention } from "../utils/agentUtils";

export function AttentionBanner({ agents, onSelectAgent }: IAttentionBannerProps) {
  // Tier 1: ACT NOW (Running agents with WAITING_QUESTION or ERROR)
  const actNowAgents = agents.filter(isAgentRequiringAttention);

  if (actNowAgents.length === 0) return null;

  return (
    <div
      data-testid="AttentionBanner"
      className="mb-6 p-4 rounded-xl bg-amber-950/30 border border-amber-800/80 shadow-lg shadow-amber-950/20"
    >
      <div data-testid="AttentionBanner--header" className="flex items-center gap-2 mb-3">
        <span className="w-5 h-5 text-amber-400 flex items-center justify-center">
          <AlertCircle size={20} />
        </span>
        <h2 data-testid="AttentionBanner--title" className="text-sm font-bold tracking-wide uppercase text-amber-300">
          Act Now — {actNowAgents.length} Agent{actNowAgents.length > 1 ? "s" : ""} Require Attention
        </h2>
      </div>

      <div data-testid="AttentionBanner--grid" className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {actNowAgents.map((agent) => {
          const isQuestion = agent.activity.activity === "WAITING_QUESTION";
          return (
            <div
              key={agent.id}
              data-testid="AttentionBanner--agent-card"
              onClick={() => onSelectAgent(agent)}
              className="flex items-start justify-between gap-3 p-3 rounded-lg bg-slate-900/90 border border-amber-900/60 hover:border-amber-600 transition-all cursor-pointer group"
            >
              <div className="flex items-start gap-2.5">
                {isQuestion ? (
                  <span className="w-4 h-4 text-amber-400 shrink-0 mt-0.5 flex items-center justify-center">
                    <HelpCircle size={16} />
                  </span>
                ) : (
                  <span className="w-4 h-4 text-red-400 shrink-0 mt-0.5 flex items-center justify-center">
                    <AlertTriangle size={16} />
                  </span>
                )}
                <div>
                  <div className="flex items-center gap-2 text-xs font-semibold text-slate-200 group-hover:text-amber-300 transition-colors">
                    <span>{agent.activity.title || "Untitled Agent"}</span>
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-400 font-mono">
                      {agent.environment.label}
                    </span>
                  </div>
                  <p className="text-xs text-slate-400 line-clamp-1 mt-0.5 font-mono">
                    {isQuestion
                      ? agent.activity.question || "Waiting on user question input"
                      : agent.activity.lastError || "Agent encountered an execution error"}
                  </p>
                </div>
              </div>

              <span className="w-4 h-4 text-slate-500 group-hover:text-amber-400 shrink-0 transition-colors self-center flex items-center justify-center">
                <ArrowRight size={16} />
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
