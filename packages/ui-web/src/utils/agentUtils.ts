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

export function formatRelativeTime(timestamp: number, now: number = Date.now()): string {
  if (!timestamp || timestamp <= 0 || Number.isNaN(timestamp)) {
    return "—";
  }

  const diffMs = now - timestamp;
  if (diffMs < 5000) {
    return "just now";
  }

  if (diffMs < 60000) {
    const seconds = Math.floor(diffMs / 1000);
    return `${seconds}s ago`;
  }

  if (diffMs < 3600000) {
    const minutes = Math.floor(diffMs / 60000);
    return `${minutes}m ago`;
  }

  if (diffMs < 86400000) {
    const hours = Math.floor(diffMs / 3600000);
    return `${hours}h ago`;
  }

  const days = Math.floor(diffMs / 86400000);
  return `${days}d ago`;
}

export function formatExactTime(timestamp: number): string {
  if (!timestamp || timestamp <= 0 || Number.isNaN(timestamp)) {
    return "Never";
  }
  return new Date(timestamp).toLocaleString();
}

export function getAgentDebugCommand(agent: IAgentState): string {
  return `just debug session replay --harness ${agent.harness} ${agent.sessionId}`;
}
