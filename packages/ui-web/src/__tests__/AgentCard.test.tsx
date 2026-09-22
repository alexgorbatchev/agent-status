import { describe, it, expect } from "bun:test";
import assert from "node:assert";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentCard } from "../components/AgentCard";
import type { IAgentState } from "../types";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_agentStateExited,
  fixture_minimalAgentState,
  factory_agentState,
} from "./fixtures";

interface IAgentHolder {
  toggledAgent?: IAgentState;
}

describe("AgentCard Component", () => {
  it("renders running agent card with all standard fields", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} />);

    expect(html).toContain('data-testid="AgentCard"');
    expect(html).toContain("work-macbook");
    expect(html).toContain("Claude Code");
    expect(html).toContain("Build agent-status dashboard");
    expect(html).toContain("Compiling typescript and verifying bundles");
    expect(html).toContain("main");
    expect(html).toContain("2 subagents");
    expect(html).toContain('data-testid="AgentCard--last-active"');
    expect(html).toContain("$0.0560");
    expect(html).toContain("7,300 tokens");
  });

  it("renders Pi harness name when harness is pi", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateExited} />);

    expect(html).toContain("Pi");
    expect(html).toContain("Refactor telemetry pipeline");
    expect(html).toContain("Completed task successfully");
    expect(html).toContain("$0.0920");
    expect(html).toContain("12,100 tokens");
  });

  it("renders question detail block when activity is WAITING_QUESTION with question text", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateWaitingQuestion} />);

    expect(html).toContain('data-testid="AgentCard--question"');
    expect(html).toContain("Should we use SQLite or PostgreSQL for auth tokens?");
    expect(html).toContain(".workspaces/feat-auth");
    expect(html).toContain("feat/auth");
  });

  it("renders error detail block when activity is ERROR with lastError text", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateError} />);

    expect(html).toContain('data-testid="AgentCard--error"');
    expect(html).toContain("Database migration returned exit code 1: syntax error near &#x27;INDEX&#x27;");
    expect(html).toContain("1 subagent");
  });

  it("renders fallback values for minimal agent with omitted fields", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_minimalAgentState} />);

    expect(html).toContain("Untitled Agent Session");
    expect(html).toContain("Idle");
    expect(html).toContain("HEAD");
    expect(html).toContain("—");
    expect(html).toContain("0 tokens");
    expect(html).not.toContain('data-testid="AgentCard--question"');
    expect(html).not.toContain('data-testid="AgentCard--error"');
    expect(html).not.toContain("subagent");
  });

  it("renders tool execution fallback when action is empty but activeTool is present", () => {
    const agent = factory_agentState({
      activity: {
        action: "",
        activeTool: "GrepSearch",
      } as unknown as IAgentState["activity"],
    });
    const html = renderToStaticMarkup(<AgentCard agent={agent} />);

    expect(html).toContain("Executing GrepSearch");
  });

  it("invokes onOpenDrawer callback with agent when card is clicked", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    let receivedAgent: IAgentState | null = null;
    const onOpenDrawer = (agent: IAgentState) => {
      receivedAgent = agent;
    };

    const { container, unmount } = await mountComponent(
      <AgentCard agent={fixture_agentStateRunning} onOpenDrawer={onOpenDrawer} />,
      mockDoc,
    );

    const card = container.querySelector('[data-testid="AgentCard"]');
    assert(card);
    card.click();

    assert(receivedAgent);
    expect<IAgentState | null>(receivedAgent).toBe(fixture_agentStateRunning);
    await unmount();
    cleanup();
  });

  it("handles card click safely when onOpenDrawer is undefined", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    const { container, unmount } = await mountComponent(<AgentCard agent={fixture_agentStateRunning} />, mockDoc);

    const card = container.querySelector('[data-testid="AgentCard"]');
    assert(card);
    expect(() => card.click()).not.toThrow();
    await unmount();
    cleanup();
  });

  it("applies reduced opacity 0.6 and hover:opacity-100 for finished (EXITED) agents", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateExited} />);
    expect(html).toContain("opacity-60");
    expect(html).toContain("hover:opacity-100");
  });

  it("does not apply reduced opacity for active (RUNNING) agents", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} />);
    expect(html).not.toContain("opacity-60");
  });

  it("renders last active timestamp badge with relative time and title tooltip", () => {
    const agent = factory_agentState({
      activity: {
        ...fixture_agentStateRunning.activity,
        updatedAt: Date.now() - 30000,
      },
    });
    const html = renderToStaticMarkup(<AgentCard agent={agent} />);

    expect(html).toContain('data-testid="AgentCard--last-active"');
    expect(html).toContain("30s ago");
    expect(html).toContain(`title="Last active: ${new Date(agent.activity.updatedAt).toLocaleString()}"`);
  });

  it("renders fallback '—' when updatedAt is 0 or negative", () => {
    const agent = factory_agentState({
      activity: {
        ...fixture_agentStateRunning.activity,
        updatedAt: 0,
      },
    });
    const html = renderToStaticMarkup(<AgentCard agent={agent} />);

    expect(html).toContain('data-testid="AgentCard--last-active"');
    expect(html).toContain('title="Last active: Never"');
  });

  it("renders silence stall indicator, Stalled badge, and amber card styling when running busy session is silent >= 120s", () => {
    const now = Date.now();
    const agent = factory_agentState({
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
        activeTool: null,
        activeToolStartedAt: null,
        updatedAt: now - 130000,
      },
    });
    const html = renderToStaticMarkup(<AgentCard agent={agent} />);

    expect(html).toContain('data-testid="AgentCard"');
    expect(html).toContain("border-amber-500/80");
    expect(html).toContain('data-testid="StatusBadge"');
    expect(html).toContain("Stalled (2m)");
    expect(html).toContain('data-testid="AgentCard--last-active"');
    expect(html).toContain("Silent for 2m");
  });

  it("renders Active relativeTime when running busy session is active < 120s", () => {
    const now = Date.now();
    const agent = factory_agentState({
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
        activeTool: null,
        activeToolStartedAt: null,
        updatedAt: now - 15000,
      },
    });
    const html = renderToStaticMarkup(<AgentCard agent={agent} />);

    expect(html).toContain('data-testid="StatusBadge"');
    expect(html).toContain("Busy");
    expect(html).not.toContain("Stalled");
    expect(html).toContain('data-testid="AgentCard--last-active"');
    expect(html).toContain("Active ");
  });

  it("renders hide button and calls onToggleHide without triggering onOpenDrawer", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    const holder: IAgentHolder = {};
    let drawerOpened = false;

    const onToggleHide = (agent: IAgentState) => {
      holder.toggledAgent = agent;
    };
    const onOpenDrawer = () => {
      drawerOpened = true;
    };

    const { container, unmount } = await mountComponent(
      <AgentCard agent={fixture_agentStateRunning} onToggleHide={onToggleHide} onOpenDrawer={onOpenDrawer} />,
      mockDoc,
    );

    const hideBtn = container.querySelector('[data-testid="AgentCard--hide-button"]');
    assert(hideBtn, "hide button must exist when onToggleHide is provided");
    expect(hideBtn.getAttribute("class")).toContain("cursor-pointer");
    hideBtn.click();

    assert(holder.toggledAgent, "toggledAgent must be defined");
    expect(holder.toggledAgent.id).toBe(fixture_agentStateRunning.id);
    expect(drawerOpened).toBe(false);

    await unmount();
    cleanup();
  });

  it("renders hidden badge and unhide button for hidden agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    const holder: IAgentHolder = {};
    const onToggleHide = (agent: IAgentState) => {
      holder.toggledAgent = agent;
    };

    const hiddenAgent: IAgentState = {
      ...fixture_agentStateRunning,
      isHidden: true,
      hiddenAt: Date.now(),
    };

    const { container, unmount } = await mountComponent(
      <AgentCard agent={hiddenAgent} onToggleHide={onToggleHide} />,
      mockDoc,
    );

    const badge = container.querySelector('[data-testid="AgentCard--hidden-badge"]');
    assert(badge, "hidden badge must be displayed");

    const unhideBtn = container.querySelector('[data-testid="AgentCard--unhide-button"]');
    assert(unhideBtn, "unhide button must be displayed");
    unhideBtn.click();

    assert(holder.toggledAgent, "toggledAgent must be defined");
    expect(holder.toggledAgent.id).toBe(hiddenAgent.id);

    await unmount();
    cleanup();
  });

  it("does not trigger onOpenDrawer when keydown event occurs on hide button", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    let drawerOpened = false;

    const onOpenDrawer = () => {
      drawerOpened = true;
    };

    const { container, unmount } = await mountComponent(
      <AgentCard agent={fixture_agentStateRunning} onToggleHide={() => {}} onOpenDrawer={onOpenDrawer} />,
      mockDoc,
    );

    const hideBtn = container.querySelector('[data-testid="AgentCard--hide-button"]');
    assert(hideBtn);

    hideBtn.dispatchEvent({ type: "keydown", key: "Enter", target: hideBtn });
    expect(drawerOpened).toBe(false);

    await unmount();
    cleanup();
  });

  describe("stall detection tiers", () => {
    it("renders healthy tier (<60s) without stall badges or warnings", () => {
      const now = Date.now();
      const healthyAgent: IAgentState = {
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: "bash",
          activeToolStartedAt: now - 30000,
        },
      };
      const html = renderToStaticMarkup(<AgentCard agent={healthyAgent} />);
      expect(html).not.toContain('data-testid="AgentCard--stall-notice"');
      expect(html).not.toContain('data-testid="AgentCard--stall-warning"');
      expect(html).not.toContain('data-testid="AgentCard--stall-duration"');
    });

    it("renders notice tier (60s - 119s) with metadata notice badge", () => {
      const now = Date.now();
      const noticeAgent: IAgentState = {
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: "bash",
          activeToolStartedAt: now - 75000,
        },
      };
      const html = renderToStaticMarkup(<AgentCard agent={noticeAgent} />);
      expect(html).toContain('data-testid="AgentCard--stall-notice"');
      expect(html).toContain("bash running for 1m 15s");
      expect(html).not.toContain('data-testid="AgentCard--stall-warning"');
    });

    it("renders warning tier (120s - 179s) with diagnostic callout and duration badge", () => {
      const now = Date.now();
      const warningAgent: IAgentState = {
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: "bash",
          action: "git rebase main",
          activeToolStartedAt: now - 135000,
        },
      };
      const html = renderToStaticMarkup(<AgentCard agent={warningAgent} />);
      expect(html).toContain('data-testid="AgentCard--stall-warning"');
      expect(html).toContain("Command running for 2m 15s without output or return");
      expect(html).toContain("Command: git rebase main");
      expect(html).toContain('data-testid="AgentCard--stall-duration"');
      expect(html).toContain("2m 15s on tool");
      expect(html).toContain('data-stall-tier="warning"');
    });

    it("renders stalled tier (>=180s) with amber glow border and diagnostic callout", () => {
      const now = Date.now();
      const stalledAgent: IAgentState = {
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: "bash",
          action: "npm test",
          activeToolStartedAt: now - 200000,
        },
      };
      const html = renderToStaticMarkup(<AgentCard agent={stalledAgent} />);
      expect(html).toContain('data-testid="AgentCard--stall-warning"');
      expect(html).toContain("Command running for 3m 20s without output or return");
      expect(html).toContain("Command: npm test");
      expect(html).toContain("border-amber-500/80");
      expect(html).toContain('data-stall-tier="stalled"');
    });
  });
});
