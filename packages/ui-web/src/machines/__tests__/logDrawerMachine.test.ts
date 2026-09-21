import { describe, it, expect, mock } from "bun:test";
import assert from "node:assert";
import { createActor, fromPromise, fromCallback } from "xstate";
import { logDrawerMachine } from "../logDrawerMachine";
import { LogBuffer } from "../../utils/LogBuffer";
import type { ITelemetryFrame, IWindowWithAgentStatus } from "../../types";
import type { IFetchHistoryArgs, StreamCleanupFn } from "../types";
import {
  fixture_machineAgent1,
  fixture_machineAgent2,
  fixture_machineTelemetryFrame1,
  fixture_machineTelemetryFrame2,
} from "./fixtures";

type EventHandler = (event: MessageEvent) => void;
type ErrorHandler = (event: Event) => void;
type VoidCallback = () => void;
type FetchInput = RequestInfo | URL;

interface IGlobalScope {
  EventSource?: typeof EventSource;
}

interface IGlobalWindowScope {
  window?: IWindowWithAgentStatus;
}

const flushTicks = async (): Promise<void> => {
  await new Promise<void>((resolve) => setTimeout(resolve, 15));
};

let capturedEventSource: MockEventSource | null = null;

class MockEventSource {
  public url: string;
  public onmessage: EventHandler | null = null;
  public onerror: ErrorHandler | null = null;
  public onopen: VoidCallback | null = null;
  public closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.registerInstance(this);
  }

  public static registerInstance(instance: MockEventSource): void {
    capturedEventSource = instance;
  }

  public close(): void {
    this.closed = true;
  }
}

