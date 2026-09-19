import React, { useState, useEffect } from "react";
import type { IAgentState, ITelemetryFrame } from "../types";
import { StatusBadge } from "./StatusBadge";
import { X, Terminal, Brain, Wrench, AlertTriangle, Layers, ChevronRight, ChevronDown } from "lucide-react";

interface LogDrawerProps {
  agent: IAgentState | null;
  apiBaseUrl?: string;
  token?: string;
  onClose: () => void;
}

export const LogDrawer: React.FC<LogDrawerProps> = ({
  agent,
  apiBaseUrl = "http://localhost:4502",
  token = "dev-secret-token",
  onClose,
}) => {
  const [logs, setLogs] = useState<ITelemetryFrame[]>([]);
  const [subagentsOpen, setSubagentsOpen] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (!agent) {
      setLogs([]);
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
      .catch((err) => console.error("Failed to load agent logs:", err));

    // Live tail SSE connection
    const es = new EventSource(
      `${apiBaseUrl}/api/v1/agents/${encodeURIComponent(agent.id)}/live-tail?token=${encodeURIComponent(token)}`
    );

    es.onmessage = (msg) => {
      try {
        const frame: ITelemetryFrame = JSON.parse(msg.data);
        setLogs((prev) => [frame, ...prev]);
      } catch (err) {
        console.error("Failed to parse log frame:", err);
      }
    };

    return () => {
      es.close();
    };
  }, [agent, apiBaseUrl, token]);

  if (!agent) return null;

  return (
    <div className="fixed inset-y-0 right-0 w-full max-w-2xl bg-slate-950 border-l border-slate-800 shadow-2xl z-50 flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-slate-800 bg-slate-900/60">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <h2 className="text-sm font-bold text-slate-100 line-clamp-1">
              {agent.activity.title || "Agent Execution Log"}
            </h2>
            <StatusBadge lifecycle={agent.activity.lifecycle} activity={agent.activity.activity} />
          </div>
          <p className="text-xs text-slate-400 font-mono">
            {agent.environment.label} • {agent.project.name} • {agent.workspace.branch}
          </p>
        </div>

        <button
          onClick={onClose}
          className="p-1.5 rounded-lg hover:bg-slate-800 text-slate-400 hover:text-slate-200 transition-colors"
        >
          <X className="w-5 h-5" />
        </button>
      </div>

      {/* Live Log Stream */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3 font-mono text-xs">
        {logs.length === 0 ? (
          <div className="text-center py-12 text-slate-500">No telemetry log events recorded yet.</div>
        ) : (
          logs.map((log, idx) => {
            const isSubagent = Boolean(log.subagentId);
            return (
              <div
                key={idx}
                className={`p-3 rounded-lg border transition-all ${
                  isSubagent
                    ? "bg-purple-950/20 border-purple-900/40 ml-4"
                    : "bg-slate-900/80 border-slate-800"
                }`}
              >
                <div className="flex items-center justify-between gap-2 mb-1 text-[11px] text-slate-400">
                  <div className="flex items-center gap-1.5">
                    {log.eventType === "thinking" && <Brain className="w-3.5 h-3.5 text-blue-400" />}
                    {log.eventType === "tool_call" && <Wrench className="w-3.5 h-3.5 text-amber-400" />}
                    {log.eventType === "error" && <AlertTriangle className="w-3.5 h-3.5 text-red-400" />}
                    {isSubagent && (
                      <span className="px-1.5 py-0.2 rounded bg-purple-900/50 text-purple-300 text-[10px]">
                        Subagent: {log.subagentId}
                      </span>
                    )}
                    <span className="font-semibold text-slate-300 uppercase">{log.eventType}</span>
                  </div>
                  <span className="text-slate-500">
                    {new Date(log.timestamp).toLocaleTimeString()}
                  </span>
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
};
