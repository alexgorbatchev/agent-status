import type { IAgentState } from "../types/types";

export function isAgentRequiringAttention(agent: IAgentState): boolean {
  return (
    agent.activity.lifecycle === "RUNNING" &&
    !agent.environment.isStale &&
    (agent.activity.activity === "WAITING_QUESTION" || agent.activity.activity === "ERROR")
  );
}
