import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { StatusBadge } from "../StatusBadge";

const meta: Meta<typeof StatusBadge> = {
  component: StatusBadge,
  title: "@agent-status/ui-web/components/StatusBadge",
};

export default meta;

type Story = StoryObj<typeof StatusBadge>;

export const RunningBusy: Story = {
  args: {
    lifecycle: "RUNNING",
    activity: "BUSY",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const badge = canvas.getByTestId("StatusBadge");
    await expect(badge).toBeInTheDocument();
    await expect(canvas.getByText("Busy")).toBeInTheDocument();
  },
};

export const RunningWaitingQuestion: Story = {
  args: {
    lifecycle: "RUNNING",
    activity: "WAITING_QUESTION",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const badge = canvas.getByTestId("StatusBadge");
    await expect(badge).toBeInTheDocument();
    await expect(canvas.getByText("Waiting on Question")).toBeInTheDocument();
  },
};

export const RunningError: Story = {
  args: {
    lifecycle: "RUNNING",
    activity: "ERROR",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const badge = canvas.getByTestId("StatusBadge");
    await expect(badge).toBeInTheDocument();
    await expect(canvas.getByText("Error")).toBeInTheDocument();
  },
};

export const ExitedFinished: Story = {
  args: {
    lifecycle: "EXITED",
    activity: "IDLE",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const badge = canvas.getByTestId("StatusBadge");
    await expect(badge).toBeInTheDocument();
    await expect(canvas.getByText("Finished")).toBeInTheDocument();
  },
};
