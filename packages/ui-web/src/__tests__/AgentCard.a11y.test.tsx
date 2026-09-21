import { describe, it, expect } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentCard } from "../components/AgentCard";
import { formatA11yTree } from "../utils/a11ySnapshot";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_agentStateExited,
  factory_agentState,
} from "./fixtures";

const noop = (): void => {};

describe("AgentCard Semantic Accessibility Snapshots", () => {
  it("matches accessibility snapshot for RUNNING (busy) agent", () => {
    const markup = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} onToggleHide={noop} />);
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Card: "Build agent-status dashboard"',
        "  ├─ Badge: (work-macbook)",
        "  ├─ Badge: (Claude Code)",
        "  ├─ Status: (RUNNING)",
        "  ├─ Button: [Hide agent session]",
        '  ├─ Heading: "Build agent-status dashboard"',
        '  ├─ Action: "Compiling typescript and verifying bundles"',
        "  ├─ Badge: (branch: main)",
        "  ├─ Badge: (2 subagents)",
        '  ├─ Cost: "$0.0560"',
        '  └─ Usage: "7,300 tokens"',
      ].join("\n"),
    );
  });

  it("matches accessibility snapshot for WAITING_QUESTION agent", () => {
    const markup = renderToStaticMarkup(<AgentCard agent={fixture_agentStateWaitingQuestion} onToggleHide={noop} />);
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Card: "Implement user authentication flow"',
        "  ├─ Badge: (work-macbook)",
        "  ├─ Badge: (Claude Code)",
        "  ├─ Status: (RUNNING)",
        "  ├─ Button: [Hide agent session]",
        '  ├─ Heading: "Implement user authentication flow"',
        '  ├─ Action: "Waiting for user input on database choice"',
        '  ├─ Callout (warning): "Should we use SQLite or PostgreSQL for auth tokens?"',
        "  ├─ Badge: (branch: feat/auth)",
        "  ├─ Badge: (worktree: .workspaces/feat-auth)",
        '  ├─ Cost: "$0.0210"',
        '  └─ Usage: "3,150 tokens"',
      ].join("\n"),
    );
  });

  it("matches accessibility snapshot for ERROR agent", () => {
    const markup = renderToStaticMarkup(<AgentCard agent={fixture_agentStateError} onToggleHide={noop} />);
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Card: "Database schema migration"',
        "  ├─ Badge: (work-macbook)",
        "  ├─ Badge: (Claude Code)",
        "  ├─ Status: (RUNNING)",
        "  ├─ Button: [Hide agent session]",
        '  ├─ Heading: "Database schema migration"',
        "  ├─ Action: \"Failed to apply migration: syntax error near 'INDEX'\"",
        "  ├─ Callout (error): \"Database migration returned exit code 1: syntax error near 'INDEX'\"",
        "  ├─ Badge: (branch: main)",
        "  ├─ Badge: (1 subagent)",
        '  ├─ Cost: "$0.0380"',
        '  └─ Usage: "4,830 tokens"',
      ].join("\n"),
    );
  });

  it("matches accessibility snapshot for IDLE agent", () => {
    const idleAgent = factory_agentState({
      id: "agent-idle",
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        title: "Code review agent",
        action: "Idle",
        subagentCount: 0,
        usage: { input: 500, output: 100, cacheRead: 0, cacheWrite: 0, reasoning: 0, total: 600 },
        cost: { currency: "USD", total: 0.003, isEstimated: false },
      },
    });

    const markup = renderToStaticMarkup(<AgentCard agent={idleAgent} onToggleHide={noop} />);
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Card: "Code review agent"',
        "  ├─ Badge: (work-macbook)",
        "  ├─ Badge: (Claude Code)",
        "  ├─ Status: (RUNNING)",
        "  ├─ Button: [Hide agent session]",
        '  ├─ Heading: "Code review agent"',
        '  ├─ Action: "Idle"',
        "  ├─ Badge: (branch: main)",
        '  ├─ Cost: "$0.0030"',
        '  └─ Usage: "600 tokens"',
      ].join("\n"),
    );
  });

  it("matches accessibility snapshot for HIDDEN agent with [Unhide] button and (Hidden) badge", () => {
    const hiddenAgent = factory_agentState({
      id: "agent-hidden",
      isHidden: true,
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
        title: "Background worker",
        action: "Running tests",
        subagentCount: 0,
        usage: { input: 1000, output: 200, cacheRead: 0, cacheWrite: 0, reasoning: 0, total: 1200 },
        cost: { currency: "USD", total: 0.005, isEstimated: false },
      },
    });

    const markup = renderToStaticMarkup(<AgentCard agent={hiddenAgent} onToggleHide={noop} />);
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Card: "Background worker"',
        "  ├─ Badge: (work-macbook)",
        "  ├─ Badge: (Claude Code)",
        "  ├─ Status: (RUNNING)",
        "  ├─ Badge: (Hidden)",
        "  ├─ Button: [Unhide agent session]",
        '  ├─ Heading: "Background worker"',
        '  ├─ Action: "Running tests"',
        "  ├─ Badge: (branch: main)",
        '  ├─ Cost: "$0.0050"',
        '  └─ Usage: "1,200 tokens"',
      ].join("\n"),
    );
  });

  it("matches accessibility snapshot for EXITED agent", () => {
    const markup = renderToStaticMarkup(<AgentCard agent={fixture_agentStateExited} onToggleHide={noop} />);
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Card: "Refactor telemetry pipeline"',
        "  ├─ Badge: (work-macbook)",
        "  ├─ Badge: (Pi)",
        "  ├─ Status: (EXITED)",
        "  ├─ Button: [Hide agent session]",
        '  ├─ Heading: "Refactor telemetry pipeline"',
        '  ├─ Action: "Completed task successfully"',
        "  ├─ Badge: (branch: main)",
        '  ├─ Cost: "$0.0920"',
        '  └─ Usage: "12,100 tokens"',
      ].join("\n"),
    );
  });
});
