import React from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { AgentGroupSection } from "../AgentGroupSection";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateIdle,
  fixture_agentStateExited,
} from "./fixtures";
import { FolderGit2 } from "lucide-react";

const meta: Meta<typeof AgentGroupSection> = {
  component: AgentGroupSection,
  title: "@agent-status/ui-web/components/AgentGroupSection",
};

type Story = StoryObj<typeof AgentGroupSection>;

const onSelectAgent = fn();

export const Default: Story = {
  args: {
    testId: "AgentGroupSection",
    icon: <FolderGit2 size={16} />,
    iconColorClass: "text-blue-400",
    title: "agent-status",
    subtitle: "(github.com/alexgorbatchev/agent-status)",
    agents: [fixture_agentStateRunning, fixture_agentStateWaitingQuestion],
    onSelectAgent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const group = canvas.getByTestId("AgentGroupSection");
    await expect(group).toBeInTheDocument();
    await expect(canvas.getByTestId("AgentGroupSection--title")).toHaveTextContent("agent-status");

    // Exercise clicking an agent card within the agent group section
    const agentCards = canvas.getAllByTestId("AgentCard");
    await expect(agentCards).toHaveLength(2);

    await userEvent.click(agentCards[0]);
    await expect(onSelectAgent).toHaveBeenCalledWith(fixture_agentStateRunning);
  },
};

export const MixedCategories: Story = {
  args: {
    testId: "AgentGroupSection",
    icon: <FolderGit2 size={16} />,
    iconColorClass: "text-blue-400",
    title: "mixed-project",
    subtitle: "(github.com/alexgorbatchev/mixed-project)",
    agents: [fixture_agentStateRunning, fixture_agentStateIdle, fixture_agentStateExited],
    onSelectAgent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const busyGrid = canvas.getByTestId("AgentGroupSection--grid-busy");
    const idleGrid = canvas.getByTestId("AgentGroupSection--grid-idle");
    const finishedGrid = canvas.getByTestId("AgentGroupSection--grid-finished");
    await expect(busyGrid).toBeInTheDocument();
    await expect(idleGrid).toBeInTheDocument();
    await expect(finishedGrid).toBeInTheDocument();
  },
};

export default meta;
