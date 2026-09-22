import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { AgentCard } from "../AgentCard";
import { fixture_agentStateRunning, fixture_agentStateExited } from "./fixtures";

const meta: Meta<typeof AgentCard> = {
  component: AgentCard,
  title: "@agent-status/ui-web/components/AgentCard",
};

export default meta;

type Story = StoryObj<typeof AgentCard>;

const onOpenDrawer = fn();

export const Default: Story = {
  args: {
    agent: fixture_agentStateRunning,
    onOpenDrawer,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const card = canvas.getByTestId("AgentCard");
    await expect(card).toBeInTheDocument();
    await expect(canvas.getByTestId("AgentCard--title")).toHaveTextContent("Build agent-status dashboard");
    await expect(canvas.getByTestId("AgentCard--last-active")).toBeInTheDocument();
    await expect(card).toHaveAttribute("role", "button");
    await expect(card).toHaveAttribute("tabindex", "0");
    await expect(card).toHaveAttribute("aria-label", "View details for Build agent-status dashboard");

    // Exercise user interaction: clicking the card triggers onOpenDrawer with agent state
    await userEvent.click(card);
    await expect(onOpenDrawer).toHaveBeenCalledTimes(1);
    await expect(onOpenDrawer).toHaveBeenCalledWith(fixture_agentStateRunning);
  },
};

export const Exited: Story = {
  args: {
    agent: fixture_agentStateExited,
    onOpenDrawer,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const card = canvas.getByTestId("AgentCard");
    await expect(card).toBeInTheDocument();
    await expect(canvas.getByTestId("AgentCard--title")).toHaveTextContent("Refactor telemetry pipeline");
    await expect(card).toHaveClass("opacity-60");
  },
};

export const StallNotice: Story = {
  args: {
    agent: {
      ...fixture_agentStateRunning,
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
        activeTool: "bash",
        activeToolStartedAt: Date.now() - 75000,
      },
    },
    onOpenDrawer,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const card = canvas.getByTestId("AgentCard");
    await expect(card).toBeInTheDocument();
    await expect(canvas.getByTestId("AgentCard--stall-notice")).toBeInTheDocument();
  },
};

export const StallWarning: Story = {
  args: {
    agent: {
      ...fixture_agentStateRunning,
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
        activeTool: "bash",
        action: "git rebase main",
        activeToolStartedAt: Date.now() - 135000,
      },
    },
    onOpenDrawer,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const card = canvas.getByTestId("AgentCard");
    await expect(card).toBeInTheDocument();
    await expect(canvas.getByTestId("AgentCard--stall-warning")).toBeInTheDocument();
    await expect(canvas.getByTestId("AgentCard--stall-duration")).toBeInTheDocument();
  },
};

export const StallAttention: Story = {
  args: {
    agent: {
      ...fixture_agentStateRunning,
      activity: {
        ...fixture_agentStateRunning.activity,
        lifecycle: "RUNNING",
        activity: "BUSY",
        activeTool: "bash",
        action: "npm test",
        activeToolStartedAt: Date.now() - 200000,
      },
    },
    onOpenDrawer,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const card = canvas.getByTestId("AgentCard");
    await expect(card).toBeInTheDocument();
    await expect(card).toHaveClass("border-amber-500/80");
    await expect(canvas.getByTestId("AgentCard--stall-warning")).toBeInTheDocument();
  },
};
