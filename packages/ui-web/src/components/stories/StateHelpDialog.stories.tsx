import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { StateHelpDialog } from "../StateHelpDialog";

const meta: Meta<typeof StateHelpDialog> = {
  component: StateHelpDialog,
  title: "@agent-status/ui-web/components/StateHelpDialog",
};

export default meta;

type Story = StoryObj<typeof StateHelpDialog>;

const onClose = fn();

const Default: Story = {
  args: {
    onClose,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const dialog = canvas.getByTestId("StateHelpDialog");
    await expect(dialog).toBeInTheDocument();
    await expect(canvas.getByTestId("StateHelpDialog--title")).toHaveTextContent("Agent States & Lifecycle Guide");
    await expect(canvas.getByTestId("StateHelpDialog--got-it-button")).toBeInTheDocument();

    const closeBtn = canvas.getByTestId("StateHelpDialog--close-button");
    await userEvent.click(closeBtn);
    await expect(onClose).toHaveBeenCalled();
  },
};

export { Default as StateHelpDialog };
