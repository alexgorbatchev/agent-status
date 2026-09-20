import { useEffect, useState, useCallback } from "react";
import type { IAgentState, IEnvironmentInfo, IWindowWithAgentStatus, ServerEvent } from "../types";

export interface IEventStreamState {
  agents: IAgentState[];
  environments: IEnvironmentInfo[];
  isConnected: boolean;
  error: string | null;
}

export function useEventStream(apiBaseUrl?: string, token: string = "dev-secret-token"): IEventStreamState {
  const resolvedBaseUrl =
    apiBaseUrl ||
    (typeof window !== "undefined" && (window as unknown as IWindowWithAgentStatus).__AGENT_STATUS_API_URL__) ||
    "http://localhost:4502";
  const [agents, setAgents] = useState<IAgentState[]>([]);
  const [environments, setEnvironments] = useState<IEnvironmentInfo[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleServerEvent = useCallback((event: ServerEvent) => {
    switch (event.type) {
      case "snapshot":
        if (event.agents) setAgents(event.agents);
        if (event.environments) setEnvironments(event.environments);
        break;

      case "agent_upsert":
        if (event.agent) {
          setAgents((prev) => {
            const idx = prev.findIndex((a) => a.id === event.agent?.id);
            if (idx >= 0) {
              const updated = [...prev];
              updated[idx] = event.agent;
              return updated;
            }
            return [event.agent, ...prev];
          });
        }
        break;

      case "agent_removed":
        if (event.agentId) {
          setAgents((prev) => prev.filter((a) => a.id !== event.agentId));
        }
        break;

      case "environment_upsert":
        if (event.environment) {
          setEnvironments((prev) => {
            const idx = prev.findIndex((e) => e.id === event.environment?.id);
            if (idx >= 0) {
              const updated = [...prev];
              updated[idx] = event.environment;
              return updated;
            }
            return [...prev, event.environment];
          });
        }
        break;
    }
  }, []);

  useEffect(() => {
    let es: EventSource | null = null;
    let retryTimeout: Timer | null = null;

    const handleMockEvent = (e: Event) => {
      const customEvent = e as CustomEvent<ServerEvent>;
      if (customEvent.detail) {
        handleServerEvent(customEvent.detail);
      }
    };

    const handleMockError = () => {
      if (es?.onerror) {
        es.onerror(new Event("error"));
      }
    };

    if (typeof window !== "undefined") {
      window.addEventListener("mock-sse-event", handleMockEvent);
      window.addEventListener("mock-sse-error", handleMockError);
    }

    const connect = () => {
      try {
        const url = `${resolvedBaseUrl}/api/v1/events?token=${encodeURIComponent(token)}`;
        es = new EventSource(url);

        es.onopen = () => {
          setIsConnected(true);
          setError(null);
        };

        es.onmessage = (msg) => {
          try {
            const data: ServerEvent = JSON.parse(msg.data);
            handleServerEvent(data);
          } catch (error) {
            console.error("Failed to parse SSE event:", error);
          }
        };

        es.onerror = () => {
          setIsConnected(false);
          setError("Disconnected from agent-server stream. Reconnecting...");
          if (es) {
            es.close();
            es = null;
          }
          retryTimeout = setTimeout(connect, 3000);
        };
      } catch (error) {
        setError(String(error));
        retryTimeout = setTimeout(connect, 3000);
      }
    };

    connect();

    return () => {
      if (es) es.close();
      if (retryTimeout) clearTimeout(retryTimeout);
      if (typeof window !== "undefined") {
        window.removeEventListener("mock-sse-event", handleMockEvent);
        window.removeEventListener("mock-sse-error", handleMockError);
      }
    };
  }, [resolvedBaseUrl, token, handleServerEvent]);

  return { agents, environments, isConnected, error };
}
