import { describe, it, expect, beforeEach, afterEach } from "bun:test";
import assert from "node:assert";
import React, { act } from "react";
import { App } from "../layouts/App";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion, fixture_agentStateExited } from "./fixtures";
import type { IAgentState, ServerEvent } from "../types";

type CleanupFn = () => void;
type FetchTarget = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
type EventSourceVoidFn = () => void;

class MockEventSource {
  static instances: MockEventSource[] = [];
  public url: string;
  public onopen: EventSourceVoidFn | null = null;
  public onerror: EventSourceVoidFn | null = null;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  close(): void {}
}

interface IGlobalEventSourceScope {
  EventSource?: typeof MockEventSource | typeof EventSource;
}

describe("App Layout & Session Visibility", () => {
  let envCleanup: CleanupFn | null = null;
  let originalFetch: typeof globalThis.fetch;
  let originalEventSource: typeof EventSource | undefined;
  const globalScope = globalThis as unknown as IGlobalEventSourceScope;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
    originalEventSource = globalThis.EventSource;
    MockEventSource.instances = [];
    globalScope.EventSource = MockEventSource;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    globalScope.EventSource = originalEventSource;
    MockEventSource.instances = [];
    envCleanup?.();
    envCleanup = null;
  });

  it("filters out hidden agents by default and displays hidden count toggle", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const visibleAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-visible",
      isHidden: false,
    };
    const hiddenAgent: IAgentState = {
      ...fixture_agentStateWaitingQuestion,
      id: "agent-hidden",
      isHidden: true,
      hiddenAt: Date.now(),
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    // Send initial snapshot via mock-sse-event inside act
    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [visibleAgent, hiddenAgent],
          environments: [visibleAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // Agent count text should reflect 1 visible, 1 hidden
    const agentCountEl = container.querySelector('[data-testid="App--agent-count"]');
    assert(agentCountEl, "agent count element must exist");
    expect(agentCountEl.textContent).toContain("1 Agent (1 hidden)");

    // Toggle button should exist and have cursor-pointer and matching ViewSwitcher styling
    const toggleBtn = container.querySelector('[data-testid="App--toggle-show-hidden"]');
    assert(toggleBtn, "toggle show hidden button must exist when hidden agents exist");
    expect(toggleBtn.textContent).toContain("Show 1 hidden");
    expect(toggleBtn.getAttribute("class")).toContain("cursor-pointer");
    expect(toggleBtn.getAttribute("class")).toContain("px-3");
    expect(toggleBtn.getAttribute("class")).toContain("py-1.5");
    expect(toggleBtn.getAttribute("class")).toContain("font-semibold");

    // Click toggle button to reveal hidden agents inside act
    await act(async () => {
      toggleBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // Now toggle button should say Hide 1 hidden
    expect(toggleBtn.textContent).toContain("Hide 1 hidden");
    expect(agentCountEl.textContent).toBe("2 Agents");

    await unmount();
  });

  it("filters out finished sessions by default and displays toggle button next to hidden toggle", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const runningAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-running-1",
      isHidden: false,
    };
    const finishedAgent: IAgentState = {
      ...fixture_agentStateExited,
      id: "agent-finished-1",
      isHidden: false,
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    // Send initial snapshot via mock-sse-event inside act
    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [runningAgent, finishedAgent],
          environments: [runningAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // Agent count text should reflect 1 visible agent
    const agentCountEl = container.querySelector('[data-testid="App--agent-count"]');
    assert(agentCountEl, "agent count element must exist");
    expect(agentCountEl.textContent).toBe("1 Agent");

    // Finished agent should be hidden by default: only 1 AgentCard rendered
    const agentCardsInitial = container.querySelectorAll('[data-testid="AgentCard"]');
    expect(agentCardsInitial.length).toBe(1);

    // Toggle finished button should exist with "Show 1 finished"
    const toggleFinishedBtn = container.querySelector('[data-testid="App--toggle-show-finished"]');
    assert(toggleFinishedBtn, "toggle show finished button must exist when finished agents exist");
    expect(toggleFinishedBtn.textContent).toContain("Show 1 finished");
    expect(toggleFinishedBtn.getAttribute("class")).toContain("cursor-pointer");
    expect(toggleFinishedBtn.getAttribute("class")).toContain("px-3");
    expect(toggleFinishedBtn.getAttribute("class")).toContain("py-1.5");
    expect(toggleFinishedBtn.getAttribute("class")).toContain("font-semibold");

    // Click toggle button to reveal finished sessions inside act
    await act(async () => {
      toggleFinishedBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // Now toggle button should say Hide 1 finished and both cards should be visible
    expect(toggleFinishedBtn.textContent).toContain("Hide 1 finished");
    expect(agentCountEl.textContent).toBe("2 Agents");
    const agentCardsVisible = container.querySelectorAll('[data-testid="AgentCard"]');
    expect(agentCardsVisible.length).toBe(2);

    // Click toggle button again to hide finished sessions
    await act(async () => {
      toggleFinishedBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(toggleFinishedBtn.textContent).toContain("Show 1 finished");
    expect(agentCountEl.textContent).toBe("1 Agent");
    const agentCardsHiddenAgain = container.querySelectorAll('[data-testid="AgentCard"]');
    expect(agentCardsHiddenAgain.length).toBe(1);

    await unmount();
  });

  it("does not render App--toggle-show-finished when no finished agents exist", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const runningAgent1: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-running-1",
      isHidden: false,
    };
    const runningAgent2: IAgentState = {
      ...fixture_agentStateWaitingQuestion,
      id: "agent-running-2",
      isHidden: false,
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [runningAgent1, runningAgent2],
          environments: [runningAgent1.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const toggleFinishedBtn = container.querySelector('[data-testid="App--toggle-show-finished"]');
    expect(toggleFinishedBtn).toBeNull();

    await unmount();
  });

  it("renders both toggle-show-hidden and toggle-show-finished side by side when both exist", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const runningAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-running-1",
      isHidden: false,
    };
    const hiddenAgent: IAgentState = {
      ...fixture_agentStateWaitingQuestion,
      id: "agent-hidden-1",
      isHidden: true,
      hiddenAt: Date.now(),
    };
    const finishedAgent: IAgentState = {
      ...fixture_agentStateExited,
      id: "agent-finished-1",
      isHidden: false,
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [runningAgent, hiddenAgent, finishedAgent],
          environments: [runningAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const toggleHiddenBtn = container.querySelector('[data-testid="App--toggle-show-hidden"]');
    const toggleFinishedBtn = container.querySelector('[data-testid="App--toggle-show-finished"]');
    assert(toggleHiddenBtn, "toggle show hidden button must exist");
    assert(toggleFinishedBtn, "toggle show finished button must exist");

    expect(toggleHiddenBtn.textContent).toContain("Show 1 hidden");
    expect(toggleFinishedBtn.textContent).toContain("Show 1 finished");

    // Only running agent visible by default
    const agentCards = container.querySelectorAll('[data-testid="AgentCard"]');
    expect(agentCards.length).toBe(1);

    await unmount();
  });

  it("calls PATCH endpoint when toggling agent visibility", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    let patchCalledWithUrl = "";
    let patchCalledWithBody = "";

    globalThis.fetch = (async (input: FetchTarget, init?: FetchInit): Promise<Response> => {
      patchCalledWithUrl = input.toString();
      patchCalledWithBody = init?.body?.toString() || "";
      return new Response(JSON.stringify({ isHidden: true }), { status: 200 });
    }) as unknown as typeof fetch;

    const testAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-toggle-test",
      isHidden: false,
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [testAgent],
          environments: [testAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // Find hide button on agent card
    const hideBtn = container.querySelector('[data-testid="AgentCard--hide-button"]');
    assert(hideBtn, "hide button must exist on visible agent card");

    await act(async () => {
      hideBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(patchCalledWithUrl).toContain("/api/v1/agents/agent-toggle-test");
    expect(patchCalledWithBody).toBe(JSON.stringify({ isHidden: true }));

    await unmount();
  });

  it("reverts optimistic update when patch request fails", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    // Simulate server error response
    globalThis.fetch = (async (): Promise<Response> => {
      return new Response("Internal Server Error", { status: 500, statusText: "Internal Server Error" });
    }) as unknown as typeof fetch;

    const testAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-rollback-test",
      isHidden: false,
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [testAgent],
          environments: [testAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const hideBtn = container.querySelector('[data-testid="AgentCard--hide-button"]');
    assert(hideBtn);

    await act(async () => {
      hideBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 30));
    });

    // Since the network failed, the override was reverted and agent remains visible
    const card = container.querySelector('[data-testid="AgentCard"]');
    assert(card, "agent card must remain visible after network failure rollback");

    await unmount();
  });

  it("displays agent immediately when incoming stream auto-unhides agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    globalThis.fetch = (async (): Promise<Response> => {
      return new Response(JSON.stringify({ isHidden: true }), { status: 200 });
    }) as unknown as typeof fetch;

    const testAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-auto-unhide-test",
      isHidden: false,
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    // Initial snapshot
    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [testAgent],
          environments: [testAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // Hide the agent
    const hideBtn = container.querySelector('[data-testid="AgentCard--hide-button"]');
    assert(hideBtn);

    await act(async () => {
      hideBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // Now hidden, agent is filtered out (0 visible, 1 hidden)
    const agentCountEl = container.querySelector('[data-testid="App--agent-count"]');
    assert(agentCountEl);
    expect(agentCountEl.textContent).toContain("0 Agents (1 hidden)");

    // Backend auto-unhides the agent due to new activity and emits agent_upsert
    await act(async () => {
      const upsertEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "agent_upsert",
          at: Date.now(),
          agent: {
            ...testAgent,
            isHidden: false,
            hiddenAt: null,
          },
        },
      });
      window.dispatchEvent(upsertEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    // hiddenOverrides should be pruned, and agent should immediately be visible again!
    expect(agentCountEl.textContent).toBe("1 Agent");
    const visibleCard = container.querySelector('[data-testid="AgentCard"]');
    assert(visibleCard, "agent must reappear automatically on real-time auto-unhide event");

    await unmount();
  });

  it("renders error banner with red background and white text when stream disconnected", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    const esInstance = MockEventSource.instances[0];
    assert(esInstance, "EventSource instance must be initialized");

    // Initially connected or no error
    expect(container.querySelector('[data-testid="App--error"]')).toBeNull();

    // Trigger stream error inside act
    await act(async () => {
      esInstance.onerror?.();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const errorEl = container.querySelector('[data-testid="App--error"]');
    assert(errorEl, "error banner must be displayed when stream is disconnected");
    expect(errorEl.textContent).toContain("Disconnected from agent-server stream. Reconnecting...");
    expect(errorEl.getAttribute("class")).toContain("bg-red-600");
    expect(errorEl.getAttribute("class")).toContain("text-white");
    await unmount();
  });

  it("toggles active agents filter when clicking Header--active-agents button", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const activeAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-active-1",
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
      },
    };
    const idleAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-idle-1",
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        activeTool: null,
      },
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [activeAgent, idleAgent],
          environments: [activeAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const agentCountEl = container.querySelector('[data-testid="App--agent-count"]');
    assert(agentCountEl);
    expect(agentCountEl.textContent).toBe("2 Agents");

    // Both cards rendered
    expect(container.querySelectorAll('[data-testid="AgentCard"]').length).toBe(2);

    // Header active agents button
    const activeBtn = container.querySelector('[data-testid="Header--active-agents"]');
    assert(activeBtn, "Header--active-agents button must exist");

    // Click active filter button inside act
    await act(async () => {
      activeBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(String(activeBtn.getAttribute("aria-pressed"))).toBe("true");
    expect(agentCountEl.textContent).toBe("1 Active of 2 Agents");
    expect(container.querySelectorAll('[data-testid="AgentCard"]').length).toBe(1);

    // Toggle off
    await act(async () => {
      activeBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(String(activeBtn.getAttribute("aria-pressed"))).toBe("false");
    expect(agentCountEl.textContent).toBe("2 Agents");
    expect(container.querySelectorAll('[data-testid="AgentCard"]').length).toBe(2);

    await unmount();
  });

  it("displays App--no-active-empty-state when active filter is enabled and no active agents match", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const idleAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-idle-only",
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        activeTool: null,
      },
    };

    const { container, unmount } = await mountComponent(<App />, mockDoc);

    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [idleAgent],
          environments: [idleAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const activeBtn = container.querySelector('[data-testid="Header--active-agents"]');
    assert(activeBtn);

    // Click active filter
    await act(async () => {
      activeBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const noActiveEmptyState = container.querySelector('[data-testid="App--no-active-empty-state"]');
    assert(noActiveEmptyState, "App--no-active-empty-state must be rendered when active filter yields 0 agents");
    expect(noActiveEmptyState.textContent).toContain("Nothing to see here");
    expect(noActiveEmptyState.textContent).toContain("No agents are currently active in your fleet.");

    // Reset button
    const resetBtn = container.querySelector('[data-testid="App--reset-active-filter"]');
    assert(resetBtn, "App--reset-active-filter must exist inside empty state");
    expect(resetBtn.textContent).toContain("Show all agents");

    // Click reset button
    await act(async () => {
      resetBtn.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    expect(container.querySelector('[data-testid="App--no-active-empty-state"]')).toBeNull();
    expect(container.querySelectorAll('[data-testid="AgentCard"]').length).toBe(1);

    await unmount();
  });

  it("calls dismissCompletion API when completed agent card is acknowledged in App layout", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    interface IFetchCall {
      url: string;
      method: string;
      body: string;
    }
    const calls: IFetchCall[] = [];

    globalThis.fetch = (async (input: FetchTarget, init?: FetchInit): Promise<Response> => {
      calls.push({
        url: input.toString(),
        method: init?.method ?? "GET",
        body: init?.body?.toString() || "",
      });
      return new Response(JSON.stringify({ ok: true, logs: [] }), { status: 200 });
    }) as unknown as typeof fetch;

    const completedAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-completed-app",
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        hasUnseenCompletion: true,
        completedAt: Date.now() - 5000,
      },
    };

    const { container, unmount } = await mountComponent(
      <App apiBaseUrl="http://localhost:4501" token="custom-token" />,
      mockDoc,
    );

    await act(async () => {
      const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
        detail: {
          type: "snapshot",
          at: Date.now(),
          agents: [completedAgent],
          environments: [completedAgent.environment],
        },
      });
      window.dispatchEvent(snapshotEvent);
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const card = container.querySelector('[data-testid="AgentCard"]');
    assert(card, "AgentCard must be rendered");

    const statusBadge = container.querySelector('[data-testid="StatusBadge--label"]');
    expect(statusBadge?.textContent).toBe("Completed");

    // Click card to acknowledge completion
    await act(async () => {
      card.click();
      await new Promise((resolve) => setTimeout(resolve, 20));
    });

    const patchCall = calls.find((c) => c.method === "PATCH");
    assert(patchCall, "PATCH call must be made");
    expect(patchCall.url).toBe("http://localhost:4501/api/v1/agents/agent-completed-app");
    expect(patchCall.body).toBe(JSON.stringify({ dismissCompletion: true }));
    expect(statusBadge?.textContent).toBe("Idle");

    await unmount();
  });

  it("logs error via console.error when dismissCompletion fails in App layout", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const originalConsoleError = console.error;
    const errorLogs: unknown[][] = [];
    console.error = (...args: unknown[]) => {
      errorLogs.push(args);
    };

    try {
      globalThis.fetch = (async (): Promise<Response> => {
        return new Response(JSON.stringify({ error: "Database disk is full" }), {
          status: 500,
          statusText: "Internal Server Error",
        });
      }) as unknown as typeof fetch;

      const completedAgent: IAgentState = {
        ...fixture_agentStateRunning,
        id: "agent-completed-fail",
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          hasUnseenCompletion: true,
          completedAt: Date.now() - 5000,
        },
      };

      const { container, unmount } = await mountComponent(
        <App apiBaseUrl="http://localhost:4501" token="custom-token" />,
        mockDoc,
      );

      await act(async () => {
        const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
          detail: {
            type: "snapshot",
            at: Date.now(),
            agents: [completedAgent],
            environments: [completedAgent.environment],
          },
        });
        window.dispatchEvent(snapshotEvent);
        await new Promise((resolve) => setTimeout(resolve, 20));
      });

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card, "AgentCard must be rendered");

      // Click card to acknowledge completion
      await act(async () => {
        card.click();
        await new Promise((resolve) => setTimeout(resolve, 50));
      });

      const errorLogged = errorLogs.some((args) =>
        args.some((arg) => typeof arg === "string" && arg.includes("Failed to dismiss completion")),
      );
      expect(errorLogged).toBe(true);

      await unmount();
    } finally {
      console.error = originalConsoleError;
    }
  });

  it("reverts optimistic completion dismissal and restores Completed badge when dismissCompletion fails", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    envCleanup = cleanup;

    const originalConsoleError = console.error;
    console.error = () => {};

    try {
      const { promise: fetchPromise, resolve: resolveFetch } = Promise.withResolvers<Response>();
      globalThis.fetch = (() => fetchPromise) as unknown as typeof fetch;

      const completedAgent: IAgentState = {
        ...fixture_agentStateRunning,
        id: "agent-completed-rollback",
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          hasUnseenCompletion: true,
          completedAt: Date.now() - 5000,
        },
      };

      const { container, unmount } = await mountComponent(
        <App apiBaseUrl="http://localhost:4501" token="custom-token" />,
        mockDoc,
      );

      await act(async () => {
        const snapshotEvent = new CustomEvent<ServerEvent>("mock-sse-event", {
          detail: {
            type: "snapshot",
            at: Date.now(),
            agents: [completedAgent],
            environments: [completedAgent.environment],
          },
        });
        window.dispatchEvent(snapshotEvent);
        await new Promise((resolve) => setTimeout(resolve, 20));
      });

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card, "AgentCard must be rendered");

      const statusBadgeBefore = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusBadgeBefore?.textContent).toBe("Completed");

      // Click card to acknowledge completion: triggers optimistic update while fetch is pending
      await act(async () => {
        card.click();
        await new Promise((resolve) => setTimeout(resolve, 10));
      });

      const statusBadgeOptimistic = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusBadgeOptimistic?.textContent).toBe("Idle");

      // Resolve fetch with 500 failure
      await act(async () => {
        resolveFetch(
          new Response(JSON.stringify({ error: "Database disk is full" }), {
            status: 500,
            statusText: "Internal Server Error",
          }),
        );
        await new Promise((resolve) => setTimeout(resolve, 50));
      });

      // After dismissal failure, optimistic update must be rolled back:
      const statusBadgeAfter = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusBadgeAfter?.textContent).toBe("Completed");

      await unmount();
    } finally {
      console.error = originalConsoleError;
    }
  });
});
