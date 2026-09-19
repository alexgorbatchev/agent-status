import type { IAgentState, IEnvironmentInfo } from "../types";

export interface IStatusBadgeProps {
  lifecycle: IAgentState["activity"]["lifecycle"];
  activity: IAgentState["activity"]["activity"];
}

export interface IHeaderProps {
  agents: IAgentState[];
  environments: IEnvironmentInfo[];
  isConnected: boolean;
}

export interface IAttentionBannerProps {
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
}

export interface IAgentCardProps {
  agent: IAgentState;
  onOpenDrawer?: (agent: IAgentState) => void;
}

export interface IProjectGroupProps {
  projectName: string;
  projectId: string;
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
}

export interface IEnvironmentGroupProps {
  environment: IEnvironmentInfo;
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
}

export interface ILogDrawerProps {
  agent: IAgentState | null;
  apiBaseUrl?: string;
  token?: string;
  onClose: () => void;
}
