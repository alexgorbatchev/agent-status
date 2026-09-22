import { describe, it, expect } from "bun:test";
import React from "react";
import { AgentCard } from "../components/AgentCard";
import { formatA11yTree } from "../utils/a11ySnapshot";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_agentStateExited,
  factory_agentState,
} from "./fixtures";

const noop = (): void => {};

describe("AgentCard Semantic Accessibility Snapshots", () => {
  it("matches accessibility snapshot for RUNNING (busy) agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const agent = factory_agentState({
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(<AgentCard agent={agent} onToggleHide={noop} />, mockDoc);
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Card: "Build agent-status dashboard"',
          "  ├─ Badge: (Claude Code)",
          "  ├─ Button: [Hide agent session]",
          "  ├─ Button: [Focus in Herdr]",
          "  ├─ Status: (RUNNING)",
          '  ├─ Heading: "Build agent-status dashboard"',
          '  ├─ Action: "Compiling typescript and verifying bundles"',
          "  ├─ Badge: (work-macbook)",
          "  ├─ Badge: (branch: main)",
          "  ├─ Badge: (2 subagents)",
          "  ├─ Badge: (silent: 2m)",
          '  ├─ Cost: "$0.0560"',
          '  └─ Usage: "7,300 tokens"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for WAITING_QUESTION agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const agent = factory_agentState({
        ...fixture_agentStateWaitingQuestion,
        activity: {
          ...fixture_agentStateWaitingQuestion.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(<AgentCard agent={agent} onToggleHide={noop} />, mockDoc);
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Card: "Implement user authentication flow"',
          "  ├─ Badge: (Claude Code)",
          "  ├─ Button: [Hide agent session]",
          "  ├─ Button: [Focus in Herdr]",
          "  ├─ Status: (RUNNING)",
          '  ├─ Heading: "Implement user authentication flow"',
          '  ├─ Action: "Waiting for user input on database choice"',
          '  ├─ Callout (warning): "Should we use SQLite or PostgreSQL for auth tokens?"',
          "  ├─ Badge: (work-macbook)",
          "  ├─ Badge: (branch: feat/auth)",
          "  ├─ Badge: (worktree: .workspaces/feat-auth)",
          "  ├─ Badge: (2m ago)",
          '  ├─ Cost: "$0.0210"',
          '  └─ Usage: "3,150 tokens"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for ERROR agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const agent = factory_agentState({
        ...fixture_agentStateError,
        activity: {
          ...fixture_agentStateError.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(<AgentCard agent={agent} onToggleHide={noop} />, mockDoc);
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Card: "Database schema migration"',
          "  ├─ Badge: (Claude Code)",
          "  ├─ Button: [Hide agent session]",
          "  ├─ Button: [Focus in Herdr]",
          "  ├─ Status: (RUNNING)",
          '  ├─ Heading: "Database schema migration"',
          "  ├─ Action: \"Failed to apply migration: syntax error near 'INDEX'\"",
          "  ├─ Callout (error): \"Database migration returned exit code 1: syntax error near 'INDEX'\"",
          "  ├─ Badge: (work-macbook)",
          "  ├─ Badge: (branch: main)",
          "  ├─ Badge: (1 subagent)",
          "  ├─ Badge: (2m ago)",
          '  ├─ Cost: "$0.0380"',
          '  └─ Usage: "4,830 tokens"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for IDLE agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
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
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(<AgentCard agent={idleAgent} onToggleHide={noop} />, mockDoc);
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Card: "Code review agent"',
          "  ├─ Badge: (Claude Code)",
          "  ├─ Button: [Hide agent session]",
          "  ├─ Button: [Focus in Herdr]",
          "  ├─ Status: (RUNNING)",
          '  ├─ Heading: "Code review agent"',
          '  ├─ Action: "Idle"',
          "  ├─ Badge: (work-macbook)",
          "  ├─ Badge: (branch: main)",
          "  ├─ Badge: (2m ago)",
          '  ├─ Cost: "$0.0030"',
          '  └─ Usage: "600 tokens"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for HIDDEN agent with [Unhide] button and (Hidden) badge", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
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
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(
        <AgentCard agent={hiddenAgent} onToggleHide={noop} />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Card: "Background worker"',
          "  ├─ Badge: (Claude Code)",
          "  ├─ Button: [Unhide agent session]",
          "  ├─ Button: [Focus in Herdr]",
          "  ├─ Badge: (Hidden)",
          "  ├─ Status: (RUNNING)",
          '  ├─ Heading: "Background worker"',
          '  ├─ Action: "Running tests"',
          "  ├─ Badge: (work-macbook)",
          "  ├─ Badge: (branch: main)",
          "  ├─ Badge: (silent: 2m)",
          '  ├─ Cost: "$0.0050"',
          '  └─ Usage: "1,200 tokens"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for EXITED agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const exitedAgent = factory_agentState({
        ...fixture_agentStateExited,
        activity: {
          ...fixture_agentStateExited.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(
        <AgentCard agent={exitedAgent} onToggleHide={noop} />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Card: "Refactor telemetry pipeline"',
          "  ├─ Badge: (Pi)",
          "  ├─ Button: [Hide agent session]",
          "  ├─ Button: [Focus in Herdr]",
          "  ├─ Status: (EXITED)",
          '  ├─ Heading: "Refactor telemetry pipeline"',
          '  ├─ Action: "Completed task successfully"',
          "  ├─ Badge: (work-macbook)",
          "  ├─ Badge: (branch: main)",
          "  ├─ Badge: (2m ago)",
          '  ├─ Cost: "$0.0920"',
          '  └─ Usage: "12,100 tokens"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for agent with active subagent chips", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const tool = "bash";
      const agentWithSubagents = factory_agentState({
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          subagentCount: 1,
          activeSubagents: [
            {
              id: "sub-worker-1",
              activity: "BUSY",
              activeTool: tool,
              action: "Running bash",
              lastError: null,
            },
          ],
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(
        <AgentCard agent={agentWithSubagents} onToggleHide={noop} />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toContain("Badge: (sub-worker-1: bash)");
    } finally {
      cleanup();
    }
  });
});
