import { describe, it, expect, mock, beforeEach, afterEach } from "bun:test";
import React from "react";
import assert from "node:assert";
import { renderToString } from "react-dom/server";
import { LogDrawer } from "../components/LogDrawer";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion, fixture_telemetryFrame } from "./fixtures";
import type { ITelemetryFrame } from "../types";

type EventHandler = (event: MessageEvent) => void;
type ErrorHandler = (event: Event) => void;
type CleanupFn = () => void;
type FetchInput = RequestInfo | URL;

interface IGlobalScope {
  EventSource?: typeof EventSource;
}

class MockEventSource {
  public url: string;
  public onmessage: EventHandler | null = null;
  public onerror: ErrorHandler | null = null;
  public closed = false;

  constructor(url: string) {
    this.url = url;
  }

  public close(): void {
    this.closed = true;
  }
}

describe("LogDrawer Component", () => {
  const originalFetch = globalThis.fetch;
  const originalEventSource = globalThis.EventSource;

  beforeEach(() => {
    globalThis.fetch = mock(async () => {
      return new Response(JSON.stringify([fixture_telemetryFrame]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as unknown as typeof fetch;

    const globalScope: IGlobalScope = globalThis;
    globalScope.EventSource = MockEventSource as unknown as typeof EventSource;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    globalThis.EventSource = originalEventSource;
  });

  it("renders null when agent is null or undefined", () => {
    const rendered = renderToString(<LogDrawer agent={null} onClose={() => {}} />);
    expect(rendered).toBe("");
  });

  it("renders agent title and execution info when agent is provided", () => {
    const rendered = renderToString(<LogDrawer agent={fixture_agentStateRunning} onClose={() => {}} />);
    expect(rendered).toContain("LogDrawer");
    expect(rendered).toContain("Build agent-status dashboard");
    expect(rendered).toContain("work-macbook");
    expect(rendered).toContain("agent-status");
    expect(rendered).toContain("main");
    expect(rendered).toContain("No telemetry log events recorded yet.");
    expect(rendered).toContain('data-testid="LogDrawer--debug-button"');
    expect(rendered).toContain('aria-label="Copy debug command to clipboard"');
  });

  it("copies debug command to clipboard when debug button is clicked", async () => {
    let copiedText = "";
    const originalClipboard = navigator.clipboard;
    Object.assign(navigator, {
      clipboard: {
        writeText: mock(async (text: string) => {
          copiedText = text;
        }),
      },
    });

    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const { container, unmount } = await mountComponent(
        <LogDrawer agent={fixture_agentStateRunning} onClose={() => {}} />,
        mockDoc,
      );

      const debugBtn = container.querySelector('[data-testid="LogDrawer--debug-button"]');
      assert(debugBtn);
      debugBtn.click();

      expect(copiedText).toBe("just debug session replay --harness claude-code session-1");
      await unmount();
    } finally {
      cleanup();
      Object.assign(navigator, { clipboard: originalClipboard });
    }
  });

  it("renders fallback title when agent activity title is missing", () => {
    const agentWithoutTitle = {
      ...fixture_agentStateRunning,
      activity: {
        ...fixture_agentStateRunning.activity,
        title: "",
      },
    };
    const rendered = renderToString(<LogDrawer agent={agentWithoutTitle} onClose={() => {}} />);
    expect(rendered).toContain("Agent Execution Log");
  });

  it("verifies effect triggers log reset and fetches logs for new agent on switch", async () => {
    const fetchedUrls: string[] = [];
    const eventSourceUrls: string[] = [];

    globalThis.fetch = mock(async (input: FetchInput) => {
      fetchedUrls.push(String(input));
      return new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as unknown as typeof fetch;

    class TrackingEventSource {
      public url: string;
      public closed = false;
      public onmessage: EventHandler | null = null;

      constructor(url: string) {
        this.url = url;
        eventSourceUrls.push(url);
      }

      public close(): void {
        this.closed = true;
      }
    }

    const globalScope: IGlobalScope = globalThis;
    globalScope.EventSource = TrackingEventSource as unknown as typeof EventSource;

    let currentLogs: ITelemetryFrame[] = [fixture_telemetryFrame];

    const simulateEffect = (agentId: string): CleanupFn => {
      currentLogs = [];

      fetch(`http://localhost:4502/api/v1/agents/${encodeURIComponent(agentId)}/logs?limit=100`, {
        headers: { Authorization: "Bearer dev-secret-token" },
      })
        .then((res) => res.json())
        .then((data: ITelemetryFrame[]) => {
          currentLogs = data;
        })
        .catch(() => {});

      const es = new TrackingEventSource(
        `http://localhost:4502/api/v1/agents/${encodeURIComponent(agentId)}/live-tail?token=dev-secret-token`,
      );

      return () => {
        es.close();
      };
    };

    const cleanup1 = simulateEffect(fixture_agentStateRunning.id);
    expect(currentLogs.length).toBe(0);
    expect(fetchedUrls[0]).toContain(encodeURIComponent(fixture_agentStateRunning.id));
    expect(eventSourceUrls[0]).toContain(encodeURIComponent(fixture_agentStateRunning.id));

    await Promise.resolve();
    await Promise.resolve();
    cleanup1();

    currentLogs = [fixture_telemetryFrame, fixture_telemetryFrame];
    expect(currentLogs.length).toBe(2);

    const cleanup2 = simulateEffect(fixture_agentStateWaitingQuestion.id);
    expect(currentLogs.length).toBe(0);
    expect(fetchedUrls[1]).toContain(encodeURIComponent(fixture_agentStateWaitingQuestion.id));
    expect(eventSourceUrls[1]).toContain(encodeURIComponent(fixture_agentStateWaitingQuestion.id));
    cleanup2();
  });
});
