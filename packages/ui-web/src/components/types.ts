import type {
  IAgentState,
  IEnvironmentInfo,
  ITelemetryFrame,
  ITokenUsage,
  LifecycleState,
  ActivityState,
} from "../types";

export interface IStatusBadgeConfig {
  label: string;
  badgeClass: string;
  indicatorClass: string;
}

export interface IStatusBadgeProps {
  lifecycle: LifecycleState;
  activity: ActivityState;
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

export interface IAgentGroupSectionProps {
  testId?: string;
  icon: React.ReactNode;
  iconColorClass?: string;
  title: string;
  subtitle: string;
  badges?: React.ReactNode;
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
}

export interface ILogDrawerProps {
  agent: IAgentState | null;
  apiBaseUrl?: string;
  token?: string;
  onClose: () => void;
}

export interface ILogFrameItemProps {
  frame: ITelemetryFrame;
}

export interface IToolExecutionBlockProps {
  toolName: string;
  toolInput?: Record<string, unknown> | null;
  toolOutput?: string | null;
}

export interface ITokenUsageBarProps {
  usage: ITokenUsage;
}
