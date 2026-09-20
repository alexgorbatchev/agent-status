import React from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { IconWrapper } from "../IconWrapper";
import { Bot } from "lucide-react";

const meta: Meta<typeof IconWrapper> = {
  component: IconWrapper,
  title: "@agent-status/ui-web/components/IconWrapper",
};

type Story = StoryObj<typeof IconWrapper>;

const Default: Story = {
  args: {
    size: "md",
    tone: "primary",
    children: <Bot size={16} />,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const wrapper = canvas.getByTestId("IconWrapper");
    await expect(wrapper).toBeInTheDocument();
    await expect(wrapper).toHaveClass("w-4");
    await expect(wrapper).toHaveClass("h-4");
    await expect(wrapper).toHaveClass("text-blue-400");
  },
};

export default meta;
export { Default as IconWrapper };
