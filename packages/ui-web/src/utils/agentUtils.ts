import type { IAgentState } from "../types";

export function calculateTotalSpend(agents: IAgentState[]): number {
  return agents.reduce((acc, a) => acc + (a.activity.cost?.total || 0), 0);
}

export function formatCurrency(amount: number, precision: number = 2): string {
  return `$${amount.toFixed(precision)}`;
}

export function filterActiveAgents(agents: IAgentState[]): IAgentState[] {
  return agents.filter((a) => a.activity.lifecycle === "RUNNING" && a.activity.activity === "BUSY");
}

export function filterRunningAgents(agents: IAgentState[]): IAgentState[] {
  return agents.filter((a) => a.activity.lifecycle === "RUNNING");
}

export function isAgentRequiringAttention(agent: IAgentState): boolean {
  return (
    agent.activity.lifecycle === "RUNNING" &&
    !agent.environment.isStale &&
    (agent.activity.activity === "WAITING_QUESTION" || agent.activity.activity === "ERROR")
  );
}
