import { describe, it, expect, mock, beforeEach, afterEach } from "bun:test";
import React from "react";
import assert from "node:assert";
import { renderToString } from "react-dom/server";
import { LogDrawer } from "../components/LogDrawer";
import { LogFrameItem } from "../components/LogFrameItem";
import { LogBuffer } from "../utils/LogBuffer";
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

  it("renders drawer with max-w-3xl width constraint and overflow containment on stream and header", () => {
    const rendered = renderToString(<LogDrawer agent={fixture_agentStateRunning} onClose={() => {}} />);
    expect(rendered).toContain("max-w-3xl");
    expect(rendered).not.toContain("max-w-2xl");

    // Stream container must constrain horizontal overflow
    expect(rendered).toContain('data-testid="LogDrawer--stream"');
    expect(rendered).toMatch(/data-testid="LogDrawer--stream"[^>]*class="[^"]*overflow-x-hidden[^"]*"/);
    expect(rendered).toMatch(/data-testid="LogDrawer--stream"[^>]*class="[^"]*min-w-0[^"]*"/);
    expect(rendered).toMatch(/data-testid="LogDrawer--stream"[^>]*class="[^"]*w-full[^"]*"/);

    // Header metadata must truncate long text
    expect(rendered).toMatch(/<p[^>]*class="[^"]*truncate[^"]*"[^>]*>.*main.*<\/p>/);
  });

  it("renders 500-character unbroken strings in log stream with overflow containment classes", () => {
    const longToken = "y".repeat(500);
    const longFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      data: {
        ...fixture_telemetryFrame.data,
        content: `Long text: ${longToken}\n\n\`\`\`sh\n${longToken}\n\`\`\`\n\n| H1 | H2 |\n|---|---|\n| ${longToken} | test |`,
        toolName: "Bash",
        toolInput: {
          command: `echo ${longToken}`,
        },
        toolOutput: longToken,
      },
    };

    const renderedDrawer = renderToString(<LogDrawer agent={fixture_agentStateRunning} onClose={() => {}} />);
    expect(renderedDrawer).toContain("max-w-3xl");
    expect(renderedDrawer).toContain("overflow-x-hidden min-w-0 w-full");

    const renderedItem = renderToString(<LogFrameItem frame={longFrame} />);
    expect(renderedItem).toContain("min-w-0 max-w-full overflow-hidden");
    expect(renderedItem).toContain("[overflow-wrap:anywhere] break-all");
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

  it("renders correlated tool execution with unified command and output in a single frame", () => {
    const callFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      seq: 1,
      timestamp: 1000,
      eventType: "tool_call",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call-xyz",
        toolName: "bash",
        toolInput: { command: "bun test --coverage" },
        toolOutput: null,
      },
    };
    const resultFrame: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      seq: 2,
      timestamp: 1850,
      eventType: "tool_result",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call-xyz",
        toolOutput: "All tests passed with 100% coverage",
        exitCode: 0,
        isError: false,
      },
    };

    const buffer = new LogBuffer([callFrame, resultFrame]);
    const html = renderToString(<LogDrawer agent={fixture_agentStateRunning} logBuffer={buffer} onClose={() => {}} />);

    const frameItemMatches = html.match(/data-testid="LogFrameItem"/g);
    expect(frameItemMatches).not.toBeNull();
    expect(frameItemMatches?.length).toBe(1);

    expect(html).toContain("bun test --coverage");
    expect(html).toContain("All tests passed with 100% coverage");
    expect(html).toContain("exit 0");
    expect(html).toContain("850ms");
  });

  it("renders orphaned tool_result when no matching tool_call exists", () => {
    const orphanResult: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      seq: 1,
      timestamp: 1000,
      eventType: "tool_result",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call-orphan",
        toolOutput: "Orphaned result content",
        exitCode: 1,
        isError: true,
      },
    };

    const buffer = new LogBuffer([orphanResult]);
    const html = renderToString(<LogDrawer agent={fixture_agentStateRunning} logBuffer={buffer} onClose={() => {}} />);

    const frameItemMatches = html.match(/data-testid="LogFrameItem"/g);
    expect(frameItemMatches?.length).toBe(1);
    expect(html).toContain("Orphaned result content");
    expect(html).toContain("exit 1");
  });

  it("renders concurrent tool calls correlated to their respective results", () => {
    const callA: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      seq: 1,
      timestamp: 1000,
      eventType: "tool_call",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call-A",
        toolName: "bash",
        toolInput: { command: "echo testA" },
      },
    };
    const callB: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      seq: 2,
      timestamp: 1050,
      eventType: "tool_call",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call-B",
        toolName: "bash",
        toolInput: { command: "echo testB" },
      },
    };
    const resultA: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      seq: 3,
      timestamp: 1300,
      eventType: "tool_result",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call-A",
        toolOutput: "output A",
        exitCode: 0,
        isError: false,
      },
    };
    const resultB: ITelemetryFrame = {
      ...fixture_telemetryFrame,
      seq: 4,
      timestamp: 1400,
      eventType: "tool_result",
      data: {
        ...fixture_telemetryFrame.data,
        toolCallId: "call-B",
        toolOutput: "output B",
        exitCode: 0,
        isError: false,
      },
    };

    const buffer = new LogBuffer([callA, callB, resultA, resultB]);
    const html = renderToString(<LogDrawer agent={fixture_agentStateRunning} logBuffer={buffer} onClose={() => {}} />);

    const frameItemMatches = html.match(/data-testid="LogFrameItem"/g);
    expect(frameItemMatches?.length).toBe(2);

    expect(html).toContain("echo testA");
    expect(html).toContain("output A");
    expect(html).toContain("echo testB");
    expect(html).toContain("output B");
  });
});
