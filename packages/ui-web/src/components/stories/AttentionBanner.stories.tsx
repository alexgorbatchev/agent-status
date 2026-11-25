import type { Meta, StoryObj } from "@storybook/react";
import { fn, userEvent, expect, within } from "@storybook/test";
import { AttentionBanner } from "../AttentionBanner";
import { fixture_agentStateWaitingQuestion, fixture_agentStateError } from "./fixtures";

const meta: Meta<typeof AttentionBanner> = {
  component: AttentionBanner,
  title: "@agent-status/ui-web/components/AttentionBanner",
};

type Story = StoryObj<typeof AttentionBanner>;

const onSelectAgent = fn();

const Default: Story = {
  args: {
    agents: [fixture_agentStateWaitingQuestion, fixture_agentStateError],
    onSelectAgent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const banner = canvas.getByTestId("AttentionBanner");
    await expect(banner).toBeInTheDocument();
    await expect(canvas.getByTestId("AttentionBanner--title")).toHaveTextContent(
      "Act Now — 2 Agents Require Attention",
    );

    const cards = canvas.getAllByTestId("AttentionBanner--agent-card");
    await expect(cards).toHaveLength(2);
    await expect(cards[0]).toHaveAttribute("role", "button");
    await expect(cards[0]).toHaveAttribute("tabindex", "0");
    await expect(cards[0]).toHaveAttribute(
      "aria-label",
      "View agent needing attention: Implement user authentication flow",
    );

    // Exercise clicking first attention card
    await userEvent.click(cards[0]);
    await expect(onSelectAgent).toHaveBeenCalledWith(fixture_agentStateWaitingQuestion);

    // Exercise clicking second attention card
    await userEvent.click(cards[1]);
    await expect(onSelectAgent).toHaveBeenCalledWith(fixture_agentStateError);
  },
};

export default meta;
export { Default as AttentionBanner };
