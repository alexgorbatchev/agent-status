import { describe, it, expect } from "bun:test";
import assert from "node:assert";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentCard } from "../components/AgentCard";
import type { IAgentState } from "../types";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_agentStateExited,
  fixture_minimalAgentState,
  factory_agentState,
} from "./fixtures";

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

  it("invokes onOpenDrawer callback with agent when card is clicked", () => {
    let receivedAgent: IAgentState | null = null;
    const onOpenDrawer = (agent: IAgentState) => {
      receivedAgent = agent;
    };

    const element = AgentCard({ agent: fixture_agentStateRunning, onOpenDrawer });
    element.props.onClick();

    assert(receivedAgent);
    expect<IAgentState | null>(receivedAgent).toBe(fixture_agentStateRunning);
  });

  it("handles card click safely when onOpenDrawer is undefined", () => {
    const element = AgentCard({ agent: fixture_agentStateRunning });
    expect(() => element.props.onClick()).not.toThrow();
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
});
