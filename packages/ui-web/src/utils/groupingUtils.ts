import type { ActivityState, IAgentState, IEnvironmentGroupItem, IEnvironmentInfo, IProjectGroupItem } from "../types";

const ACTIVITY_PRIORITY: Record<ActivityState, number> = {
  BUSY: 0,
  WAITING_QUESTION: 1,
  ERROR: 2,
  IDLE: 3,
};

export function compareAgentsByActivity(a: IAgentState, b: IAgentState): number {
  const priorityA = ACTIVITY_PRIORITY[a.activity.activity] ?? 999;
  const priorityB = ACTIVITY_PRIORITY[b.activity.activity] ?? 999;
  if (priorityA !== priorityB) {
    return priorityA - priorityB;
  }
  return (b.activity?.updatedAt ?? 0) - (a.activity?.updatedAt ?? 0);
}

export function sortAgents(agents: IAgentState[]): IAgentState[] {
  return [...agents].sort(compareAgentsByActivity);
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

  const result = Object.values(groups);
  for (const group of result) {
    group.agents.sort(compareAgentsByActivity);
  }
  return result;
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
    envAgents.sort(compareAgentsByActivity);
    result.push({
      environment: env,
      agents: envAgents,
    });
  }
  return result;
}
