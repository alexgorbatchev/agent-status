import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { Header } from "../Header";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion, fixture_environmentInfo } from "./fixtures";

const meta: Meta<typeof Header> = {
  component: Header,
  title: "@agent-status/ui-web/components/Header",
};

export default meta;

type Story = StoryObj<typeof Header>;

export const ConnectedFleet: Story = {
  args: {
    agents: [fixture_agentStateRunning, fixture_agentStateWaitingQuestion],
    environments: [fixture_environmentInfo],
    isConnected: true,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const header = canvas.getByTestId("Header");
    await expect(header).toBeInTheDocument();
    await expect(canvas.getByTestId("Header--title")).toHaveTextContent("Agent Status");
    const liveIndicator = canvasElement.querySelector(".bg-emerald-400");
    await expect(liveIndicator).toBeInTheDocument();
  },
};

export const Disconnected: Story = {
  args: {
    agents: [],
    environments: [],
    isConnected: false,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const header = canvas.getByTestId("Header");
    await expect(header).toBeInTheDocument();
    const title = canvas.getByTestId("Header--title");
    await expect(title).toBeInTheDocument();
    const disconnectedIndicator = canvasElement.querySelector(".bg-red-400");
    await expect(disconnectedIndicator).toBeInTheDocument();
  },
};
