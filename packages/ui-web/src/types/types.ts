export type LifecycleState = "RUNNING" | "EXITED";

export type ActivityState = "BUSY" | "WAITING_QUESTION" | "ERROR" | "RATE_LIMITED" | "IDLE";

export type HarnessType = "claude-code" | "pi" | "codex" | "opencode";

export type AgentPhase = "planning" | "editing" | "testing" | "debugging" | "investigating";

export type GroupByOption = "project" | "environment";

export interface IEnvironmentInfo {
  id: string; // sha256(machine_id + ":" + uid)[:16]
  label: string; // Mutable display name: e.g. "work-macbook"
  hostname: string;
  platform: "darwin" | "linux";
  user: string;
  isStale: boolean; // true if heartbeat > 30s
  lastHeartbeatAt: number;
}

export interface IWorkspaceInfo {
  isWorktree: boolean;
  branch: string;
  isDetached: boolean;
  relPath: string | null;
  cwd: string;
}

export interface IProjectInfo {
  id: string; // Normalized origin e.g. "github.com/alexgorbatchev/ai-registry"
  name: string; // Display name e.g. "ai-registry"
  canonicalRoot: string;
}

export interface ITokenUsage {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  reasoning: number;
  total: number; // Sum of all 5 components
}

export interface ICostInfo {
  currency: "USD";
  total: number; // Direct + subagent cost in USD
  isEstimated: boolean;
}

export interface IRunInfo {
  id: string;
  pid: number;
  harnessVersion: string;
  startedAt: number;
  endedAt: number | null;
}

export type SubagentStatus = "running" | "completed" | "settling";

export interface ISubagentSummary {
  id: string;
  activity: ActivityState;
  activeTool?: string | null;
  action?: string;
  lastError?: string | null;
  status?: SubagentStatus;
  startedAt?: number;
  completedAt?: number | null;
}

export interface IAgentActivity {
  lifecycle: LifecycleState;
  activity: ActivityState;
  title: string; // Harness rolling title or prompt summary
  action: string; // LLM generated 1-sentence current action
  actionStale: boolean;
  phase: AgentPhase | null;
  activeTool: string | null;
  activeTarget: string | null;
  activeToolStartedAt?: number | null;
  question: string | null; // Active prompt when WAITING_QUESTION
  lastThinkingExcerpt: string | null;
  lastError: string | null;
  rateLimitResetAt?: number | null;
  retryAfterSeconds?: number | null;
  model?: string | null;
  contextWindow?: number | null;
  contextFullness?: number | null;
  usage: ITokenUsage;
  cost: ICostInfo | null;
  subagentCount: number;
  activeSubagents?: ISubagentSummary[];
  currentRun: IRunInfo | null;
  firstStartedAt: number;
  updatedAt: number;
  hasUnseenCompletion?: boolean | null;
  completedAt?: number | null;
  has_unseen_completion?: boolean | null;
  completed_at?: number | null;
}

export interface IAgentState {
  id: string; // `${envId}:${harness}:${sessionId}`
  sessionId: string;
  harness: HarnessType;
  environment: IEnvironmentInfo;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  activity: IAgentActivity;
  isHidden?: boolean;
  hiddenAt?: number | null;
}

export interface IWordToken {
  type: "add" | "del" | "same";
  value: string;
}

export interface IDiffLine {
  type: "add" | "del" | "ctx";
  oldLineNumber?: number;
  newLineNumber?: number;
  content: string;
  wordTokens?: IWordToken[];
}

export interface IDiffHunk {
  header?: string;
  lines: IDiffLine[];
}

export interface IFileModification {
  action: "EDIT" | "WRITE";
  filePath: string;
  hunks?: IDiffHunk[];
  content?: string;
}

export interface ITelemetryFrameData {
  role: "user" | "assistant" | "system" | null;
  content: string | null;
  title: string | null;
  lastPrompt: string | null;
  model: string | null;
  modelContextWindow?: number | null;
  contextFullness?: number | null;
  toolName: string | null;
  toolCallId: string | null;
  toolInput: Record<string, unknown> | null;
  fileModification?: IFileModification | null;
  toolOutput: string | null;
  isError: boolean | null;
  isRateLimit?: boolean | null;
  rateLimitResetAt?: number | null;
  retryAfterSeconds?: number | null;
  exitCode: number | null;
  durationMs?: number | null;
  question: string | null;
  stopReason: string | null;
  usage: ITokenUsage | null;
  rawHarnessCost: number | null;
}

export type TelemetryEventType =
  | "run_started"
  | "run_exited"
  | "turn_start"
  | "thinking"
  | "tool_call"
  | "tool_result"
  | "question_prompt"
  | "error"
  | "turn_end"
  | "title_change";

export interface ITelemetryFrame {
  version: "1.0";
  seq: number;
  envId: string;
  environment: IEnvironmentInfo;
  agentId: string;
  sessionId: string;
  subagentId: string | null; // Set if event belongs to a subagent
  pid: number;
  harness: HarnessType;
  harnessVersion: string;
  project: IProjectInfo;
  workspace: IWorkspaceInfo;
  timestamp: number;
  eventType: TelemetryEventType;
  data: ITelemetryFrameData;
}

export interface IWorktreeBranchGroup {
  worktreeKey: string;
  branch?: string;
  relPath?: string;
  agents: IAgentState[];
}

export interface IProjectTreeGroup {
  id: string;
  name: string;
  rootAgents: IAgentState[]; // agents on main checkout (isWorktree != true)
  worktrees: IWorktreeBranchGroup[]; // linked worktree branches
  allAgents: IAgentState[]; // convenience flat list for counts / total spend
}

export interface IProjectGroupItem extends IProjectTreeGroup {}

export interface IEnvironmentGroupItem {
  environment: IEnvironmentInfo;
  agents: IAgentState[];
}

export interface IWindowWithAgentStatus {
  __AGENT_STATUS_API_URL__?: string;
}

export type ObserverMessage =
  | { type: "hello"; envId: string; environment: IEnvironmentInfo; since: number; dropped: number }
  | { type: "frames"; frames: ITelemetryFrame[] };

export type IngesterMessage =
  | { type: "ack"; upTo: number }
  | { type: "error"; code: "unsupported_version" | "unauthorized" | "malformed"; message: string };

export type ServerEvent =
  | { type: "snapshot"; at: number; agents: IAgentState[]; environments: IEnvironmentInfo[] }
  | { type: "agent_upsert"; at: number; agent: IAgentState }
  | { type: "agent_removed"; at: number; agentId: string }
  | { type: "environment_upsert"; at: number; environment: IEnvironmentInfo }
  | { type: "log_append"; at: number; agentId: string; event: ITelemetryFrame };

export interface IEventStreamState {
  agents: IAgentState[];
  environments: IEnvironmentInfo[];
  isConnected: boolean;
  error: string | null;
  updateAgent?: (agent: IAgentState) => void;
}
