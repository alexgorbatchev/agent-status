import { useEffect, useState, useCallback } from "react";
import type { IAgentState, IEnvironmentInfo, IServerEvent } from "../types";

interface EventStreamState {
  agents: IAgentState[];
  environments: IEnvironmentInfo[];
  isConnected: boolean;
  error: string | null;
}

export function useEventStream(apiBaseUrl: string = "http://localhost:4502", token: string = "dev-secret-token"): EventStreamState {
  const [agents, setAgents] = useState<IAgentState[]>([]);
  const [environments, setEnvironments] = useState<IEnvironmentInfo[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleServerEvent = useCallback((event: IServerEvent) => {
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
              updated[idx] = event.agent!;
              return updated;
            }
            return [event.agent!, ...prev];
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
              updated[idx] = event.environment!;
              return updated;
            }
            return [...prev, event.environment!];
          });
        }
        break;
    }
  }, []);

  useEffect(() => {
    let es: EventSource | null = null;
    let retryTimeout: Timer | null = null;

    const connect = () => {
      try {
        const url = `${apiBaseUrl}/api/v1/events?token=${encodeURIComponent(token)}`;
        es = new EventSource(url);

        es.onopen = () => {
          setIsConnected(true);
          setError(null);
        };

        es.onmessage = (msg) => {
          try {
            const data: IServerEvent = JSON.parse(msg.data);
            handleServerEvent(data);
          } catch (err) {
            console.error("Failed to parse SSE event:", err);
          }
        };

        es.onerror = (err) => {
          setIsConnected(false);
          setError("Disconnected from agent-server stream. Reconnecting...");
          if (es) {
            es.close();
            es = null;
          }
          retryTimeout = setTimeout(connect, 3000);
        };
      } catch (err) {
        setError(String(err));
        retryTimeout = setTimeout(connect, 3000);
      }
    };

    connect();

    return () => {
      if (es) es.close();
      if (retryTimeout) clearTimeout(retryTimeout);
    };
  }, [apiBaseUrl, token, handleServerEvent]);

  return { agents, environments, isConnected, error };
}
