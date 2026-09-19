import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { EnvironmentGroup } from "../EnvironmentGroup";
import { fixture_environmentInfo, fixture_agentStateRunning } from "./fixtures";

const meta: Meta<typeof EnvironmentGroup> = {
  component: EnvironmentGroup,
  title: "@agent-status/ui-web/components/EnvironmentGroup",
};

type Story = StoryObj<typeof EnvironmentGroup>;

const onSelectAgent = fn();

const Default: Story = {
  args: {
    environment: fixture_environmentInfo,
    agents: [fixture_agentStateRunning],
    onSelectAgent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const group = canvas.getByTestId("EnvironmentGroup");
    await expect(group).toBeInTheDocument();
    await expect(canvas.getByTestId("EnvironmentGroup--title")).toHaveTextContent("work-macbook");

    // Exercise user interaction: selecting an agent card inside this environment group
    const card = canvas.getByTestId("AgentCard");
    await userEvent.click(card);
    await expect(onSelectAgent).toHaveBeenCalledWith(fixture_agentStateRunning);
  },
};

export default meta;
export { Default as EnvironmentGroup };
