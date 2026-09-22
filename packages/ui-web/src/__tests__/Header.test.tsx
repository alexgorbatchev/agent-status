import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { Header } from "../components/Header";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_environmentInfo,
} from "./fixtures";
import type { IAgentState } from "../types";

describe("Header Component", () => {
  it("renders brand, active agents, and environment metrics", () => {
    const html = renderToStaticMarkup(
      <Header
        agents={[fixture_agentStateRunning]}
        environments={[fixture_environmentInfo]}
        isConnected={true}
        onOpenHelp={() => {}}
      />,
    );

    expect(html).toContain('data-testid="Header"');
    expect(html).toContain("Agent Status");
    expect(html).toContain('data-testid="Header--active-agents"');
    expect(html).toContain("Active:");
    expect(html).not.toContain('data-testid="Header--attention-agents"');
  });

  it("renders Needs Attention counter when agents are in WAITING_QUESTION or ERROR", () => {
    const html = renderToStaticMarkup(
      <Header
        agents={[fixture_agentStateRunning, fixture_agentStateWaitingQuestion, fixture_agentStateError]}
        environments={[fixture_environmentInfo]}
        isConnected={true}
        onOpenHelp={() => {}}
      />,
    );

    expect(html).toContain('data-testid="Header--attention-agents"');
    expect(html).toContain("Needs Attention:");
    expect(html).toContain("2");
  });

  it("increments Needs Attention counter when an agent has a stalled tool (>=180s)", () => {
    const now = Date.now();
    const stalledAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "agent-stalled-header-test",
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
        activeTool: "bash",
        activeToolStartedAt: now - 200000,
      },
    };

    const html = renderToStaticMarkup(
      <Header
        agents={[fixture_agentStateRunning, stalledAgent]}
        environments={[fixture_environmentInfo]}
        isConnected={true}
        onOpenHelp={() => {}}
      />,
    );

    expect(html).toContain('data-testid="Header--attention-agents"');
    expect(html).toContain("Needs Attention:");
    expect(html).toContain("1");
  });

  it("renders active agents metric as an accessible button with aria-pressed and aria-label", () => {
    const htmlInactive = renderToStaticMarkup(
      <Header
        agents={[fixture_agentStateRunning]}
        environments={[fixture_environmentInfo]}
        isConnected={true}
        isActiveFilter={false}
        onToggleActiveFilter={() => {}}
      />,
    );

    expect(htmlInactive).toContain('data-testid="Header--active-agents"');
    expect(htmlInactive).toContain("<button");
    expect(htmlInactive).toContain('aria-pressed="false"');
    expect(htmlInactive).toContain('aria-label="Filter by active agents"');

    const htmlActive = renderToStaticMarkup(
      <Header
        agents={[fixture_agentStateRunning]}
        environments={[fixture_environmentInfo]}
        isConnected={true}
        isActiveFilter={true}
        onToggleActiveFilter={() => {}}
      />,
    );

    expect(htmlActive).toContain('aria-pressed="true"');
    expect(htmlActive).toContain('aria-label="Disable active agents filter"');
    expect(htmlActive).toContain("bg-blue-950/70");
    expect(htmlActive).toContain("text-blue-300");
  });
});
