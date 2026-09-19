import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { LogDrawer } from "../LogDrawer";
import { fixture_agentStateRunning } from "./fixtures";

const meta: Meta<typeof LogDrawer> = {
  component: LogDrawer,
  title: "@agent-status/ui-web/components/LogDrawer",
};

type Story = StoryObj<typeof LogDrawer>;

const onClose = fn();

const Default: Story = {
  args: {
    agent: fixture_agentStateRunning,
    onClose,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const drawer = canvas.getByTestId("LogDrawer");
    await expect(drawer).toBeInTheDocument();
    await expect(canvas.getByTestId("LogDrawer--title")).toHaveTextContent("Build agent-status dashboard");

    // Exercise clicking close button on drawer
    const closeBtn = canvas.getByTestId("LogDrawer--close-button");
    await userEvent.click(closeBtn);
    await expect(onClose).toHaveBeenCalledTimes(1);
  },
};

export default meta;
export { Default as LogDrawer };
