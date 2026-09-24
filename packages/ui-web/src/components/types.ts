import type React from "react";
import type { IFileModificationPayload } from "../utils/diffUtils";
import type { LogBuffer } from "../utils/LogBuffer";
import type {
  IAgentState,
  IEnvironmentInfo,
  ITelemetryFrame,
  ITokenUsage,
  LifecycleState,
  ActivityState,
  IFileModification,
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
  spin?: boolean;
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
  stallTier?: "healthy" | "notice" | "warning" | "stalled";
  toolDuration?: string;
  isSessionStalled?: boolean;
  silenceDuration?: string;
}

export interface IHeaderProps {
  agents: IAgentState[];
  environments: IEnvironmentInfo[];
  isConnected: boolean;
  isActiveFilter?: boolean;
  onToggleActiveFilter?: () => void;
  onOpenHelp?: () => void;
  now?: number;
}

export interface IStateHelpDialogProps {
  onClose: () => void;
}

export interface IAttentionBannerProps {
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
  now?: number;
}

export interface IAgentCardProps {
  agent: IAgentState;
  projectName?: string;
  onOpenDrawer?: (agent: IAgentState) => void;
  onToggleHide?: (agent: IAgentState) => void;
  defaultShowSubagents?: boolean;
}

export interface IProjectGroupProps {
  projectName: string;
  projectId: string;
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
  onToggleHide?: (agent: IAgentState) => void;
}

export interface IEnvironmentGroupProps {
  environment: IEnvironmentInfo;
  agents: IAgentState[];
  onSelectAgent: (agent: IAgentState) => void;
  onToggleHide?: (agent: IAgentState) => void;
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
  onToggleHide?: (agent: IAgentState) => void;
}

export interface ILogDrawerProps {
  agent: IAgentState | null;
  apiBaseUrl?: string;
  token?: string;
  onClose: () => void;
  logBuffer?: LogBuffer;
}

export interface ILogFrameItemProps {
  frame: ITelemetryFrame;
}

export interface IToolExecutionBlockProps {
  toolName?: string | null;
  toolInput?: Record<string, unknown> | null;
  fileModification?: IFileModification | null;
  toolOutput?: string | null;
  exitCode?: number | null;
  isError?: boolean | null;
  toolCallId?: string | null;
  durationMs?: number | null;
}

export interface IAnsiTextProps {
  text: string;
  className?: string;
}

export interface ITokenUsageBarProps {
  usage: ITokenUsage;
  contextWindow?: number | null;
  contextFullness?: number | null;
}

export interface IDiffViewerProps {
  payload: IFileModificationPayload;
}