describe("logDrawerMachine State Machine", () => {
  it("initializes in idle state with empty context when input is omitted", () => {
    const actor = createActor(logDrawerMachine, { input: undefined });
    actor.start();

    expect(actor.getSnapshot().value).toBe("idle");
    expect(actor.getSnapshot().context.agent).toBeNull();
    expect(actor.getSnapshot().context.errorMessage).toBeNull();
    expect(actor.getSnapshot().context.logBuffer.count).toBe(0);
    actor.stop();
  });

  it("transitions from idle to fetchingHistory on OPEN event with valid agent", () => {
    const actor = createActor(logDrawerMachine, { input: undefined });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });

    expect(actor.getSnapshot().value).toBe("fetchingHistory");
    expect(actor.getSnapshot().context.agent).toEqual(fixture_machineAgent1);
    actor.stop();
  });

  it("transitions from idle to fetchingHistory on SELECT_AGENT event with valid agent", () => {
    const actor = createActor(logDrawerMachine, { input: undefined });
    actor.start();

    actor.send({ type: "SELECT_AGENT", agent: fixture_machineAgent1 });

    expect(actor.getSnapshot().value).toBe("fetchingHistory");
    expect(actor.getSnapshot().context.agent).toEqual(fixture_machineAgent1);
    actor.stop();
  });

  it("populates LogBuffer and transitions to streamingLive on fetchHistory success", async () => {
    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          return [fixture_machineTelemetryFrame1];
        }),
        streamLiveTail: fromCallback((): StreamCleanupFn => {
          return (): void => {};
        }),
      },
    });

    const buffer = new LogBuffer();
    const actor = createActor(mockMachine, {
      input: { logBuffer: buffer },
    });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    expect(actor.getSnapshot().value).toBe("fetchingHistory");

    await flushTicks();

    expect(actor.getSnapshot().value).toBe("streamingLive");
    expect(buffer.count).toBe(1);
    expect(buffer.getSnapshot()[0]).toEqual(fixture_machineTelemetryFrame1);
    actor.stop();
  });

  it("sets errorMessage and transitions to error when fetchHistory fails", async () => {
    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          return Promise.reject(new Error("HTTP 500: Internal Server Error"));
        }),
        streamLiveTail: fromCallback((): StreamCleanupFn => {
          return (): void => {};
        }),
      },
    });

    const actor = createActor(mockMachine, { input: undefined });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });

    await flushTicks();

    expect(actor.getSnapshot().value).toBe("error");
    expect(actor.getSnapshot().context.errorMessage).toBe("HTTP 500: Internal Server Error");
    actor.stop();
  });

  it("transitions from error to fetchingHistory on RETRY event and clears errorMessage", async () => {
    const fetchPromises: Promise<ITelemetryFrame[]>[] = [
      Promise.reject(new Error("First attempt failed")),
      Promise.resolve([fixture_machineTelemetryFrame1]),
    ];

    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          const next = fetchPromises.shift();
          return next ?? Promise.resolve([]);
        }),
        streamLiveTail: fromCallback((): StreamCleanupFn => {
          return (): void => {};
        }),
      },
    });

    const actor = createActor(mockMachine, { input: undefined });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    await flushTicks();

    expect(actor.getSnapshot().value).toBe("error");

    actor.send({ type: "RETRY" });
    expect(actor.getSnapshot().value).toBe("fetchingHistory");
    expect(actor.getSnapshot().context.errorMessage).toBeNull();

    await flushTicks();

    expect(actor.getSnapshot().value).toBe("streamingLive");
    actor.stop();
  });

  it("transitions to reconnecting when SSE_ERROR occurs during streamingLive", async () => {
    let sendSSEError: VoidCallback = () => {};
    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          return [];
        }),
        streamLiveTail: fromCallback(({ sendBack }): StreamCleanupFn => {
          sendSSEError = (): void => {
            sendBack({ type: "SSE_ERROR", error: "Stream interrupted" });
          };
          return (): void => {};
        }),
      },
    });

    const actor = createActor(mockMachine, { input: undefined });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    await flushTicks();

    expect(actor.getSnapshot().value).toBe("streamingLive");

    sendSSEError();
    expect(actor.getSnapshot().value).toBe("reconnecting");
    expect(actor.getSnapshot().context.errorMessage).toBe("Stream interrupted");
    actor.stop();
  });

  it("recovers from reconnecting to streamingLive on RETRY event", async () => {
    let sendSSEError: VoidCallback = () => {};
    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          return [];
        }),
        streamLiveTail: fromCallback(({ sendBack }): StreamCleanupFn => {
          sendSSEError = (): void => {
            sendBack({ type: "SSE_ERROR", error: "Stream error" });
          };
          return (): void => {};
        }),
      },
    });

    const actor = createActor(mockMachine, { input: undefined });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    await flushTicks();

    expect(actor.getSnapshot().value).toBe("streamingLive");

    sendSSEError();
    expect(actor.getSnapshot().value).toBe("reconnecting");

    actor.send({ type: "RETRY" });
    expect(actor.getSnapshot().value).toBe("streamingLive");
    actor.stop();
  });

  it("aborts in-flight fetchHistory and restarts when SELECT_AGENT is dispatched with new agent", async () => {
    let abortedCount = 0;
    let fetchStartCount = 0;

    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async ({ signal }: IFetchHistoryArgs): Promise<ITelemetryFrame[]> => {
          fetchStartCount++;
          return new Promise<ITelemetryFrame[]>((resolve, reject) => {
            const timer = setTimeout(() => {
              resolve([fixture_machineTelemetryFrame1]);
            }, 100);

            signal.addEventListener("abort", () => {
              clearTimeout(timer);
              abortedCount++;
              reject(new DOMException("Aborted", "AbortError"));
            });
          });
        }),
        streamLiveTail: fromCallback((): StreamCleanupFn => {
          return (): void => {};
        }),
      },
    });

    const buffer = new LogBuffer();
    const actor = createActor(mockMachine, {
      input: { logBuffer: buffer },
    });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    expect(fetchStartCount).toBe(1);
    expect(abortedCount).toBe(0);

    // Switch agent while fetch 1 is inflight
    actor.send({ type: "SELECT_AGENT", agent: fixture_machineAgent2 });
    expect(fetchStartCount).toBe(2);
    expect(abortedCount).toBe(1);
    expect(actor.getSnapshot().context.agent).toEqual(fixture_machineAgent2);

    actor.stop();
  });

  it("aborts in-flight fetchHistory and transitions to idle on CLOSE event", async () => {
    let abortedCount = 0;

    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async ({ signal }: IFetchHistoryArgs): Promise<ITelemetryFrame[]> => {
          return new Promise<ITelemetryFrame[]>((_resolve, reject) => {
            signal.addEventListener("abort", () => {
              abortedCount++;
              reject(new DOMException("Aborted", "AbortError"));
            });
          });
        }),
        streamLiveTail: fromCallback((): StreamCleanupFn => {
          return (): void => {};
        }),
      },
    });

    const buffer = new LogBuffer();
    buffer.set([fixture_machineTelemetryFrame1]);

    const actor = createActor(mockMachine, {
      input: { logBuffer: buffer },
    });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    expect(actor.getSnapshot().value).toBe("fetchingHistory");

    actor.send({ type: "CLOSE" });
    expect(actor.getSnapshot().value).toBe("idle");
    expect(abortedCount).toBe(1);
    expect(actor.getSnapshot().context.agent).toBeNull();
    expect(buffer.count).toBe(0);

    actor.stop();
  });

  it("disconnects SSE stream and cleans up on machine exit", async () => {
    let sseCleanedUp = false;

    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          return [];
        }),
        streamLiveTail: fromCallback((): StreamCleanupFn => {
          return (): void => {
            sseCleanedUp = true;
          };
        }),
      },
    });

    const actor = createActor(mockMachine, { input: undefined });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    await flushTicks();

    expect(actor.getSnapshot().value).toBe("streamingLive");
    expect(sseCleanedUp).toBe(false);

    actor.send({ type: "CLOSE" });
    expect(actor.getSnapshot().value).toBe("idle");
    expect(sseCleanedUp).toBe(true);

    actor.stop();
  });

  it("cleans up and transitions to idle when SELECT_AGENT with null is dispatched", async () => {
    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          return [fixture_machineTelemetryFrame1];
        }),
        streamLiveTail: fromCallback((): StreamCleanupFn => {
          return (): void => {};
        }),
      },
    });

    const buffer = new LogBuffer();
    const actor = createActor(mockMachine, {
      input: { logBuffer: buffer },
    });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    await flushTicks();

    expect(actor.getSnapshot().value).toBe("streamingLive");

    actor.send({ type: "SELECT_AGENT", agent: null });
    expect(actor.getSnapshot().value).toBe("idle");
    expect(actor.getSnapshot().context.agent).toBeNull();
    expect(buffer.count).toBe(0);

    actor.stop();
  });

  it("executes default fetchHistory actor against real fetch and parses frames", async () => {
    const originalFetch = globalThis.fetch;
    const fetchedUrls: string[] = [];

    globalThis.fetch = mock(async (input: FetchInput) => {
      fetchedUrls.push(String(input));
      return new Response(JSON.stringify([fixture_machineTelemetryFrame1, fixture_machineTelemetryFrame2]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as unknown as typeof fetch;

    const buffer = new LogBuffer();
    const actor = createActor(logDrawerMachine, {
      input: {
        apiBaseUrl: "http://localhost:4502",
        token: "test-token",
        logBuffer: buffer,
      },
    });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    expect(actor.getSnapshot().value).toBe("fetchingHistory");

    await flushTicks();

    expect(fetchedUrls.length).toBe(1);
    expect(fetchedUrls[0]).toContain("/api/v1/agents/env-1%3Aclaude-code%3Asession-1/logs?limit=100");
    expect(buffer.count).toBe(2);

    actor.stop();
    globalThis.fetch = originalFetch;
  });

  it("executes default streamLiveTail actor against real EventSource and prepends frames", async () => {
    const globalScope: IGlobalScope = globalThis;
    const originalEventSource = globalScope.EventSource;

    capturedEventSource = null;
    globalScope.EventSource = MockEventSource as unknown as typeof EventSource;

    const mockMachine = logDrawerMachine.provide({
      actors: {
        fetchHistory: fromPromise(async (): Promise<ITelemetryFrame[]> => {
          return [fixture_machineTelemetryFrame1];
        }),
      },
    });

    const buffer = new LogBuffer();
    const actor = createActor(mockMachine, {
      input: {
        apiBaseUrl: "http://localhost:4502",
        token: "live-token",
        logBuffer: buffer,
      },
    });
    actor.start();

    actor.send({ type: "OPEN", agent: fixture_machineAgent1 });
    await flushTicks();

    expect(actor.getSnapshot().value).toBe("streamingLive");
    assert(capturedEventSource !== null);
    const esInstance: MockEventSource = capturedEventSource;
    expect(esInstance.url).toContain("/api/v1/agents/env-1%3Aclaude-code%3Asession-1/live-tail?token=live-token");

    // Simulate SSE live message arrival
    esInstance.onmessage?.({
      data: JSON.stringify(fixture_machineTelemetryFrame2),
    } as MessageEvent);

    expect(buffer.count).toBe(2);
    expect(buffer.getSnapshot()[0]).toEqual(fixture_machineTelemetryFrame2);

    // Simulate stream close
    actor.send({ type: "CLOSE" });
    expect(esInstance.closed).toBe(true);

    actor.stop();
    globalScope.EventSource = originalEventSource;
  });

  it("resolves apiBaseUrl from window.__AGENT_STATUS_API_URL__ when input apiBaseUrl is omitted", () => {
    const globalWindow = globalThis as unknown as IGlobalWindowScope;
    const prevWindow = globalWindow.window;
    globalWindow.window = { __AGENT_STATUS_API_URL__: "http://injected-server:5000" };

    try {
      const actor = createActor(logDrawerMachine, {
        input: {
          token: "my-token",
        },
      });
      actor.start();

      expect(actor.getSnapshot().context.apiBaseUrl).toBe("http://injected-server:5000");
      actor.stop();
    } finally {
      globalWindow.window = prevWindow;
    }
  });
});
