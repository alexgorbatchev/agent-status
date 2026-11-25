import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { AnsiText } from "../AnsiText";

const meta: Meta<typeof AnsiText> = {
  component: AnsiText,
  title: "@agent-status/ui-web/components/AnsiText",
};

export default meta;

type Story = StoryObj<typeof AnsiText>;

export const Default: Story = {
  args: {
    text: "Plain terminal text without ANSI codes",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const element = canvas.getByTestId("AnsiText");
    await expect(element).toBeInTheDocument();
    await expect(element).toHaveTextContent("Plain terminal text without ANSI codes");
  },
};

export const ThinkingAnsiColors: Story = {
  args: {
    text: "\u001b[38;2;138;190;183mThinking:\u001b[39m \u001b[38;2;128;128;128mAnalyzing options\u001b[39m",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const element = canvas.getByTestId("AnsiText");
    await expect(element).toBeInTheDocument();
    await expect(element).toHaveTextContent("Thinking: Analyzing options");
  },
};

export const Standard16Colors: Story = {
  args: {
    text: "\u001b[1;32mSUCCESS\u001b[0m \u001b[31mFAILURE\u001b[0m",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const element = canvas.getByTestId("AnsiText");
    await expect(element).toBeInTheDocument();
    await expect(element).toHaveTextContent("SUCCESS FAILURE");
  },
};
