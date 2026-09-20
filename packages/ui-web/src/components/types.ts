import type React from "react";
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

export type IconSize = "xs" | "sm" | "md" | "lg" | "xl";
export type IconTone = "default" | "muted" | "primary" | "warning" | "error" | "success" | "purple" | "subtle";
export type IconAlign = "center" | "top";

export interface IIconWrapperProps {
  children: React.ReactNode;
  size?: IconSize;
  tone?: IconTone;
  align?: IconAlign;
}

export interface IViewSwitcherOption<T extends string = string> {
  id: T;
  label: string;
  icon?: React.ReactNode;
  testId?: string;
}

export interface IViewSwitcherProps<T extends string = string> {
  options: IViewSwitcherOption<T>[];
  value: T;
  onChange: (value: T) => void;
}

export interface IStatusBadgeProps {
  lifecycle: LifecycleState;
  activity: ActivityState;
}

export interface IHeaderProps {
  agents: IAgentState[];
  environments: IEnvironmentInfo[];
  isConnected: boolean;
  onOpenHelp?: () => void;
}

export interface IStateHelpDialogProps {
  onClose: () => void;
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
  toolName?: string | null;
  toolInput?: Record<string, unknown> | null;
  toolOutput?: string | null;
}

export interface ITokenUsageBarProps {
  usage: ITokenUsage;
}
