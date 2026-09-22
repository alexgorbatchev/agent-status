import { useEffect, useState, useCallback } from "react";
import type { IAgentState, IEnvironmentInfo, IEventStreamState, ServerEvent } from "../types";
import { calculateReconnectDelay } from "../utils/calculateReconnectDelay";
import { DEFAULT_DEV_TOKEN, resolveApiBaseUrl } from "../utils/config";

function isCustomEvent<T>(event: Event): event is CustomEvent<T> {
  return "detail" in event;
}

export function useEventStream(apiBaseUrl?: string, token: string = DEFAULT_DEV_TOKEN): IEventStreamState {
  const resolvedBaseUrl = resolveApiBaseUrl(apiBaseUrl);
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
            return [...prev, event.agent];
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

  const updateAgent = useCallback((updated: IAgentState) => {
    setAgents((prev) => {
      const idx = prev.findIndex((a) => a.id === updated.id);
      if (idx >= 0) {
        const copy = [...prev];
        copy[idx] = updated;
        return copy;
      }
      return [...prev, updated];
    });
  }, []);

  useEffect(() => {
    let es: EventSource | null = null;
    let retryTimeout: ReturnType<typeof setTimeout> | null = null;
    let retryAttempt = 0;

    const handleMockEvent = (e: Event) => {
      if (isCustomEvent<ServerEvent>(e) && e.detail) {
        handleServerEvent(e.detail);
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

    const scheduleReconnect = () => {
      const delay = calculateReconnectDelay(retryAttempt);
      retryAttempt++;
      retryTimeout = setTimeout(connect, delay);
    };

    const connect = () => {
      try {
        const url = `${resolvedBaseUrl}/api/v1/events?token=${encodeURIComponent(token)}`;
        es = new EventSource(url);

        es.onopen = () => {
          setIsConnected(true);
          setError(null);
          retryAttempt = 0;
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
          scheduleReconnect();
        };
      } catch (error) {
        setError(String(error));
        scheduleReconnect();
      }
    };

    connect();

    return () => {
      if (typeof window !== "undefined") {
        window.removeEventListener("mock-sse-event", handleMockEvent);
        window.removeEventListener("mock-sse-error", handleMockError);
      }
      if (es) es.close();
      if (retryTimeout) clearTimeout(retryTimeout);
    };
  }, [resolvedBaseUrl, token, handleServerEvent]);

  return { agents, environments, isConnected, error, updateAgent };
}
