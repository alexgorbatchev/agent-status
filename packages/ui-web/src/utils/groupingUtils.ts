import type { IAgentState, IEnvironmentGroupItem, IEnvironmentInfo, IProjectGroupItem } from "../types";

export type AgentCategory = "busy" | "idle" | "finished";

export const CATEGORY_ORDER: readonly AgentCategory[] = ["busy", "idle", "finished"] as const;

const CATEGORY_PRIORITY: Record<AgentCategory, number> = {
  busy: 0,
  idle: 1,
  finished: 2,
};

export function getAgentCategory(agent: IAgentState): AgentCategory {
  if (agent.activity.lifecycle === "EXITED") {
    return "finished";
  }
  if (agent.activity.activity === "IDLE") {
    return "idle";
  }
  return "busy";
}

export function groupAgentsByCategory(agents: IAgentState[]): Record<AgentCategory, IAgentState[]> {
  const groups: Record<AgentCategory, IAgentState[]> = {
    busy: [],
    idle: [],
    finished: [],
  };

  for (const agent of agents) {
    const category = getAgentCategory(agent);
    groups[category].push(agent);
  }

  return groups;
}

export function compareAgentsByCategory(a: IAgentState, b: IAgentState): number {
  const priorityA = CATEGORY_PRIORITY[getAgentCategory(a)];
  const priorityB = CATEGORY_PRIORITY[getAgentCategory(b)];
  return priorityA - priorityB;
}

export function sortAgents(agents: IAgentState[]): IAgentState[] {
  return [...agents].sort(compareAgentsByCategory);
}

export function groupAgentsByProject(agents: IAgentState[]): IProjectGroupItem[] {
  const groups: Record<string, IProjectGroupItem> = {};
  for (const agent of agents) {
    const projectId = agent.project.id || "unknown";
    const existing = groups[projectId];
    if (existing) {
      existing.agents.push(agent);
    } else {
      groups[projectId] = {
        id: projectId,
        name: agent.project.name || projectId,
        agents: [agent],
      };
    }
  }

  return Object.values(groups);
}

export function groupAgentsByEnvironment(
  agents: IAgentState[],
  environments: IEnvironmentInfo[],
): IEnvironmentGroupItem[] {
  const envMap = new Map<string, IEnvironmentInfo>();
  for (const env of environments) {
    envMap.set(env.id, env);
  }
  for (const agent of agents) {
    if (!envMap.has(agent.environment.id)) {
      envMap.set(agent.environment.id, agent.environment);
    }
  }

  const result: IEnvironmentGroupItem[] = [];
  for (const env of envMap.values()) {
    const envAgents = agents.filter((a) => a.environment.id === env.id);
    result.push({
      environment: env,
      agents: envAgents,
    });
  }
  return result;
}
