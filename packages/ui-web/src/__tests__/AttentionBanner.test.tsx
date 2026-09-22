import { describe, it, expect } from "bun:test";
import assert from "node:assert";
import { renderToStaticMarkup } from "react-dom/server";
import { AttentionBanner } from "../components/AttentionBanner";
import type { IAgentState } from "../types";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_agentStateExited,
  fixture_staleEnvironmentInfo,
  factory_agentState,
} from "./fixtures";

describe("AttentionBanner Component", () => {
  it("returns null when agent list is empty", () => {
    const element = AttentionBanner({ agents: [], onSelectAgent: () => {} });
    expect(element).toBeNull();
  });

  it("returns null when all agents are running in BUSY or IDLE states", () => {
    const element = AttentionBanner({
      agents: [fixture_agentStateRunning],
      onSelectAgent: () => {},
    });
    expect(element).toBeNull();
  });

  it("returns null when agents have exited even if their activity was ERROR or WAITING_QUESTION", () => {
    const exitedError = factory_agentState({
      activity: {
        lifecycle: "EXITED",
        activity: "ERROR",
        lastError: "fatal crash",
      } as unknown as IAgentState["activity"],
    });
    const element = AttentionBanner({
      agents: [fixture_agentStateExited, exitedError],
      onSelectAgent: () => {},
    });
    expect(element).toBeNull();
  });

  it("returns null when agents require attention but belong to stale environments", () => {
    const staleWaiting = factory_agentState({
      environment: fixture_staleEnvironmentInfo,
      activity: {
        lifecycle: "RUNNING",
        activity: "WAITING_QUESTION",
        question: "Is this stale?",
      } as unknown as IAgentState["activity"],
    });
    const element = AttentionBanner({
      agents: [staleWaiting],
      onSelectAgent: () => {},
    });
    expect(element).toBeNull();
  });

  it("renders single agent card requiring attention for WAITING_QUESTION", () => {
    const html = renderToStaticMarkup(
      <AttentionBanner agents={[fixture_agentStateWaitingQuestion]} onSelectAgent={() => {}} />,
    );

    expect(html).toContain('data-testid="AttentionBanner"');
    expect(html).toContain("Act Now — 1 Agent Requires Attention");
    expect(html).toContain("Implement user authentication flow");
    expect(html).toContain("work-macbook");
    expect(html).toContain("Should we use SQLite or PostgreSQL for auth tokens?");
  });

  it("renders single agent card requiring attention for ERROR", () => {
    const html = renderToStaticMarkup(<AttentionBanner agents={[fixture_agentStateError]} onSelectAgent={() => {}} />);

    expect(html).toContain('data-testid="AttentionBanner"');
    expect(html).toContain("Act Now — 1 Agent Requires Attention");
    expect(html).toContain("Database schema migration");
    expect(html).toContain("work-macbook");
    expect(html).toContain("Database migration returned exit code 1: syntax error near &#x27;INDEX&#x27;");
  });

  it("renders multiple cards and plural heading when multiple agents require attention", () => {
    const html = renderToStaticMarkup(
      <AttentionBanner
        agents={[fixture_agentStateWaitingQuestion, fixture_agentStateError, fixture_agentStateRunning]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain("Act Now — 2 Agents Require Attention");
    expect(html).toContain("Implement user authentication flow");
    expect(html).toContain("Database schema migration");
  });

  it("renders fallback text when attention agent title, question, or lastError are empty", () => {
    const emptyWaiting = factory_agentState({
      activity: {
        lifecycle: "RUNNING",
        activity: "WAITING_QUESTION",
        title: "",
        question: null,
      } as unknown as IAgentState["activity"],
    });
    const emptyError = factory_agentState({
      id: "env-1:claude-code:session-empty-error",
      activity: {
        lifecycle: "RUNNING",
        activity: "ERROR",
        title: "",
        lastError: null,
      } as unknown as IAgentState["activity"],
    });

    const html = renderToStaticMarkup(<AttentionBanner agents={[emptyWaiting, emptyError]} onSelectAgent={() => {}} />);

    expect(html).toContain("Untitled Agent");
    expect(html).toContain("Waiting on user question input");
    expect(html).toContain("Agent encountered an execution error");
  });

  it("invokes onSelectAgent callback when an attention agent card is clicked", () => {
    let selectedAgent: IAgentState | null = null;
    const onSelectAgent = (agent: IAgentState) => {
      selectedAgent = agent;
    };

    const element = AttentionBanner({
      agents: [fixture_agentStateWaitingQuestion],
      onSelectAgent,
    });

    assert(element !== null);
    const card = element.props.children[1].props.children[0];
    card.props.onClick();

    assert(selectedAgent);
    expect<IAgentState | null>(selectedAgent).toBe(fixture_agentStateWaitingQuestion);
  });
});
