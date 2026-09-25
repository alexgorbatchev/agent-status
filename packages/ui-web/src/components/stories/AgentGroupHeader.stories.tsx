import React from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { AgentGroupHeader } from "../AgentGroupHeader";
import { FolderGit2 } from "lucide-react";

const meta: Meta<typeof AgentGroupHeader> = {
  component: AgentGroupHeader,
  title: "@agent-status/ui-web/components/AgentGroupHeader",
};

type Story = StoryObj<typeof AgentGroupHeader>;

const Default: Story = {
  args: {
    icon: <FolderGit2 size={16} />,
    iconColorClass: "text-blue-400",
    title: "agent-status",
    subtitle: "(github.com/alexgorbatchev/agent-status)",
    activeCount: 3,
    spend: 0.125,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const header = canvas.getByTestId("AgentGroupHeader");
    await expect(header).toBeInTheDocument();
    await expect(canvas.getByTestId("AgentGroupHeader--title")).toHaveTextContent("agent-status");
    await expect(canvas.getByTestId("AgentGroupHeader--active-badge")).toHaveTextContent("3 active");
    await expect(canvas.getByTestId("AgentGroupHeader--spend")).toHaveTextContent("$0.125");
  },
};

export default meta;
export { Default as AgentGroupHeader };
