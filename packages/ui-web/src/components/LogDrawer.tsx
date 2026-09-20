import React, { useState, useEffect } from "react";
import type { ILogDrawerProps } from "./types";
import type { ITelemetryFrame, IWindowWithAgentStatus } from "../types";
import { StatusBadge } from "./StatusBadge";
import { X, Brain, Wrench, AlertTriangle, User, Play, CheckCircle2 } from "lucide-react";
import Markdown from "react-markdown";

export function LogDrawer({ agent, apiBaseUrl, token = "dev-secret-token", onClose }: ILogDrawerProps) {
  const resolvedBaseUrl =
    apiBaseUrl ||
    (typeof window !== "undefined" && (window as unknown as IWindowWithAgentStatus).__AGENT_STATUS_API_URL__) ||
    "http://localhost:4502";
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
          logs.map((log, idx) => {
            const isSubagent = Boolean(log.subagentId);
            const isUser = log.data.role === "user" || log.eventType === "turn_start";
            return (
              <div
                key={idx}
                className={`p-3 rounded-lg border transition-all ${
                  isUser
                    ? "bg-blue-950/20 border-blue-800/40"
                    : isSubagent
                      ? "bg-purple-950/20 border-purple-900/40 ml-4"
                      : "bg-slate-900/80 border-slate-800"
                }`}
              >
                <div className="flex items-center justify-between gap-2 mb-1 text-[11px] text-slate-400 font-mono">
                  <div className="flex items-center gap-1.5">
                    {isUser ? (
                      <span className="w-3.5 h-3.5 text-blue-400 flex items-center justify-center">
                        <User size={14} />
                      </span>
                    ) : log.eventType === "thinking" ? (
                      <span className="w-3.5 h-3.5 text-blue-400 flex items-center justify-center">
                        <Brain size={14} />
                      </span>
                    ) : log.eventType === "tool_call" ? (
                      <span className="w-3.5 h-3.5 text-amber-400 flex items-center justify-center">
                        <Wrench size={14} />
                      </span>
                    ) : log.eventType === "error" ? (
                      <span className="w-3.5 h-3.5 text-red-400 flex items-center justify-center">
                        <AlertTriangle size={14} />
                      </span>
                    ) : log.eventType === "run_started" ? (
                      <span className="w-3.5 h-3.5 text-emerald-400 flex items-center justify-center">
                        <Play size={14} />
                      </span>
                    ) : log.eventType === "turn_end" ? (
                      <span className="w-3.5 h-3.5 text-slate-400 flex items-center justify-center">
                        <CheckCircle2 size={14} />
                      </span>
                    ) : null}
                    {isSubagent && (
                      <span className="px-1.5 py-0.5 rounded bg-purple-900/50 text-purple-300 text-[10px]">
                        Subagent: {log.subagentId}
                      </span>
                    )}
                    <span className="font-semibold text-slate-300 uppercase">
                      {isUser ? "USER PROMPT" : log.eventType}
                    </span>
                  </div>
                  <span className="text-slate-500">{new Date(log.timestamp).toLocaleTimeString()}</span>
                </div>

                {log.data.content && (
                  <div className="text-slate-300 prose prose-invert max-w-none text-xs leading-relaxed break-words space-y-1">
                    <Markdown>{log.data.content}</Markdown>
                  </div>
                )}

                {log.data.question && (
                  <div className="mt-1 p-2 rounded bg-amber-950/30 border border-amber-800/40 text-amber-200">
                    <span className="font-semibold text-[11px] block mb-1">Question:</span>
                    <Markdown>{log.data.question}</Markdown>
                  </div>
                )}

                {log.data.toolName && (
                  <div className="mt-1 font-mono">
                    <span className="text-amber-300 font-semibold">{log.data.toolName}</span>
                    {log.data.toolInput && (
                      <pre className="mt-1 p-2 rounded bg-slate-950 text-slate-400 text-[10px] overflow-x-auto">
                        {JSON.stringify(log.data.toolInput, null, 2)}
                      </pre>
                    )}
                  </div>
                )}

                {log.data.toolOutput && (
                  <div className="mt-1 p-2 rounded bg-slate-950/80 text-slate-300 font-mono text-[10px] overflow-x-auto max-h-40">
                    {log.data.toolOutput}
                  </div>
                )}

                {log.data.usage && (
                  <div className="mt-1.5 pt-1.5 border-t border-slate-800/60 flex items-center gap-3 text-[10px] text-slate-500 font-mono">
                    <span>In: {log.data.usage.input.toLocaleString()}</span>
                    <span>Out: {log.data.usage.output.toLocaleString()}</span>
                    {log.data.usage.cacheRead > 0 && <span>Cache: {log.data.usage.cacheRead.toLocaleString()}</span>}
                    <span>Total: {log.data.usage.total.toLocaleString()}</span>
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
