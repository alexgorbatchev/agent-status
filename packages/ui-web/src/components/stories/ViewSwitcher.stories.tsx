import React from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { expect, within, userEvent } from "@storybook/test";
import { ViewSwitcher } from "../ViewSwitcher";
import { Layers, Server } from "lucide-react";

const meta: Meta<typeof ViewSwitcher> = {
  component: ViewSwitcher,
  title: "@agent-status/ui-web/components/ViewSwitcher",
};

type Story = StoryObj<typeof ViewSwitcher>;

const Default: Story = {
  args: {
    options: [
      {
        id: "project",
        label: "Group by Project",
        icon: <Layers size={14} />,
        testId: "App--group-by-project",
      },
      {
        id: "environment",
        label: "Group by Environment",
        icon: <Server size={14} />,
        testId: "App--group-by-environment",
      },
    ],
    value: "project",
    onChange: () => {},
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const switcher = canvas.getByTestId("ViewSwitcher");
    await expect(switcher).toBeInTheDocument();

    const projectBtn = canvas.getByTestId("App--group-by-project");
    const envBtn = canvas.getByTestId("App--group-by-environment");

    await expect(projectBtn).toBeInTheDocument();
    await expect(envBtn).toBeInTheDocument();
    await expect(projectBtn).toHaveClass("bg-blue-600");
    await expect(envBtn).toHaveClass("bg-slate-900");

    await userEvent.click(envBtn);
  },
};

export default meta;
export { Default as ViewSwitcher };
