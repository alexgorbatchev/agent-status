import React from "react";
import { describe, it, expect } from "bun:test";
import { renderToString } from "react-dom/server";
import { ProjectGroup } from "../components/ProjectGroup";
import { fixture_agentStateRunning, fixture_agentStateExited } from "./fixtures";
import type { IAgentState } from "../types";

describe("ProjectGroup Component", () => {
  it("renders project header with title, subtitle, spend, and active count without worktree sections when flat", () => {
    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[fixture_agentStateRunning, fixture_agentStateExited]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="ProjectGroup"');
    expect(html).toContain('data-testid="ProjectGroup--header"');
    expect(html).toContain('data-testid="ProjectGroup--title"');
    expect(html).toContain("agent-status");
    expect(html).toContain('data-testid="ProjectGroup--subtitle"');
    expect(html).toContain("(github.com/alexgorbatchev/agent-status)");
    expect(html).toContain('data-testid="ProjectGroup--spend"');
    expect(html).toContain("1 active");
    expect(html).not.toContain('data-testid="ProjectGroup--worktree-section"');
  });

  it("renders tree-structured branches when linked worktrees are present", () => {
    const rootAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "root-agent",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: false,
        branch: "main",
      },
    };

    const worktreeAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "wt-agent",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: true,
        branch: "feat-auth",
        relPath: ".workspaces/feat-auth",
        cwd: "/repo/.workspaces/feat-auth",
      },
    };

    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[rootAgent, worktreeAgent]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="ProjectGroup"');
    expect(html).toContain('data-testid="ProjectGroup--root-branch"');
    expect(html).toContain('<h3 data-testid="ProjectGroup--root-branch"');
    expect(html).toContain("[main]");
    expect(html).toContain("1 session");
    expect(html).toContain('aria-hidden="true">├─</span>');

    expect(html).toContain('data-testid="ProjectGroup--worktree-section"');
    expect(html).toContain('<h3 data-testid="ProjectGroup--worktree-header"');
    expect(html).toContain("[worktree: feat-auth]");
    expect(html).toContain("(.workspaces/feat-auth)");
    expect(html).toContain('aria-hidden="true">└─</span>');
  });

  it("falls back to [HEAD] when root branch is empty", () => {
    const rootAgentWithoutBranch: IAgentState = {
      ...fixture_agentStateRunning,
      id: "root-no-branch",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: false,
        branch: "",
      },
    };

    const worktreeAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "wt-agent",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: true,
        branch: "feat-auth",
        relPath: ".workspaces/feat-auth",
      },
    };

    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[rootAgentWithoutBranch, worktreeAgent]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain("[HEAD]");
    expect(html).not.toContain("[main]");
  });

  it("renders multiple worktree branches with correct tree branch glyphs", () => {
    const rootAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "root-agent",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: false,
        branch: "main",
      },
    };

    const wtAgent1: IAgentState = {
      ...fixture_agentStateRunning,
      id: "wt-1",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: true,
        branch: "feat-auth",
        relPath: ".workspaces/feat-auth",
      },
    };

    const wtAgent2: IAgentState = {
      ...fixture_agentStateRunning,
      id: "wt-2",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: true,
        branch: "feat-ui",
        relPath: ".workspaces/feat-ui",
      },
    };

    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[rootAgent, wtAgent1, wtAgent2]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="ProjectGroup--root-branch"');
    const wtSections = html.split('data-testid="ProjectGroup--worktree-section"');
    expect(wtSections.length).toBe(3);

    // wt1 is not last, so it gets ├─
    // wt2 is last, so it gets └─
    expect(wtSections[1]).toContain("├─");
    expect(wtSections[1]).toContain("[worktree: feat-auth]");

    expect(wtSections[2]).toContain("└─");
    expect(wtSections[2]).toContain("[worktree: feat-ui]");
  });

  it("renders worktree branches when no root agents exist", () => {
    const wtAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "wt-only",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: true,
        branch: "feat-auth",
        relPath: ".workspaces/feat-auth",
      },
    };

    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[wtAgent]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).not.toContain('data-testid="ProjectGroup--root-branch"');
    expect(html).toContain('data-testid="ProjectGroup--worktree-section"');
    expect(html).toContain("[worktree: feat-auth]");
    expect(html).toContain("└─");
  });

  it("handles empty agents list gracefully", () => {
    const html = renderToString(
      <ProjectGroup
        projectName="empty-project"
        projectId="github.com/alexgorbatchev/empty"
        agents={[]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="ProjectGroup"');
    expect(html).toContain('data-testid="ProjectGroup--title"');
    expect(html).toContain("empty-project");
    expect(html).toContain("$0.000");
    expect(html).toContain("0 active");
  });

  it("scopes category grids uniquely to their parent section", () => {
    const rootAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "root-agent",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: false,
        branch: "main",
      },
    };

    const worktreeAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "wt-agent",
      workspace: {
        ...fixture_agentStateRunning.workspace,
        isWorktree: true,
        branch: "feat-auth",
        relPath: ".workspaces/feat-auth",
        cwd: "/repo/.workspaces/feat-auth",
      },
    };

    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[rootAgent, worktreeAgent]}
        onSelectAgent={() => {}}
      />,
    );

    // Root section category grid should be scoped to ProjectGroup--root-grid-busy
    expect(html).toContain('data-testid="ProjectGroup--root-grid-busy"');
    // Worktree section category grid should be scoped to ProjectGroup--worktree-grid-busy
    expect(html).toContain('data-testid="ProjectGroup--worktree-grid-busy"');
    // The unscoped ProjectGroup--grid-busy must not appear in hierarchical mode
    expect(html).not.toContain('data-testid="ProjectGroup--grid-busy"');
  });

  it("does not render agents from another project when projectId does not match", () => {
    const html = renderToString(
      <ProjectGroup
        projectName="other-project"
        projectId="github.com/alexgorbatchev/other-project"
        agents={[fixture_agentStateRunning]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="ProjectGroup"');
    expect(html).toContain('data-testid="ProjectGroup--title"');
    expect(html).toContain("other-project");
    expect(html).toContain("0 active");
    expect(html).not.toContain(fixture_agentStateRunning.activity.title);
  });
});
