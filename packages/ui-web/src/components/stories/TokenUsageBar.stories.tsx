import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { TokenUsageBar } from "../TokenUsageBar";
import { fixture_tokenUsage } from "./fixtures";

const meta: Meta<typeof TokenUsageBar> = {
  component: TokenUsageBar,
  title: "@agent-status/ui-web/components/TokenUsageBar",
};

export default meta;

type Story = StoryObj<typeof TokenUsageBar>;

export const Default: Story = {
  args: {
    usage: fixture_tokenUsage,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const bar = canvas.getByTestId("TokenUsageBar");
    await expect(bar).toBeInTheDocument();
    await expect(canvas.getByTestId("TokenUsageBar--input")).toHaveTextContent("In: 1,000");
    await expect(canvas.getByTestId("TokenUsageBar--output")).toHaveTextContent("Out: 200");
    await expect(canvas.getByTestId("TokenUsageBar--cache")).toHaveTextContent("Cache: 5,000");
    await expect(canvas.getByTestId("TokenUsageBar--total")).toHaveTextContent("Total: 7,300");
  },
};

export const WithoutCache: Story = {
  args: {
    usage: {
      input: 500,
      output: 100,
      cacheRead: 0,
      cacheWrite: 0,
      reasoning: 0,
      total: 600,
    },
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const bar = canvas.getByTestId("TokenUsageBar");
    await expect(bar).toBeInTheDocument();
    await expect(canvas.getByTestId("TokenUsageBar--input")).toHaveTextContent("In: 500");
    await expect(canvas.getByTestId("TokenUsageBar--output")).toHaveTextContent("Out: 100");
    await expect(canvas.queryByTestId("TokenUsageBar--cache")).not.toBeInTheDocument();
    await expect(canvas.getByTestId("TokenUsageBar--total")).toHaveTextContent("Total: 600");
  },
};
