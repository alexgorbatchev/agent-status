import React, { useEffect, useMemo, useSyncExternalStore } from "react";
import { useMachine } from "@xstate/react";
import type { ILogDrawerProps } from "./types";
import { StatusBadge } from "./StatusBadge";
import { LogFrameItem } from "./LogFrameItem";
import { IconWrapper } from "./IconWrapper";
import { DEFAULT_DEV_TOKEN } from "../utils/config";
import { LogBuffer } from "../utils/LogBuffer";
import { logDrawerMachine } from "../machines/logDrawerMachine";
import { X, RefreshCw, AlertCircle } from "lucide-react";

export function LogDrawer({
  agent,
  apiBaseUrl,
  token = DEFAULT_DEV_TOKEN,
  onClose,
}: ILogDrawerProps): React.JSX.Element | null {
  const logBuffer = useMemo(() => new LogBuffer(), []);
  const [snapshot, send] = useMachine(logDrawerMachine, {
    input: {
      agent,
      apiBaseUrl,
      token,
      logBuffer,
    },
  });

  const logs = useSyncExternalStore(logBuffer.subscribe, logBuffer.getSnapshot, logBuffer.getSnapshot);

  useEffect(() => {
    if (agent) {
      send({ type: "SELECT_AGENT", agent, apiBaseUrl, token });
    } else {
      send({ type: "CLOSE" });
    }
  }, [agent, apiBaseUrl, token, send]);

  if (!agent) {
    return null;
  }

  const isError = snapshot.matches("error");
  const isReconnecting = snapshot.matches("reconnecting");
  const isFetchingHistory = snapshot.matches("fetchingHistory");

  return (
    <div
      data-testid="LogDrawer"
      className="fixed inset-y-0 right-0 w-full max-w-2xl bg-slate-950 border-l border-slate-800 shadow-2xl z-50 flex flex-col font-sans"
    >
      {/* Header */}
      <div
        data-testid="LogDrawer--header"
        className="flex items-center justify-between p-4 border-b border-slate-800 bg-slate-900/60"
      >
        <div>
          <div className="flex items-center gap-2 mb-1">
            <h2 data-testid="LogDrawer--title" className="text-sm font-bold text-slate-100 line-clamp-1">
              {agent.activity.title || "Agent Execution Log"}
            </h2>
            <StatusBadge lifecycle={agent.activity.lifecycle} activity={agent.activity.activity} />
          </div>
          <p className="text-xs text-slate-400 font-mono">
            {agent.environment.label} • {agent.project.name} • {agent.workspace.branch}
          </p>
        </div>

        <button
          type="button"
          aria-label="Close log drawer"
          data-testid="LogDrawer--close-button"
          onClick={onClose}
          className="p-1.5 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors"
        >
          <IconWrapper size="lg">
            <X size={20} />
          </IconWrapper>
        </button>
      </div>

      {/* Error / Reconnect Banner */}
      {isError && (
        <div
          data-testid="LogDrawer--error"
          className="p-3 bg-red-950/70 border-b border-red-800/80 flex items-center justify-between text-xs text-red-200 font-mono"
        >
          <div className="flex items-center gap-2">
            <IconWrapper size="sm" tone="error">
              <AlertCircle size={14} />
            </IconWrapper>
            <span>{snapshot.context.errorMessage ?? "Failed to load agent telemetry logs"}</span>
          </div>
          <button
            type="button"
            data-testid="LogDrawer--retry-button"
            onClick={() => send({ type: "RETRY" })}
            className="px-2 py-1 bg-red-900/60 hover:bg-red-800/80 rounded text-red-100 flex items-center gap-1 transition-colors"
          >
            <IconWrapper size="xs">
              <RefreshCw size={12} />
            </IconWrapper>
            <span>Retry</span>
          </button>
        </div>
      )}

      {isReconnecting && (
        <div
          data-testid="LogDrawer--reconnecting"
          className="p-2 bg-amber-950/60 border-b border-amber-800/60 flex items-center gap-2 text-xs text-amber-200 font-mono"
        >
          <IconWrapper size="xs" tone="warning">
            <RefreshCw size={12} />
          </IconWrapper>
          <span>Live tail disconnected. Reconnecting...</span>
        </div>
      )}

      {/* Live Log Stream */}
      <div data-testid="LogDrawer--stream" className="flex-1 overflow-y-auto p-4 space-y-3 text-xs">
        {logs.length === 0 ? (
          <div className="text-center py-12 text-slate-500 font-mono">
            {isFetchingHistory ? "Loading telemetry log events..." : "No telemetry log events recorded yet."}
          </div>
        ) : (
          logs.map((log, idx) => <LogFrameItem key={`${log.seq}-${log.timestamp}-${idx}`} frame={log} />)
        )}
      </div>
    </div>
  );
}
