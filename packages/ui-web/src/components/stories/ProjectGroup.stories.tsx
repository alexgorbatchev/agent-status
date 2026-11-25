import React from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { ProjectGroup } from "../ProjectGroup";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion, fixture_agentStateWorktree } from "./fixtures";

const meta: Meta<typeof ProjectGroup> = {
  component: ProjectGroup,
  title: "@agent-status/ui-web/components/ProjectGroup",
};

type Story = StoryObj<typeof ProjectGroup>;

const onSelectAgent = fn();

export const Default: Story = {
  args: {
    projectName: "agent-status",
    projectId: "github.com/alexgorbatchev/agent-status",
    agents: [fixture_agentStateRunning, fixture_agentStateWaitingQuestion],
    onSelectAgent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const group = canvas.getByTestId("ProjectGroup");
    await expect(group).toBeInTheDocument();
    await expect(canvas.getByTestId("ProjectGroup--title")).toHaveTextContent("agent-status");

    // Exercise clicking an agent card within the project group
    const agentCards = canvas.getAllByTestId("AgentCard");
    await expect(agentCards).toHaveLength(2);

    await userEvent.click(agentCards[0]);
    await expect(onSelectAgent).toHaveBeenCalledWith(fixture_agentStateRunning);
  },
};

export const WithWorktrees: Story = {
  args: {
    projectName: "agent-status",
    projectId: "github.com/alexgorbatchev/agent-status",
    agents: [fixture_agentStateRunning, fixture_agentStateWorktree],
    onSelectAgent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const group = canvas.getByTestId("ProjectGroup");
    await expect(group).toBeInTheDocument();
    await expect(canvas.getByTestId("ProjectGroup--title")).toHaveTextContent("agent-status");

    // Verify root branch and worktree branch headers
    const rootBranch = canvas.getByTestId("ProjectGroup--root-branch");
    await expect(rootBranch).toBeInTheDocument();
    await expect(rootBranch).toHaveTextContent("[main]");

    const worktreeSection = canvas.getByTestId("ProjectGroup--worktree-section");
    await expect(worktreeSection).toBeInTheDocument();
    await expect(canvas.getByTestId("ProjectGroup--worktree-header")).toHaveTextContent("[worktree: feat-auth]");

    // Two agent cards rendered
    const agentCards = canvas.getAllByTestId("AgentCard");
    await expect(agentCards).toHaveLength(2);
  },
};

export default meta;
