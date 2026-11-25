import type {
  IAgentState,
  IEnvironmentGroupItem,
  IEnvironmentInfo,
  IProjectGroupItem,
  IProjectTreeGroup,
} from "../types";
import { isAgentStaleIdle } from "./agentUtils";

export type AgentCategory = "busy" | "idle" | "finished";

export const CATEGORY_ORDER: readonly AgentCategory[] = ["busy", "idle", "finished"];

export function getAgentCategory(agent: IAgentState): AgentCategory {
  if (agent.activity.lifecycle === "EXITED") {
    return "finished";
  }
  if (agent.activity.activity === "IDLE") {
    return "idle";
  }
  return "busy";
}

export function groupAgentsByCategory(
  agents: IAgentState[],
  now: number = Date.now(),
): Record<AgentCategory, IAgentState[]> {
  const groups: Record<AgentCategory, IAgentState[]> = {
    busy: [],
    idle: [],
    finished: [],
  };

  for (const agent of agents) {
    const category = getAgentCategory(agent);
    groups[category].push(agent);
  }

  groups.busy.sort((a, b) => b.activity.updatedAt - a.activity.updatedAt);

  const warmIdle: IAgentState[] = [];
  const staleIdle: IAgentState[] = [];
  for (const agent of groups.idle) {
    if (isAgentStaleIdle(agent.activity, now)) {
      staleIdle.push(agent);
    } else {
      warmIdle.push(agent);
    }
  }
  groups.idle = [...warmIdle, ...staleIdle];

  groups.finished.sort((a, b) => b.activity.updatedAt - a.activity.updatedAt);

  return groups;
}

export function groupAgentsByProjectHierarchy(agents: IAgentState[]): IProjectTreeGroup[] {
  const projectMap = new Map<string, IProjectTreeGroup>();

  for (const agent of agents) {
    const projectId = agent.project.id || "unknown";
    const projectName = agent.project.name || projectId;

    let projectGroup = projectMap.get(projectId);
    if (!projectGroup) {
      projectGroup = {
        id: projectId,
        name: projectName,
        rootAgents: [],
        worktrees: [],
        allAgents: [],
      };
      projectMap.set(projectId, projectGroup);
    }

    projectGroup.allAgents.push(agent);

    if (agent.workspace.isWorktree) {
      const worktreeKey = agent.workspace.relPath || agent.workspace.branch || agent.workspace.cwd || "unknown";
      let wtGroup = projectGroup.worktrees.find((w) => w.worktreeKey === worktreeKey);
      if (!wtGroup) {
        wtGroup = {
          worktreeKey,
          branch: agent.workspace.branch || undefined,
          relPath: agent.workspace.relPath || undefined,
          agents: [],
        };
        projectGroup.worktrees.push(wtGroup);
      }
      wtGroup.agents.push(agent);
    } else {
      projectGroup.rootAgents.push(agent);
    }
  }

  return Array.from(projectMap.values());
}

export function groupAgentsByProject(agents: IAgentState[]): IProjectGroupItem[] {
  return groupAgentsByProjectHierarchy(agents);
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
