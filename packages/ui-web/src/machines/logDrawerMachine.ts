import { setup, assign, fromPromise, fromCallback } from "xstate";
import type { ITelemetryFrame } from "../types";
import { LogBuffer } from "../utils/LogBuffer";
import { DEFAULT_DEV_TOKEN, resolveApiBaseUrl } from "../utils/config";
import type {
  ILogDrawerMachineContext,
  LogDrawerMachineEvent,
  ILogDrawerMachineInput,
  IFetchHistoryArgs,
  IStreamLiveTailArgs,
  StreamCleanupFn,
} from "./types";

function isTelemetryFrame(value: unknown): value is ITelemetryFrame {
  return typeof value === "object" && value !== null && "eventType" in value && "timestamp" in value;
}

function isTelemetryFrameArray(value: unknown): value is ITelemetryFrame[] {
  return Array.isArray(value);
}

export const logDrawerMachine = setup({
  types: {
    context: {} as ILogDrawerMachineContext,
    events: {} as LogDrawerMachineEvent,
    input: {} as ILogDrawerMachineInput | undefined,
  },
  actors: {
    fetchHistory: fromPromise(async ({ input, signal }: IFetchHistoryArgs): Promise<ITelemetryFrame[]> => {
      if (!input.agentId) {
        return [];
      }
      const resolvedBaseUrl = resolveApiBaseUrl(input.apiBaseUrl);
      const response = await fetch(
        `${resolvedBaseUrl}/api/v1/agents/${encodeURIComponent(input.agentId)}/logs?limit=100`,
        {
          headers: { Authorization: `Bearer ${input.token}` },
          signal,
        },
      );
      if (!response.ok) {
        throw new Error(`Failed to fetch logs: HTTP ${response.status}`);
      }
      const data: unknown = await response.json();
      if (isTelemetryFrameArray(data)) {
        return data;
      }
      return [];
    }),

    streamLiveTail: fromCallback(({ input, sendBack }: IStreamLiveTailArgs): StreamCleanupFn => {
      if (!input.agentId || typeof EventSource === "undefined") {
        return (): void => {};
      }
      const resolvedBaseUrl = resolveApiBaseUrl(input.apiBaseUrl);
      const es = new EventSource(
        `${resolvedBaseUrl}/api/v1/agents/${encodeURIComponent(input.agentId)}/live-tail?token=${encodeURIComponent(input.token)}`,
      );

      es.onopen = (): void => {
        sendBack({ type: "SSE_OPEN" });
      };

      es.onmessage = (msg: MessageEvent): void => {
        try {
          const parsed: unknown = JSON.parse(msg.data);
          if (isTelemetryFrame(parsed)) {
            input.logBuffer.prepend(parsed);
          }
        } catch (error) {
          console.error("Failed to parse log frame:", error);
        }
      };

      es.onerror = (): void => {
        sendBack({ type: "SSE_ERROR", error: "Live stream disconnected. Reconnecting..." });
      };

      return (): void => {
        es.close();
      };
    }),
  },
  actions: {
    setAgentAndConfig: assign(({ context, event }) => {
      if ("agent" in event) {
        const nextBaseUrl =
          "apiBaseUrl" in event && event.apiBaseUrl ? resolveApiBaseUrl(event.apiBaseUrl) : context.apiBaseUrl;
        const nextToken = "token" in event && event.token ? event.token : context.token;
        return {
          agent: event.agent,
          apiBaseUrl: nextBaseUrl,
          token: nextToken,
          errorMessage: null,
        };
      }
      return {};
    }),

    resetMachine: assign(({ context }) => {
      context.logBuffer.clear();
      return {
        agent: null,
        errorMessage: null,
      };
    }),

    clearLogs: ({ context }) => {
      context.logBuffer.clear();
    },

    clearError: assign({
      errorMessage: null,
    }),

    setError: assign({
      errorMessage: ({ event }) => {
        if ("error" in event && typeof event.error === "string") {
          return event.error;
        }
        return "Failed to load agent logs";
      },
    }),
  },
  guards: {
    hasAgent: ({ event }) => {
      return "agent" in event && event.agent !== null && event.agent !== undefined;
    },
    isAgentNull: ({ event }) => {
      return "agent" in event && (event.agent === null || event.agent === undefined);
    },
  },
}).createMachine({
  id: "logDrawerMachine",
  initial: "idle",
  context: ({ input }) => ({
    agent: input?.agent ?? null,
    apiBaseUrl: resolveApiBaseUrl(input?.apiBaseUrl),
    token: input?.token ?? DEFAULT_DEV_TOKEN,
    logBuffer: input?.logBuffer ?? new LogBuffer(),
    errorMessage: null,
  }),
  states: {
    idle: {
      on: {
        OPEN: {
          guard: "hasAgent",
          target: "fetchingHistory",
          actions: "setAgentAndConfig",
        },
        SELECT_AGENT: {
          guard: "hasAgent",
          target: "fetchingHistory",
          actions: "setAgentAndConfig",
        },
        CLOSE: {
          target: "idle",
          actions: "resetMachine",
        },
      },
    },

    fetchingHistory: {
      entry: "clearLogs",
      invoke: {
        src: "fetchHistory",
        input: ({ context }) => ({
          agentId: context.agent?.id ?? "",
          apiBaseUrl: context.apiBaseUrl,
          token: context.token,
        }),
        onDone: {
          target: "streamingLive",
          actions: [
            ({ context, event }) => {
              context.logBuffer.set(event.output);
            },
            "clearError",
          ],
        },
        onError: {
          target: "error",
          actions: assign({
            errorMessage: ({ event }) =>
              event.error instanceof Error ? event.error.message : "Failed to load agent logs",
          }),
        },
      },
      on: {
        CLOSE: {
          target: "idle",
          actions: "resetMachine",
        },
        SELECT_AGENT: [
          {
            guard: "isAgentNull",
            target: "idle",
            actions: "resetMachine",
          },
          {
            target: "fetchingHistory",
            reenter: true,
            actions: "setAgentAndConfig",
          },
        ],
        OPEN: {
          guard: "hasAgent",
          target: "fetchingHistory",
          reenter: true,
          actions: "setAgentAndConfig",
        },
      },
    },

    streamingLive: {
      invoke: {
        src: "streamLiveTail",
        input: ({ context }) => ({
          agentId: context.agent?.id ?? "",
          apiBaseUrl: context.apiBaseUrl,
          token: context.token,
          logBuffer: context.logBuffer,
        }),
      },
      on: {
        SSE_ERROR: {
          target: "reconnecting",
          actions: "setError",
        },
        SSE_OPEN: {
          actions: "clearError",
        },
        CLOSE: {
          target: "idle",
          actions: "resetMachine",
        },
        SELECT_AGENT: [
          {
            guard: "isAgentNull",
            target: "idle",
            actions: "resetMachine",
          },
          {
            target: "fetchingHistory",
            actions: "setAgentAndConfig",
          },
        ],
        OPEN: {
          guard: "hasAgent",
          target: "fetchingHistory",
          actions: "setAgentAndConfig",
        },
      },
    },

    reconnecting: {
      after: {
        3000: {
          target: "streamingLive",
        },
      },
      on: {
        RETRY: {
          target: "streamingLive",
        },
        CLOSE: {
          target: "idle",
          actions: "resetMachine",
        },
        SELECT_AGENT: [
          {
            guard: "isAgentNull",
            target: "idle",
            actions: "resetMachine",
          },
          {
            target: "fetchingHistory",
            actions: "setAgentAndConfig",
          },
        ],
        OPEN: {
          guard: "hasAgent",
          target: "fetchingHistory",
          actions: "setAgentAndConfig",
        },
      },
    },

    error: {
      on: {
        RETRY: {
          target: "fetchingHistory",
          actions: "clearError",
        },
        CLOSE: {
          target: "idle",
          actions: "resetMachine",
        },
        SELECT_AGENT: [
          {
            guard: "isAgentNull",
            target: "idle",
            actions: "resetMachine",
          },
          {
            target: "fetchingHistory",
            actions: "setAgentAndConfig",
          },
        ],
        OPEN: {
          guard: "hasAgent",
          target: "fetchingHistory",
          actions: "setAgentAndConfig",
        },
      },
    },
  },
});
