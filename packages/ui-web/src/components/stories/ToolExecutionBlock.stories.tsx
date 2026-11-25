import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { ToolExecutionBlock } from "../ToolExecutionBlock";

const meta: Meta<typeof ToolExecutionBlock> = {
  component: ToolExecutionBlock,
  title: "@agent-status/ui-web/components/ToolExecutionBlock",
};

export default meta;

type Story = StoryObj<typeof ToolExecutionBlock>;

export const Default: Story = {
  args: {
    toolName: "Bash",
    toolInput: { command: "bun test" },
    toolOutput: "1 pass, 0 fail",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const block = canvas.getByTestId("ToolExecutionBlock");
    await expect(block).toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--name")).toHaveTextContent("Bash");
    await expect(canvas.getByTestId("ToolExecutionBlock--input")).toHaveTextContent("bun test");
    await expect(canvas.getByTestId("ToolExecutionBlock--output")).toHaveTextContent("1 pass, 0 fail");
  },
};

export const MultilineOutput: Story = {
  args: {
    toolName: "Bash",
    toolInput: { command: "bun test" },
    toolOutput: "Line 1: starting test runner\nLine 2: 89 passed\nLine 3: 0 failed",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const block = canvas.getByTestId("ToolExecutionBlock");
    await expect(block).toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--output")).toHaveTextContent(
      "Line 1: starting test runner Line 2: 89 passed Line 3: 0 failed",
    );
  },
};

export const WithoutToolName: Story = {
  args: {
    toolOutput: "Anonymous tool execution output line 1\nLine 2",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const block = canvas.getByTestId("ToolExecutionBlock");
    await expect(block).toBeInTheDocument();
    await expect(canvas.queryByTestId("ToolExecutionBlock--name")).not.toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--output")).toHaveTextContent(
      "Anonymous tool execution output line 1 Line 2",
    );
  },
};

export const WithoutInputOrOutput: Story = {
  args: {
    toolName: "ReadFile",
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const block = canvas.getByTestId("ToolExecutionBlock");
    await expect(block).toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--name")).toHaveTextContent("ReadFile");
    await expect(canvas.queryByTestId("ToolExecutionBlock--input")).not.toBeInTheDocument();
    await expect(canvas.queryByTestId("ToolExecutionBlock--output")).not.toBeInTheDocument();
  },
};

export const CleanExit: Story = {
  args: {
    toolName: "bash",
    toolInput: { command: "just test" },
    toolOutput: "16 pass, 0 fail",
    exitCode: 0,
    isError: false,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const status = canvas.getByTestId("ToolExecutionBlock--status");
    await expect(status).toBeInTheDocument();
    await expect(status).toHaveTextContent("exit 0");
    await expect(canvas.getByTestId("ToolExecutionBlock--stream-type")).toHaveTextContent("OUTPUT");
  },
};

export const FailedExit: Story = {
  args: {
    toolName: "bash",
    toolInput: { command: "bun run build" },
    toolOutput: "Error: build failed with exit code 1",
    exitCode: 1,
    isError: true,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const status = canvas.getByTestId("ToolExecutionBlock--status");
    await expect(status).toBeInTheDocument();
    await expect(status).toHaveTextContent("exit 1");
    await expect(canvas.getByTestId("ToolExecutionBlock--stream-type")).toHaveTextContent("STDERR");
  },
};

export const StructuredParameters: Story = {
  args: {
    toolName: "grep",
    toolInput: { pattern: "TODO", path: "src", recursive: true },
    toolOutput: "src/App.tsx:42: // TODO: fix",
    exitCode: 0,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const params = canvas.getByTestId("ToolExecutionBlock--params");
    await expect(params).toBeInTheDocument();
    await expect(params).toHaveTextContent("pattern");
    await expect(params).toHaveTextContent("TODO");
  },
};

export const FiftySevenLinesOutput: Story = {
  args: {
    toolName: "bash",
    toolInput: { command: "git status" },
    toolOutput: Array.from({ length: 57 }, (_, i) => `Line ${i + 1}: telemetry event processing item`).join("\n"),
    exitCode: 0,
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const scrollContainer = canvas.getByTestId("ToolExecutionBlock--output-scroll-container");
    await expect(scrollContainer).toBeInTheDocument();
    await expect(canvas.getByTestId("ToolExecutionBlock--line-count")).toHaveTextContent("57 lines");
  },
};
