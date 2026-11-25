import { describe, it, expect } from "bun:test";
import React from "react";
import { AgentGroupSection } from "../components/AgentGroupSection";
import { formatA11yTree } from "../utils/a11ySnapshot";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import { fixture_agentStateRunning, fixture_agentStateExited, factory_agentState } from "./fixtures";
import type { IAgentState } from "../types";
import { FolderGit2 } from "lucide-react";

const noop = (): void => {};

describe("AgentGroupSection Semantic Accessibility Snapshots", () => {
  it("matches accessibility snapshot for multi-category agent section", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const busyAgent = factory_agentState({
        ...fixture_agentStateRunning,
        activity: {
          ...fixture_agentStateRunning.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const idleAgent: IAgentState = factory_agentState({
        ...fixture_agentStateRunning,
        id: "idle-session",
        activity: {
          ...fixture_agentStateRunning.activity,
          activity: "IDLE",
          title: "Idle Worker",
          updatedAt: Date.now() - 120000,
        },
      });

      const finishedAgent = factory_agentState({
        ...fixture_agentStateExited,
        activity: {
          ...fixture_agentStateExited.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(
        <AgentGroupSection
          icon={<FolderGit2 size={16} />}
          title="test-project"
          subtitle="(github.com/alexgorbatchev/test-project)"
          agents={[busyAgent, idleAgent, finishedAgent]}
          onSelectAgent={noop}
        />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Banner: "Root"',
          '  ├─ Heading: "test-project"',
          '  ├─ Card: "Build agent-status dashboard"',
          "  │  ├─ Badge: (Claude Code)",
          "  │  ├─ Button: [Focus in Herdr]",
          "  │  ├─ Status: (RUNNING)",
          '  │  ├─ Heading: "Build agent-status dashboard"',
          '  │  ├─ Action: "Compiling typescript and verifying bundles"',
          "  │  ├─ Badge: (work-macbook)",
          "  │  ├─ Badge: (branch: main)",
          "  │  ├─ Badge: (2 subagents)",
          "  │  ├─ Badge: (silent: 2m)",
          '  │  ├─ Cost: "$0.0560"',
          '  │  └─ Usage: "7,300 tokens"',
          '  ├─ Card: "Idle Worker"',
          "  │  ├─ Badge: (Claude Code)",
          "  │  ├─ Button: [Focus in Herdr]",
          "  │  ├─ Status: (RUNNING)",
          '  │  ├─ Heading: "Idle Worker"',
          '  │  ├─ Action: "Compiling typescript and verifying bundles"',
          "  │  ├─ Badge: (work-macbook)",
          "  │  ├─ Badge: (branch: main)",
          "  │  ├─ Badge: (2 subagents)",
          "  │  ├─ Badge: (2m ago)",
          '  │  ├─ Cost: "$0.0560"',
          '  │  └─ Usage: "7,300 tokens"',
          '  └─ Card: "Refactor telemetry pipeline"',
          "     ├─ Badge: (Pi)",
          "     ├─ Button: [Focus in Herdr]",
          "     ├─ Status: (EXITED)",
          '     ├─ Heading: "Refactor telemetry pipeline"',
          '     ├─ Action: "Completed task successfully"',
          "     ├─ Badge: (work-macbook)",
          "     ├─ Badge: (branch: main)",
          "     ├─ Badge: (2m ago)",
          '     ├─ Cost: "$0.0920"',
          '     └─ Usage: "12,100 tokens"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });
});
