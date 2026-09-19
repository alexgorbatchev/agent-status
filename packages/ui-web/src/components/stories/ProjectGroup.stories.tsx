import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { ProjectGroup } from "../ProjectGroup";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion } from "./fixtures";

const meta: Meta<typeof ProjectGroup> = {
  component: ProjectGroup,
  title: "@agent-status/ui-web/components/ProjectGroup",
};

type Story = StoryObj<typeof ProjectGroup>;

const onSelectAgent = fn();

const Default: Story = {
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

export default meta;
export { Default as ProjectGroup };
