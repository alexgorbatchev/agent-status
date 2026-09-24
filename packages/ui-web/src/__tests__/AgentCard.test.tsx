import { describe, it, expect } from "bun:test";
import assert from "node:assert";
import { act } from "react";
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
    // fixture_agentStateRunning has a historical timestamp triggering the stall silence badge
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

  it("does not display 'Idle' in AgentCard--action and reserves 2 rows", () => {
    const idleAgent = factory_agentState({
      activity: {
        action: "Idle",
        activeTool: null,
      } as unknown as IAgentState["activity"],
    });
    const html = renderToStaticMarkup(<AgentCard agent={idleAgent} />);

    const match = html.match(/<p[^>]*data-testid="AgentCard--action"[^>]*>(.*?)<\/p>/);
    assert(match);
    expect(match[1]).not.toContain("Idle");
    expect(match[0]).toContain("h-8");
    expect(match[0]).toContain("leading-4");
    expect(match[0]).toContain("line-clamp-2");
  });

  it("does not display 'Idle' in AgentCard--action when action is empty and activeTool is absent", () => {
    const emptyActionAgent = factory_agentState({
      activity: {
        action: "",
        activeTool: null,
      } as unknown as IAgentState["activity"],
    });
    const html = renderToStaticMarkup(<AgentCard agent={emptyActionAgent} />);

    const match = html.match(/<p[^>]*data-testid="AgentCard--action"[^>]*>(.*?)<\/p>/);
    assert(match);
    expect(match[1]).not.toContain("Idle");
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

  it("renders last active timestamp badge with relative time and title tooltip for non-busy agent", () => {
    const agent = factory_agentState({
      activity: {
        ...fixture_agentStateRunning.activity,
        activity: "IDLE",
        updatedAt: Date.now() - 30000,
      },
    });
    const html = renderToStaticMarkup(<AgentCard agent={agent} />);

    expect(html).toContain('data-testid="AgentCard--last-active"');
    expect(html).toContain("30s ago");
    expect(html).toContain(`title="Last active: ${new Date(agent.activity.updatedAt).toLocaleString()}"`);
  });

  it("renders fallback '—' when updatedAt is 0 or negative for non-busy agent", () => {
    const agent = factory_agentState({
      activity: {
        ...fixture_agentStateRunning.activity,
        activity: "IDLE",
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

  it("does not render last-active badge when running session is busy and not stalled, and displays 'Working' status", () => {
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
    expect(html).toContain("Working");
    expect(html).not.toContain("Busy");
    expect(html).not.toContain("Stalled");
    expect(html).not.toContain('data-testid="AgentCard--last-active"');
    expect(html).not.toContain("Active ");
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

  it("renders header with left-aligned harness badge and hide button without bullet separator", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} onToggleHide={() => {}} />);

    expect(html).not.toContain("•");

    const harnessIndex = html.indexOf('data-testid="AgentCard--harness-badge"');
    const hideIndex = html.indexOf('data-testid="AgentCard--hide-button"');
    const statusIndex = html.indexOf('data-testid="StatusBadge"');

    expect(harnessIndex).toBeGreaterThan(-1);
    expect(hideIndex).toBeGreaterThan(-1);
    expect(statusIndex).toBeGreaterThan(-1);
    expect(harnessIndex).toBeLessThan(hideIndex);
    expect(hideIndex).toBeLessThan(statusIndex);
  });

  it("renders env badge before branch badge in metadata badges", () => {
    const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} />);

    const envIndex = html.indexOf('data-testid="AgentCard--env-badge"');
    const branchIndex = html.indexOf('data-testid="AgentCard--branch-badge"');

    expect(envIndex).toBeGreaterThan(-1);
    expect(branchIndex).toBeGreaterThan(-1);
    expect(envIndex).toBeLessThan(branchIndex);
    expect(html).toContain(fixture_agentStateRunning.environment.label);
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
      expect(html).not.toContain("⏱️");
      expect(html).not.toContain('data-testid="AgentCard--stall-warning"');

      const lastActiveIndex = html.indexOf('data-testid="AgentCard--last-active"');
      const stallNoticeIndex = html.indexOf('data-testid="AgentCard--stall-notice"');
      expect(lastActiveIndex).toBeGreaterThan(-1);
      expect(stallNoticeIndex).toBeGreaterThan(lastActiveIndex);
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
      expect(html).not.toContain("⏱️");
      expect(html).toContain('data-stall-tier="warning"');

      const lastActiveIndex = html.indexOf('data-testid="AgentCard--last-active"');
      const stallDurationIndex = html.indexOf('data-testid="AgentCard--stall-duration"');
      expect(lastActiveIndex).toBeGreaterThan(-1);
      expect(stallDurationIndex).toBeGreaterThan(lastActiveIndex);
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

  describe("Subagent status chips", () => {
    it("hides subagent chips by default and toggles visibility when subagents badge is clicked", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      let drawerOpened = false;
      const onOpenDrawer = (): void => {
        drawerOpened = true;
      };

      try {
        const agentWithSubagents: IAgentState = {
          ...fixture_agentStateRunning,
          activity: {
            ...fixture_agentStateRunning.activity,
            subagentCount: 2,
            activeSubagents: [
              {
                id: "sub-1",
                activity: "BUSY",
                activeTool: "bash",
                action: "npm test",
              },
              {
                id: "sub-2",
                activity: "ERROR",
                lastError: "compile error",
              },
            ],
          },
        };

        // Static markup has subagents hidden by default
        const staticHtml = renderToStaticMarkup(<AgentCard agent={agentWithSubagents} />);
        expect(staticHtml).toContain('data-testid="AgentCard--subagents-badge"');
        expect(staticHtml).not.toContain('data-testid="AgentCard--subagents"');

        // Dynamic interactive mount
        const { container, unmount } = await mountComponent(
          <AgentCard agent={agentWithSubagents} onOpenDrawer={onOpenDrawer} />,
          mockDoc,
        );

        // Hidden by default
        expect(container.querySelector('[data-testid="AgentCard--subagents"]')).toBeNull();

        const badge = container.querySelector('[data-testid="AgentCard--subagents-badge"]') as HTMLElement | null;
        assert(badge, "subagents badge must be present");
        expect(badge.getAttribute("aria-expanded")).toBe("false");

        // Click badge to expand
        await act(async () => {
          badge.click();
        });
        expect(drawerOpened).toBe(false); // click must not propagate to onOpenDrawer
        expect(badge.getAttribute("aria-expanded")).toBe("true");

        const subagentsContainer = container.querySelector('[data-testid="AgentCard--subagents"]');
        assert(subagentsContainer, "subagents container must be visible after click");
        expect(container.querySelector('[data-testid="AgentCard--subagent-chip-sub-1"]')).not.toBeNull();
        expect(container.querySelector('[data-testid="AgentCard--subagent-chip-sub-2"]')).not.toBeNull();

        // Click badge again to collapse
        await act(async () => {
          badge.click();
        });
        expect(badge.getAttribute("aria-expanded")).toBe("false");
        expect(container.querySelector('[data-testid="AgentCard--subagents"]')).toBeNull();

        await unmount();
      } finally {
        cleanup();
      }
    });

    it("renders subagents with distinct running vs completed status indicators and smooth transitions", () => {
      const agentWithDwellSubagents: IAgentState = {
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          activeSubagents: [
            {
              id: "sub-running",
              activity: "BUSY",
              activeTool: "Grep",
              status: "running",
              startedAt: 1000,
            },
            {
              id: "sub-settling",
              activity: "IDLE",
              status: "completed",
              startedAt: 1000,
              completedAt: 3000,
            },
          ],
        },
      };

      const html = renderToStaticMarkup(<AgentCard agent={agentWithDwellSubagents} defaultShowSubagents />);

      // Chip status attributes
      expect(html).toContain('data-subagent-status="running"');
      expect(html).toContain('data-subagent-status="completed"');

      // Status indicator element per chip
      expect(html).toContain('data-testid="AgentCard--subagent-status-sub-running"');
      expect(html).toContain('data-testid="AgentCard--subagent-status-sub-settling"');

      // Smooth transition classes on completed subagent chip
      expect(html).toContain("transition-all");
    });
  });

  describe("Focus in Herdr button", () => {
    it("renders Focus in Herdr button enabled for running agent and disabled for exited agent", () => {
      const runningHtml = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} />);
      expect(runningHtml).toContain('data-testid="AgentCard--focus-button"');
      expect(runningHtml).toContain('aria-label="Focus in Herdr"');
      expect(runningHtml).not.toContain('disabled=""');

      const exitedHtml = renderToStaticMarkup(<AgentCard agent={fixture_agentStateExited} />);
      expect(exitedHtml).toContain('data-testid="AgentCard--focus-button"');
      expect(exitedHtml).toContain("disabled");
    });

    it("prevents propagation to card on click and invokes focusAgent", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      let openedDrawer = false;
      const originalFetch = globalThis.fetch;
      const fetchedUrls: string[] = [];
      globalThis.fetch = (async (input: unknown): Promise<Response> => {
        fetchedUrls.push(String(input));
        return new Response(JSON.stringify({ ok: true, paneId: "w1:p1" }), { status: 200 });
      }) as unknown as typeof fetch;

      try {
        const { container, unmount } = await mountComponent(
          <AgentCard
            agent={fixture_agentStateRunning}
            onOpenDrawer={() => {
              openedDrawer = true;
            }}
          />,
          mockDoc,
        );

        const focusBtn = container.querySelector('[data-testid="AgentCard--focus-button"]');
        assert(focusBtn, "Focus button must be present");

        await act(async () => {
          focusBtn.click();
        });

        expect(openedDrawer).toBe(false);
        expect(fetchedUrls.length).toBe(1);
        expect(fetchedUrls[0]).toContain(`/api/v1/agents/${encodeURIComponent(fixture_agentStateRunning.id)}/focus`);

        await unmount();
      } finally {
        globalThis.fetch = originalFetch;
        cleanup();
      }
    });

    it("surfaces error feedback when focus fails", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      const originalFetch = globalThis.fetch;
      globalThis.fetch = (async (): Promise<Response> => {
        return new Response(JSON.stringify({ error: "Herdr daemon offline" }), { status: 503 });
      }) as unknown as typeof fetch;

      try {
        const { container, unmount } = await mountComponent(<AgentCard agent={fixture_agentStateRunning} />, mockDoc);

        const focusBtn = container.querySelector('[data-testid="AgentCard--focus-button"]');
        assert(focusBtn, "Focus button must be present");

        await act(async () => {
          focusBtn.click();
        });

        const errorEl = container.querySelector('[data-testid="AgentCard--focus-error"]');
        assert(errorEl, "Error feedback must be rendered");
        expect(errorEl.textContent).toContain("Herdr daemon offline");

        await unmount();
      } finally {
        globalThis.fetch = originalFetch;
        cleanup();
      }
    });

    it("strips redundant project prefix from session title in card display", () => {
      const agent = factory_agentState({
        project: {
          id: "github.com/alexgorbatchev/agent-status",
          name: "agent-status",
          canonicalRoot: "/repo",
        },
        activity: {
          ...fixture_agentStateRunning.activity,
          title: "agent-status: Add test coverage for title trimming",
        },
      });

      const html = renderToStaticMarkup(<AgentCard agent={agent} />);
      expect(html).toContain('data-testid="AgentCard--title"');
      expect(html).toContain("Add test coverage for title trimming");
      expect(html).not.toContain("agent-status: Add test coverage for title trimming");
    });

    it("strips bracketed project prefix from session title in card display", () => {
      const agent = factory_agentState({
        project: {
          id: "github.com/alexgorbatchev/agent-status",
          name: "agent-status",
          canonicalRoot: "/repo",
        },
        activity: {
          ...fixture_agentStateRunning.activity,
          title: "[agent-status] Implement OAuth tokens",
        },
      });

      const html = renderToStaticMarkup(<AgentCard agent={agent} />);
      expect(html).toContain("Implement OAuth tokens");
      expect(html).not.toContain("[agent-status] Implement OAuth tokens");
    });

    it("uses explicit projectName prop to strip project prefix when provided", () => {
      const agent = factory_agentState({
        project: {
          id: "custom-id",
          name: "custom-name",
          canonicalRoot: "/repo",
        },
        activity: {
          ...fixture_agentStateRunning.activity,
          title: "override-project · Fix database connection pooling",
        },
      });

      const html = renderToStaticMarkup(<AgentCard agent={agent} projectName="override-project" />);
      expect(html).toContain("Fix database connection pooling");
      expect(html).not.toContain("override-project · Fix database connection pooling");
    });

    it("preserves title when title equals project name without substantive tail", () => {
      const agent = factory_agentState({
        project: {
          id: "github.com/alexgorbatchev/agent-status",
          name: "agent-status",
          canonicalRoot: "/repo",
        },
        activity: {
          ...fixture_agentStateRunning.activity,
          title: "agent-status",
        },
      });

      const html = renderToStaticMarkup(<AgentCard agent={agent} />);
      expect(html).toContain("agent-status");
    });
  });

  describe("Semantic Precedence over Harness Branding (#106)", () => {
    it("enforces red card border and suppresses brand color on harness mark during ERROR state", () => {
      const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateError} />);

      // Outer card border highlight must switch to semantic error red
      const cardMatch = html.match(/data-testid="AgentCard"[^>]*class="([^"]*)"/);
      assert(cardMatch);
      const cardClass = cardMatch[1];
      expect(cardClass).toContain("border-red-600/80");
      expect(cardClass).not.toContain("border-slate-800");
      expect(html).toContain('data-semantic-status="error"');

      // Harness mark badge must be suppressed to muted brand state and must not carry active brand color
      expect(html).toContain('data-brand-state="muted"');
      const badgeMatch = html.match(/data-testid="AgentCard--harness-badge"[^>]*class="([^"]*)"/);
      assert(badgeMatch);
      const badgeClass = badgeMatch[1];
      expect(badgeClass).toContain("text-slate-500");
      expect(badgeClass).not.toContain("text-[var(--brand-claude)]");

      // Attention callout strictly uses semantic red
      expect(html).toContain('data-testid="AgentCard--error"');
      expect(html).toContain('data-callout-type="error"');
      expect(html).toContain("border-red-800/60");
    });

    it("enforces amber card border and suppresses brand color on harness mark during WAITING_QUESTION state", () => {
      const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateWaitingQuestion} />);

      // Outer card border highlight must switch to semantic warning amber
      const cardMatch = html.match(/data-testid="AgentCard"[^>]*class="([^"]*)"/);
      assert(cardMatch);
      const cardClass = cardMatch[1];
      expect(cardClass).toContain("border-amber-600/80");
      expect(cardClass).not.toContain("border-slate-800");
      expect(html).toContain('data-semantic-status="warning"');

      // Harness mark badge must be suppressed to muted brand state and must not carry active brand color
      expect(html).toContain('data-brand-state="muted"');
      const badgeMatch = html.match(/data-testid="AgentCard--harness-badge"[^>]*class="([^"]*)"/);
      assert(badgeMatch);
      const badgeClass = badgeMatch[1];
      expect(badgeClass).toContain("text-slate-500");
      expect(badgeClass).not.toContain("text-[var(--brand-claude)]");

      // Attention callout strictly uses semantic amber
      expect(html).toContain('data-testid="AgentCard--question"');
      expect(html).toContain('data-callout-type="warning"');
      expect(html).toContain("border-amber-800/60");
    });

    it("renders active brand color on harness mark only during neutral working state", () => {
      const workingAgent = factory_agentState({
        harness: "claude-code",
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          updatedAt: Date.now(), // not stalled
        },
      });

      const html = renderToStaticMarkup(<AgentCard agent={workingAgent} />);

      // Card border must be neutral slate, not error red or warning amber
      const cardMatch = html.match(/data-testid="AgentCard"[^>]*class="([^"]*)"/);
      assert(cardMatch);
      const cardClass = cardMatch[1];
      expect(cardClass).toContain("border-slate-800");
      expect(cardClass).not.toContain("border-red-600/80");
      expect(cardClass).not.toContain("border-amber-600/80");

      // Harness badge has active brand color token
      expect(html).toContain('data-brand-state="active"');
      const badgeMatch = html.match(/data-testid="AgentCard--harness-badge"[^>]*class="([^"]*)"/);
      assert(badgeMatch);
      const badgeClass = badgeMatch[1];
      expect(badgeClass).toContain("text-[var(--brand-claude)]");
      expect(badgeClass).not.toContain("text-slate-500");
    });

    it("renders quiescent muted ink for finished agents without brand dominance", () => {
      const finishedAgent = factory_agentState({
        harness: "pi",
        activity: {
          ...fixture_agentStateExited.activity,
          lifecycle: "EXITED",
          activity: "IDLE",
        },
      });

      const html = renderToStaticMarkup(<AgentCard agent={finishedAgent} />);

      expect(html).toContain('data-brand-state="quiescent"');
      const badgeMatch = html.match(/data-testid="AgentCard--harness-badge"[^>]*class="([^"]*)"/);
      assert(badgeMatch);
      const badgeClass = badgeMatch[1];
      expect(badgeClass).toContain("text-slate-400");
      expect(badgeClass).not.toContain("text-[var(--brand-pi)]");
    });
  });

  describe("idle freshness tiers and visual dimming", () => {
    it("sets data-freshness='fresh' on active RUNNING BUSY agents", () => {
      const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} />);
      expect(html).toContain('data-freshness="fresh"');
      expect(html).not.toContain("bg-slate-950/40");
    });

    it("sets data-freshness='fresh' on RUNNING IDLE agents updated < 15m ago", () => {
      const agent = factory_agentState({
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          updatedAt: Date.now() - 5 * 60 * 1000,
        },
      });
      const html = renderToStaticMarkup(<AgentCard agent={agent} />);
      expect(html).toContain('data-freshness="fresh"');
      expect(html).not.toContain("bg-slate-950/40");
    });

    it("sets data-freshness='standard' on RUNNING IDLE agents updated between 15m and 2h ago", () => {
      const agent = factory_agentState({
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          updatedAt: Date.now() - 30 * 60 * 1000,
        },
      });
      const html = renderToStaticMarkup(<AgentCard agent={agent} />);
      expect(html).toContain('data-freshness="standard"');
      expect(html).not.toContain("bg-slate-950/40");
    });

    it("sets data-freshness='stale' and applies dimming classes on RUNNING IDLE agents updated >= 2h ago", () => {
      const agent = factory_agentState({
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          updatedAt: Date.now() - 3 * 60 * 60 * 1000,
        },
      });
      const html = renderToStaticMarkup(<AgentCard agent={agent} />);
      expect(html).toContain('data-freshness="stale"');
      expect(html).toContain("opacity-60 bg-slate-950/40 border-slate-800/50 hover:opacity-100");
    });

    it("sets data-freshness='fresh' on finished EXITED agents", () => {
      const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateExited} />);
      expect(html).toContain('data-freshness="fresh"');
    });
  });

  describe("Unacknowledged completion (#108)", () => {
    it("renders Completed status badge when agent has unseen completion", () => {
      const completedAgent = factory_agentState({
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          hasUnseenCompletion: true,
          completedAt: Date.now() - 10000,
        },
      });

      const html = renderToStaticMarkup(<AgentCard agent={completedAgent} />);
      expect(html).toContain('<span data-testid="StatusBadge--label">Completed</span>');
    });

    it("acknowledges completion and returns badge to Idle when clicked", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      let acknowledged = false;
      let openedDrawer = false;
      const completedAgent = factory_agentState({
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          hasUnseenCompletion: true,
          completedAt: Date.now() - 5000,
        },
      });

      const { container, unmount } = await mountComponent(
        <AgentCard
          agent={completedAgent}
          onOpenDrawer={() => {
            openedDrawer = true;
          }}
          onAcknowledgeCompletion={() => {
            acknowledged = true;
          }}
        />,
        mockDoc,
      );

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card, "AgentCard must be present");

      const statusLabelBefore = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusLabelBefore?.textContent).toBe("Completed");

      await act(async () => {
        card.click();
      });

      expect(openedDrawer).toBe(true);
      expect(acknowledged).toBe(true);

      const statusLabelAfter = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusLabelAfter?.textContent).toBe("Idle");

      await unmount();
      cleanup();
    });

    it("acknowledges completion when card receives focus", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      let acknowledged = false;
      const completedAgent = factory_agentState({
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          hasUnseenCompletion: true,
          completedAt: Date.now() - 5000,
        },
      });

      const { container, unmount } = await mountComponent(
        <AgentCard
          agent={completedAgent}
          onAcknowledgeCompletion={() => {
            acknowledged = true;
          }}
        />,
        mockDoc,
      );

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card, "AgentCard must be present");

      const statusLabelBefore = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusLabelBefore?.textContent).toBe("Completed");

      await act(async () => {
        card.dispatchEvent({ type: "focusin", bubbles: true });
        card.dispatchEvent({ type: "focus", bubbles: true });
      });

      expect(acknowledged).toBe(true);
      const statusLabelAfter = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusLabelAfter?.textContent).toBe("Idle");

      await unmount();
      cleanup();
    });

    it("acknowledges completion on Enter key down", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      let acknowledged = false;
      const completedAgent = factory_agentState({
        activity: {
          ...fixture_agentStateRunning.activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          hasUnseenCompletion: true,
          completedAt: Date.now() - 5000,
        },
      });

      const { container, unmount } = await mountComponent(
        <AgentCard
          agent={completedAgent}
          onAcknowledgeCompletion={() => {
            acknowledged = true;
          }}
        />,
        mockDoc,
      );

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card, "AgentCard must be present");

      await act(async () => {
        card.dispatchEvent({ type: "keydown", key: "Enter", bubbles: true });
      });

      expect(acknowledged).toBe(true);
      const statusLabelAfter = container.querySelector('[data-testid="StatusBadge--label"]');
      expect(statusLabelAfter?.textContent).toBe("Idle");

      await unmount();
      cleanup();
    });
  });
});
