import type React from "react";
import type { IAgentState, IEnvironmentInfo } from "../types";

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
