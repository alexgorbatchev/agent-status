import React, { useState, useEffect } from "react";
import type { ILogDrawerProps } from "./types";
import type { ITelemetryFrame } from "../types";
import { StatusBadge } from "./StatusBadge";
import { LogFrameItem } from "./LogFrameItem";
import { DEFAULT_DEV_TOKEN, resolveApiBaseUrl } from "../utils/config";
import { X } from "lucide-react";

export function LogDrawer({
  agent,
  apiBaseUrl,
  token = DEFAULT_DEV_TOKEN,
  onClose,
}: ILogDrawerProps): React.JSX.Element | null {
  const resolvedBaseUrl = resolveApiBaseUrl(apiBaseUrl);
  const [logs, setLogs] = useState<ITelemetryFrame[]>([]);
  const [prevAgentId, setPrevAgentId] = useState(agent?.id);

  if (agent?.id !== prevAgentId) {
    setPrevAgentId(agent?.id);
    setLogs([]);
  }

  useEffect(() => {
    if (!agent) {
      return;
    }

    // Fetch initial logs
    fetch(`${resolvedBaseUrl}/api/v1/agents/${encodeURIComponent(agent.id)}/logs?limit=100`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data)) {
          setLogs(data);
        }
      })
      .catch((error) => console.error("Failed to load agent logs:", error));

    // Live tail SSE connection
    const es = new EventSource(
      `${resolvedBaseUrl}/api/v1/agents/${encodeURIComponent(agent.id)}/live-tail?token=${encodeURIComponent(token)}`,
    );

    es.onmessage = (msg) => {
      try {
        const frame: ITelemetryFrame = JSON.parse(msg.data);
        setLogs((prev) => [frame, ...prev]);
      } catch (error) {
        console.error("Failed to parse log frame:", error);
      }
    };

    return () => {
      es.close();
    };
  }, [agent, resolvedBaseUrl, token]);

  if (!agent) return null;

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
          data-testid="LogDrawer--close-button"
          onClick={onClose}
          className="p-1.5 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors"
        >
          <span className="w-5 h-5 flex items-center justify-center">
            <X size={20} />
          </span>
        </button>
      </div>

      {/* Live Log Stream */}
      <div data-testid="LogDrawer--stream" className="flex-1 overflow-y-auto p-4 space-y-3 text-xs">
        {logs.length === 0 ? (
          <div className="text-center py-12 text-slate-500 font-mono">No telemetry log events recorded yet.</div>
        ) : (
          logs.map((log, idx) => <LogFrameItem key={idx} frame={log} />)
        )}
      </div>
    </div>
  );
}
