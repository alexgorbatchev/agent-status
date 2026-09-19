import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, expect, within } from "@storybook/test";
import { App } from "../App";

const meta: Meta<typeof App> = {
  component: App,
  title: "@agent-status/ui-web/layouts/App",
};

type Story = StoryObj<typeof App>;

const Default: Story = {
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const app = canvas.getByTestId("App");
    await expect(app).toBeInTheDocument();

    const main = canvas.getByTestId("App--main");
    await expect(main).toBeInTheDocument();

    // Verify initial group by project view is active
    const projectBtn = canvas.getByTestId("App--group-by-project");
    const environmentBtn = canvas.getByTestId("App--group-by-environment");

    // Exercise clicking "Group by Environment"
    await userEvent.click(environmentBtn);
    await expect(environmentBtn).toHaveClass("bg-blue-600");

    // Exercise switching back to "Group by Project"
    await userEvent.click(projectBtn);
    await expect(projectBtn).toHaveClass("bg-blue-600");
  },
};

export default meta;
export { Default as App };
