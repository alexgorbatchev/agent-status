import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { LogFrameItem } from "../LogFrameItem";
import {
  fixture_telemetryFrame,
  fixture_telemetryFrameSubagent,
  fixture_telemetryFrameUserPrompt,
  fixture_telemetryFrameToolResult,
  fixture_telemetryFrameThinkingAnsi,
} from "./fixtures";

const meta: Meta<typeof LogFrameItem> = {
  component: LogFrameItem,
  title: "@agent-status/ui-web/components/LogFrameItem",
};

export default meta;

type Story = StoryObj<typeof LogFrameItem>;

export const ToolCall: Story = {
  args: {
    frame: fixture_telemetryFrame,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const item = canvas.getByTestId("LogFrameItem");
    await expect(item).toBeInTheDocument();
    await expect(canvas.getByTestId("LogFrameItem--type")).toHaveTextContent("tool_call");
    await expect(canvas.getByTestId("LogFrameItem--content")).toHaveTextContent(
      "Executing test suite to verify changes.",
    );
    await expect(canvas.getByTestId("ToolExecutionBlock")).toBeInTheDocument();
  },
};

export const SubagentToolCall: Story = {
  args: {
    frame: fixture_telemetryFrameSubagent,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const item = canvas.getByTestId("LogFrameItem");
    await expect(item).toBeInTheDocument();
    await expect(canvas.getByTestId("LogFrameItem--subagent")).toHaveTextContent("Subagent: subagent-worker-1");
  },
};

export const UserPrompt: Story = {
  args: {
    frame: fixture_telemetryFrameUserPrompt,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const item = canvas.getByTestId("LogFrameItem");
    await expect(item).toBeInTheDocument();
    await expect(canvas.getByTestId("LogFrameItem--type")).toHaveTextContent("USER PROMPT");
    await expect(canvas.getByTestId("LogFrameItem--content")).toHaveTextContent(
      "Please build the new telemetry component.",
    );
  },
};

export const ToolResult: Story = {
  args: {
    frame: fixture_telemetryFrameToolResult,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const item = canvas.getByTestId("LogFrameItem");
    await expect(item).toBeInTheDocument();
    await expect(canvas.getByTestId("LogFrameItem--type")).toHaveTextContent("tool_result");
    await expect(canvas.getByTestId("ToolExecutionBlock")).toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--output")).toHaveTextContent(
      "PASS packages/ui-web/src/App.test.tsx",
    );
  },
};

export const ThinkingWithAnsiColors: Story = {
  args: {
    frame: fixture_telemetryFrameThinkingAnsi,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const item = canvas.getByTestId("LogFrameItem");
    await expect(item).toBeInTheDocument();
    await expect(canvas.getByTestId("LogFrameItem--type")).toHaveTextContent("thinking");
    const content = canvas.getByTestId("LogFrameItem--content");
    await expect(content).toBeInTheDocument();
    await expect(content).toHaveTextContent("Thinking: Okay, here's my interpretation");
    await expect(content).toHaveTextContent("Running the Checks");
  },
};
