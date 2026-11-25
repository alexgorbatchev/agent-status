import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_agentStateExited,
  fixture_environmentInfo,
} from "../src/components/stories/fixtures";

test.describe("Live Stream & Fleet Hydration", () => {
  test("renders active agents and metrics from initial snapshot and live upserts", async ({ page }) => {
    const sseServer = await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning, fixture_agentStateExited],
        environments: [fixture_environmentInfo],
      },
    ]);

    await page.goto("/");

    // Verify Active count and Total spend calculations
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:1");
    await expect(page.getByTestId("Header--environments")).toContainText("Environments:1/1");

    // Finished agent is hidden by default: 1 visible, toggle finished button visible
    await expect(page.getByTestId("App--toggle-show-finished")).toContainText("Show 1 finished");
    await expect(page.getByTestId("App--agent-count")).toContainText("1 Agent");
    await expect(page.getByText("Build agent-status dashboard")).toBeVisible();
    await expect(page.getByText("Refactor telemetry pipeline")).not.toBeVisible();

    // Reveal finished sessions
    await page.getByTestId("App--toggle-show-finished").click();
    await expect(page.getByTestId("App--toggle-show-finished")).toContainText("Hide 1 finished");
    await expect(page.getByTestId("App--agent-count")).toContainText("2 Agents");
    await expect(page.getByText("Refactor telemetry pipeline")).toBeVisible();

    // Verify action line on running card
    await expect(page.getByText("Compiling typescript and verifying bundles")).toBeVisible();

    // Send a live agent_upsert event with WAITING_QUESTION
    await sseServer.sendEvent({
      type: "agent_upsert",
      at: Date.now(),
      agent: fixture_agentStateWaitingQuestion,
    });

    // Verify dynamic update in attention counter and total agent count
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:1");
    await expect(page.getByTestId("Header--attention-agents")).toContainText("Needs Attention:1");
    await expect(page.getByTestId("App--agent-count")).toContainText("3 Agents");
    // Verify card rendered
    await expect(page.getByRole("heading", { name: "Implement user authentication flow" })).toBeVisible();

    // Send another live upsert with ERROR
    await sseServer.sendEvent({
      type: "agent_upsert",
      at: Date.now(),
      agent: fixture_agentStateError,
    });

    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:1");
    await expect(page.getByTestId("Header--attention-agents")).toContainText("Needs Attention:2");
    await expect(page.getByTestId("App--agent-count")).toContainText("4 Agents");

    // Send a live upsert with a new BUSY agent to verify active counter increment
    const secondBusyAgent = {
      ...fixture_agentStateRunning,
      id: "agent-running-2",
      activity: {
        ...fixture_agentStateRunning.activity,
        title: "Optimize image compression pipeline",
      },
    };
    await sseServer.sendEvent({
      type: "agent_upsert",
      at: Date.now(),
      agent: secondBusyAgent,
    });

    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:2");
    await expect(page.getByTestId("App--agent-count")).toContainText("5 Agents");
    await expect(page.getByRole("heading", { name: "Optimize image compression pipeline" })).toBeVisible();
  });

  test("handles agent_removed SSE event by removing agent cards and updating fleet counts", async ({ page }) => {
    const sseServer = await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning, fixture_agentStateExited],
        environments: [fixture_environmentInfo],
      },
    ]);

    await page.goto("/");

    // Reveal finished sessions so both agents are visible
    await page.getByTestId("App--toggle-show-finished").click();
    await expect(page.getByText("Build agent-status dashboard")).toBeVisible();
    await expect(page.getByText("Refactor telemetry pipeline")).toBeVisible();
    await expect(page.getByTestId("App--agent-count")).toContainText("2 Agents");

    // Dispatch agent_removed for the exited agent
    await sseServer.sendEvent({
      type: "agent_removed",
      at: Date.now(),
      agentId: fixture_agentStateExited.id,
    });

    // Exited agent card should disappear, running agent remains
    await expect(page.getByText("Refactor telemetry pipeline")).not.toBeVisible();
    await expect(page.getByText("Build agent-status dashboard")).toBeVisible();
    await expect(page.getByTestId("App--agent-count")).toContainText("1 Agent");

    // Dispatch agent_removed for the remaining running agent
    await sseServer.sendEvent({
      type: "agent_removed",
      at: Date.now(),
      agentId: fixture_agentStateRunning.id,
    });

    // Both cards should now be gone, showing empty state
    await expect(page.getByText("Build agent-status dashboard")).not.toBeVisible();
    await expect(page.getByTestId("App--empty-state")).toBeVisible();
    await expect(page.getByTestId("App--agent-count")).toContainText("0 Agents");
  });

  test("displays disconnect indicator and error banner on stream drop and recovers on reconnect", async ({ page }) => {
    const sseServer = await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning],
        environments: [fixture_environmentInfo],
      },
    ]);

    await page.goto("/");

    // 1. Initially connected: green indicator, no error banner
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:1");
    await expect(page.getByTestId("App--error")).not.toBeVisible();
    await expect(page.locator("header h1 span.bg-emerald-400")).toBeVisible();

    // 2. Simulate stream drop / server failure
    await sseServer.simulateError();

    // Verify error banner and red pulsing disconnect indicator appear
    await expect(page.getByTestId("App--error")).toBeVisible();
    await expect(page.getByTestId("App--error")).toContainText(
      "Disconnected from agent-server stream. Reconnecting...",
    );
    await expect(page.locator("header h1 span.bg-red-400")).toBeVisible();

    // 3. After retry interval (3s in useEventStream), automatic reconnect clears error and restores green indicator
    await expect(page.getByTestId("App--error")).not.toBeVisible({ timeout: 10_000 });
    await expect(page.locator("header h1 span.bg-emerald-400")).toBeVisible();
  });
});
