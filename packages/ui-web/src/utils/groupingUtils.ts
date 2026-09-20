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
