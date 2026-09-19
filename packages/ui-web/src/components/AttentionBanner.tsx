import React from "react";
import type { IAgentState } from "../types";
import { AlertCircle, HelpCircle, AlertTriangle, ArrowRight } from "lucide-react";

interface AttentionBannerProps {
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
}

export const AttentionBanner: React.FC<AttentionBannerProps> = ({ agents, onSelectAgent }) => {
  // Tier 1: ACT NOW (Running agents with WAITING_QUESTION or ERROR)
  const actNowAgents = agents.filter(
    (a) =>
      a.activity.lifecycle === "RUNNING" &&
      !a.environment.isStale &&
      (a.activity.activity === "WAITING_QUESTION" || a.activity.activity === "ERROR")
  );

  if (actNowAgents.length === 0) return null;

  return (
    <div className="mb-6 p-4 rounded-xl bg-amber-950/30 border border-amber-800/80 shadow-lg shadow-amber-950/20">
      <div className="flex items-center gap-2 mb-3">
        <AlertCircle className="w-5 h-5 text-amber-400" />
        <h2 className="text-sm font-bold tracking-wide uppercase text-amber-300">
          Act Now — {actNowAgents.length} Agent{actNowAgents.length > 1 ? "s" : ""} Require Attention
        </h2>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {actNowAgents.map((agent) => {
          const isQuestion = agent.activity.activity === "WAITING_QUESTION";
          return (
            <div
              key={agent.id}
              onClick={() => onSelectAgent(agent)}
              className="flex items-start justify-between gap-3 p-3 rounded-lg bg-slate-900/90 border border-amber-900/60 hover:border-amber-600 transition-all cursor-pointer group"
            >
              <div className="flex items-start gap-2.5">
                {isQuestion ? (
                  <HelpCircle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
                ) : (
                  <AlertTriangle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
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

              <ArrowRight className="w-4 h-4 text-slate-500 group-hover:text-amber-400 shrink-0 transition-colors self-center" />
            </div>
          );
        })}
      </div>
    </div>
  );
};
