import type { IAgentState } from "../types";
import type { LogBuffer } from "../utils/LogBuffer";

export interface ILogDrawerMachineContext {
  agent: IAgentState | null;
  apiBaseUrl: string;
  token: string;
  logBuffer: LogBuffer;
  errorMessage: string | null;
  retryCount: number;
}

export type LogDrawerMachineEvent =
  | { type: "OPEN"; agent: IAgentState; apiBaseUrl?: string; token?: string }
  | { type: "CLOSE" }
  | { type: "SELECT_AGENT"; agent: IAgentState | null; apiBaseUrl?: string; token?: string }
  | { type: "SSE_ERROR"; error?: string }
  | { type: "SSE_OPEN" }
  | { type: "RETRY" };

export interface ILogDrawerMachineInput {
  agent?: IAgentState | null;
  apiBaseUrl?: string;
  token?: string;
  logBuffer?: LogBuffer;
}

export interface IFetchHistoryInput {
  agentId: string;
  apiBaseUrl: string;
  token: string;
}

export interface IStreamLiveTailInput {
  agentId: string;
  apiBaseUrl: string;
  token: string;
  logBuffer: LogBuffer;
}

export type StreamCleanupFn = () => void;
export type StreamLiveTailSendBack = (event: LogDrawerMachineEvent) => void;

export interface IFetchHistoryArgs {
  input: IFetchHistoryInput;
  signal: AbortSignal;
}

export interface IStreamLiveTailArgs {
  input: IStreamLiveTailInput;
  sendBack: StreamLiveTailSendBack;
}

export type LogDrawerStateValue = "idle" | "fetchingHistory" | "streamingLive" | "reconnecting" | "error";
