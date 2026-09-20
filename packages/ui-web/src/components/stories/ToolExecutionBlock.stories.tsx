import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { ToolExecutionBlock } from "../ToolExecutionBlock";

const meta: Meta<typeof ToolExecutionBlock> = {
  component: ToolExecutionBlock,
  title: "@agent-status/ui-web/components/ToolExecutionBlock",
};

export default meta;

type Story = StoryObj<typeof ToolExecutionBlock>;

export const Default: Story = {
  args: {
    toolName: "Bash",
    toolInput: { command: "bun test" },
    toolOutput: "1 pass, 0 fail",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const block = canvas.getByTestId("ToolExecutionBlock");
    await expect(block).toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--name")).toHaveTextContent("Bash");
    await expect(canvas.getByTestId("ToolExecutionBlock--input")).toHaveTextContent("bun test");
    await expect(canvas.getByTestId("ToolExecutionBlock--output")).toHaveTextContent("1 pass, 0 fail");
  },
};

export const WithoutInputOrOutput: Story = {
  args: {
    toolName: "ReadFile",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const block = canvas.getByTestId("ToolExecutionBlock");
    await expect(block).toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--name")).toHaveTextContent("ReadFile");
    await expect(canvas.queryByTestId("ToolExecutionBlock--input")).not.toBeInTheDocument();
    await expect(canvas.queryByTestId("ToolExecutionBlock--output")).not.toBeInTheDocument();
  },
};
