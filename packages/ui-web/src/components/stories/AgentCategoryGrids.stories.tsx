import type { Meta, StoryObj } from "@storybook/react";
import { fn, expect, within } from "@storybook/test";
import { AgentCategoryGrids } from "../AgentCategoryGrids";
import { fixture_agentStateRunning, fixture_agentStateIdle, fixture_agentStateExited } from "./fixtures";

const meta: Meta<typeof AgentCategoryGrids> = {
  component: AgentCategoryGrids,
  title: "@agent-status/ui-web/components/AgentCategoryGrids",
};

type Story = StoryObj<typeof AgentCategoryGrids>;

const onSelectAgent = fn();

const Default: Story = {
  args: {
    agents: [fixture_agentStateRunning, fixture_agentStateIdle, fixture_agentStateExited],
    onSelectAgent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const grids = canvas.getByTestId("AgentCategoryGrids");
    await expect(grids).toBeInTheDocument();
    const busyGrid = canvas.getByTestId("AgentCategoryGrids-busy");
    const idleGrid = canvas.getByTestId("AgentCategoryGrids-idle");
    const finishedGrid = canvas.getByTestId("AgentCategoryGrids-finished");
    await expect(busyGrid).toBeInTheDocument();
    await expect(idleGrid).toBeInTheDocument();
    await expect(finishedGrid).toBeInTheDocument();
  },
};

export default meta;
export { Default as AgentCategoryGrids };
