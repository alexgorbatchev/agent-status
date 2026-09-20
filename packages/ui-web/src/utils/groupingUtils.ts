import type { IAgentState, IEnvironmentGroupItem, IEnvironmentInfo, IProjectGroupItem } from "../types";

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
  const map = new Map<string, IAgentState[]>();
  for (const agent of agents) {
    const environmentId = agent.environment.id;
    const existing = map.get(environmentId);
    if (existing) {
      existing.push(agent);
    } else {
      map.set(environmentId, [agent]);
    }
  }

  return environments.map((environment) => ({
    environment,
    agents: map.get(environment.id) || [],
  }));
}
