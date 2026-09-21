import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";
import {
  fixture_agentStateRunning,
  fixture_environmentInfo,
  fixture_telemetryFrame,
  fixture_telemetryFrameSubagent,
  fixture_telemetryFrameThinking,
  fixture_telemetryFrameError,
  fixture_telemetryFrameWithUsage,
} from "../src/components/stories/fixtures";

test.describe("LogDrawer & Live Tail", () => {
  test("opens slide-over drawer, fetches initial logs, and handles live frames", async ({ page }) => {
    await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning],
        environments: [fixture_environmentInfo],
      },
    ]);

    // Mock the REST logs endpoint
    await page.route("**/api/v1/agents/*/logs*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "application/json",
          "Access-Control-Allow-Origin": "*",
        },
        body: JSON.stringify([fixture_telemetryFrame]),
      });
    });

    // Mock the live tail SSE endpoint
    await page.route("**/api/v1/agents/*/live-tail*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
          "Access-Control-Allow-Origin": "*",
        },
        body: `data: ${JSON.stringify(fixture_telemetryFrame)}\n\n`,
      });
    });

    await page.goto("/");

    // Click on the agent card to open the drawer
    await page.getByText("Build agent-status dashboard").click();

    // Verify LogDrawer is visible
    const drawer = page.getByTestId("LogDrawer");
    await expect(drawer).toBeVisible();

    // Verify drawer header details
    await expect(page.getByTestId("LogDrawer--title")).toContainText("Build agent-status dashboard");
    await expect(page.getByTestId("LogDrawer--debug-button")).toBeVisible();
    await expect(page.getByTestId("LogDrawer--debug-button")).toHaveAttribute(
      "aria-label",
      "Copy debug command to clipboard",
    );

    // Verify log frame content rendered in drawer stream
    await expect(
      page.getByTestId("LogDrawer--stream").getByText("Executing test suite to verify changes.").first(),
    ).toBeVisible();
    await expect(page.getByTestId("LogDrawer--stream").getByText("Bash").first()).toBeVisible();

    // Close drawer via close button
    await page.getByTestId("LogDrawer--close-button").click();
    await expect(drawer).not.toBeVisible();
  });

  test("renders telemetry frame varieties: subagents, thinking blocks, errors, and token breakdown", async ({
    page,
  }) => {
    await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning],
        environments: [fixture_environmentInfo],
      },
    ]);

    const diverseFrames = [
      fixture_telemetryFrameSubagent,
      fixture_telemetryFrameThinking,
      fixture_telemetryFrameError,
      fixture_telemetryFrameWithUsage,
    ];

    await page.route("**/api/v1/agents/*/logs*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "application/json",
          "Access-Control-Allow-Origin": "*",
        },
        body: JSON.stringify(diverseFrames),
      });
    });

    await page.route("**/api/v1/agents/*/live-tail*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
          "Access-Control-Allow-Origin": "*",
        },
        body: `data: ${JSON.stringify(fixture_telemetryFrameSubagent)}\n\n`,
      });
    });

    await page.goto("/");

    // Open drawer
    await page.getByText("Build agent-status dashboard").click();
    const stream = page.getByTestId("LogDrawer--stream");
    await expect(stream).toBeVisible();

    // 1. Assert subagent badge and content
    await expect(stream.getByText("Subagent: subagent-worker-1").first()).toBeVisible();
    await expect(stream.getByText("Running subagent task to analyze dependencies.").first()).toBeVisible();

    // 2. Assert thinking block
    await expect(stream.getByText("THINKING").first()).toBeVisible();
    await expect(
      stream.getByText("Analyzing state transitions and evaluating edge cases for SSE reconnects.").first(),
    ).toBeVisible();

    // 3. Assert error block
    await expect(stream.getByText("ERROR").first()).toBeVisible();
    await expect(stream.getByText("Command failed with exit code 1: syntax error near 'INDEX'").first()).toBeVisible();

    // 4. Assert token usage breakdown row
    await expect(stream.getByText("In: 1,250").first()).toBeVisible();
    await expect(stream.getByText("Out: 350").first()).toBeVisible();
    await expect(stream.getByText("Cache: 4,000").first()).toBeVisible();
    await expect(stream.getByText("Total: 6,200").first()).toBeVisible();
  });

  test("renders empty state notice when agent has no telemetry logs", async ({ page }) => {
    await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning],
        environments: [fixture_environmentInfo],
      },
    ]);

    await page.route("**/api/v1/agents/*/logs*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "application/json",
          "Access-Control-Allow-Origin": "*",
        },
        body: JSON.stringify([]),
      });
    });

    await page.route("**/api/v1/agents/*/live-tail*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
          "Access-Control-Allow-Origin": "*",
        },
        body: "",
      });
    });

    await page.goto("/");
    await page.getByText("Build agent-status dashboard").click();

    const stream = page.getByTestId("LogDrawer--stream");
    await expect(stream.getByText("No telemetry log events recorded yet.")).toBeVisible();
  });

  test("contains long 500-character unbroken strings without horizontal overflow", async ({ page }) => {
    await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning],
        environments: [fixture_environmentInfo],
      },
    ]);

    const longUnbrokenToken = "x".repeat(500);
    const longStringFrame = {
      ...fixture_telemetryFrame,
      data: {
        ...fixture_telemetryFrame.data,
        content: `Markdown with long unbroken token: ${longUnbrokenToken}\n\n\`\`\`sh\n${longUnbrokenToken}\n\`\`\`\n\n| Column | Value |\n|---|---|\n| Data | ${longUnbrokenToken} |`,
        toolName: "Bash",
        toolInput: {
          command: `echo ${longUnbrokenToken}`,
        },
        toolOutput: longUnbrokenToken,
      },
    };

    await page.route("**/api/v1/agents/*/logs*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "application/json",
          "Access-Control-Allow-Origin": "*",
        },
        body: JSON.stringify([longStringFrame]),
      });
    });

    await page.route("**/api/v1/agents/*/live-tail*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          Connection: "keep-alive",
          "Access-Control-Allow-Origin": "*",
        },
        body: `data: ${JSON.stringify(longStringFrame)}\n\n`,
      });
    });

    await page.goto("/");
    await page.getByText("Build agent-status dashboard").click();

    const drawer = page.getByTestId("LogDrawer");
    await expect(drawer).toBeVisible();

    const stream = page.getByTestId("LogDrawer--stream");
    await expect(stream).toBeVisible();

    // Verify stream scrollWidth does not exceed clientWidth (zero horizontal overflow)
    const overflowDiff = await stream.evaluate((el) => el.scrollWidth - el.clientWidth);
    expect(overflowDiff).toBeLessThanOrEqual(0);
  });
});
