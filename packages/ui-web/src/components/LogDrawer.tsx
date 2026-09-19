import React, { useState, useEffect } from "react";
import type { ILogDrawerProps } from "./types";
import type { ITelemetryFrame } from "../types";
import { StatusBadge } from "./StatusBadge";
import { X, Brain, Wrench, AlertTriangle } from "lucide-react";

export function LogDrawer({
  agent,
  apiBaseUrl = "http://localhost:4502",
  token = "dev-secret-token",
  onClose,
}: ILogDrawerProps) {
  const [logs, setLogs] = useState<ITelemetryFrame[]>([]);

  useEffect(() => {
    if (!agent) {
      return;
    }

    // Fetch initial logs
    fetch(`${apiBaseUrl}/api/v1/agents/${encodeURIComponent(agent.id)}/logs?limit=100`, {
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
      `${apiBaseUrl}/api/v1/agents/${encodeURIComponent(agent.id)}/live-tail?token=${encodeURIComponent(token)}`,
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
  }, [agent, apiBaseUrl, token]);

  if (!agent) return null;

  return (
    <div
      data-testid="LogDrawer"
      className="fixed inset-y-0 right-0 w-full max-w-2xl bg-slate-950 border-l border-slate-800 shadow-2xl z-50 flex flex-col"
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
      <div data-testid="LogDrawer--stream" className="flex-1 overflow-y-auto p-4 space-y-3 font-mono text-xs">
        {logs.length === 0 ? (
          <div className="text-center py-12 text-slate-500">No telemetry log events recorded yet.</div>
        ) : (
          logs.map((log, idx) => {
            const isSubagent = Boolean(log.subagentId);
            return (
              <div
                key={idx}
                className={`p-3 rounded-lg border transition-all ${
                  isSubagent ? "bg-purple-950/20 border-purple-900/40 ml-4" : "bg-slate-900/80 border-slate-800"
                }`}
              >
                <div className="flex items-center justify-between gap-2 mb-1 text-[11px] text-slate-400">
                  <div className="flex items-center gap-1.5">
                    {log.eventType === "thinking" && (
                      <span className="w-3.5 h-3.5 text-blue-400 flex items-center justify-center">
                        <Brain size={14} />
                      </span>
                    )}
                    {log.eventType === "tool_call" && (
                      <span className="w-3.5 h-3.5 text-amber-400 flex items-center justify-center">
                        <Wrench size={14} />
                      </span>
                    )}
                    {log.eventType === "error" && (
                      <span className="w-3.5 h-3.5 text-red-400 flex items-center justify-center">
                        <AlertTriangle size={14} />
                      </span>
                    )}
                    {isSubagent && (
                      <span className="px-1.5 py-0.2 rounded bg-purple-900/50 text-purple-300 text-[10px]">
                        Subagent: {log.subagentId}
                      </span>
                    )}
                    <span className="font-semibold text-slate-300 uppercase">{log.eventType}</span>
                  </div>
                  <span className="text-slate-500">{new Date(log.timestamp).toLocaleTimeString()}</span>
                </div>

                {log.data.content && (
                  <p className="text-slate-300 whitespace-pre-wrap break-words">{log.data.content}</p>
                )}

                {log.data.toolName && (
                  <div className="mt-1">
                    <span className="text-amber-300 font-semibold">{log.data.toolName}</span>
                    {log.data.toolInput && (
                      <pre className="mt-1 p-2 rounded bg-slate-950 text-slate-400 text-[10px] overflow-x-auto">
                        {JSON.stringify(log.data.toolInput, null, 2)}
                      </pre>
                    )}
                  </div>
                )}

                {log.data.toolOutput && (
                  <div className="mt-1 p-2 rounded bg-slate-950/80 text-slate-300 text-[10px] overflow-x-auto max-h-40">
                    {log.data.toolOutput}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
