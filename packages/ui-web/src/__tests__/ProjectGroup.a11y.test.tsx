import { describe, it, expect } from "bun:test";
import React from "react";
import { ProjectGroup } from "../components/ProjectGroup";
import { formatA11yTree } from "../utils/a11ySnapshot";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import { fixture_agentStateRunning, factory_agentState } from "./fixtures";
import type { IAgentState } from "../types";

const noop = (): void => {};

describe("ProjectGroup Semantic Accessibility Snapshots", () => {
  it("matches accessibility snapshot for hierarchical project with worktrees", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const rootAgent: IAgentState = factory_agentState({
        ...fixture_agentStateRunning,
        id: "root-agent",
        workspace: {
          ...fixture_agentStateRunning.workspace,
          isWorktree: false,
          branch: "main",
        },
        activity: {
          ...fixture_agentStateRunning.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const wtAgent: IAgentState = factory_agentState({
        ...fixture_agentStateRunning,
        id: "wt-agent",
        workspace: {
          ...fixture_agentStateRunning.workspace,
          isWorktree: true,
          branch: "feat-auth",
          relPath: ".workspaces/feat-auth",
        },
        activity: {
          ...fixture_agentStateRunning.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(
        <ProjectGroup
          projectName="agent-status"
          projectId="github.com/alexgorbatchev/agent-status"
          agents={[rootAgent, wtAgent]}
          onSelectAgent={noop}
        />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toContain('Heading: "agent-status"');
      expect(tree).toContain('Heading: "[main] (1 session)"');
      expect(tree).toContain('Heading: "[worktree: feat-auth] (.workspaces/feat-auth) (1 session)"');
      expect(tree).not.toContain('Heading: "├─');
      expect(tree).not.toContain('Heading: "└─');
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for flat project without worktrees", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const rootAgent: IAgentState = factory_agentState({
        ...fixture_agentStateRunning,
        id: "root-agent",
        workspace: {
          ...fixture_agentStateRunning.workspace,
          isWorktree: false,
          branch: "main",
        },
        activity: {
          ...fixture_agentStateRunning.activity,
          updatedAt: Date.now() - 120000,
        },
      });

      const { container, unmount } = await mountComponent(
        <ProjectGroup
          projectName="agent-status"
          projectId="github.com/alexgorbatchev/agent-status"
          agents={[rootAgent]}
          onSelectAgent={noop}
        />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toContain('Heading: "agent-status"');
      expect(tree).not.toContain('Heading: "[main] (1 session)"');
    } finally {
      cleanup();
    }
  });
});
