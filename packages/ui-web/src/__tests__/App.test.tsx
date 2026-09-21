import { describe, it, expect, beforeEach, afterEach } from "bun:test";
import assert from "node:assert";
import React, { act } from "react";
import { App } from "../layouts/App";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion } from "./fixtures";
import type { IAgentState, ServerEvent } from "../types";

type CleanupFn = () => void;
type FetchTarget = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

describe("App Layout & Session Visibility", () => {
  let envCleanup: CleanupFn | null = null;
  let originalFetch: typeof globalThis.fetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
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

    // Toggle button should exist and have cursor-pointer
    const toggleBtn = container.querySelector('[data-testid="App--toggle-show-hidden"]');
    assert(toggleBtn, "toggle show hidden button must exist when hidden agents exist");
    expect(toggleBtn.textContent).toContain("Show 1 hidden");
    expect(toggleBtn.getAttribute("class")).toContain("cursor-pointer");

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
});
