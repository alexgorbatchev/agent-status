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
    await expect(page.getByTestId("App--agent-count")).toContainText("2 Total Agents");

    // Verify agent cards rendered
    await expect(page.getByText("Build agent-status dashboard")).toBeVisible();
    await expect(page.getByText("Refactor telemetry pipeline")).toBeVisible();

    // Verify action line on running card
    await expect(page.getByText("Compiling typescript and verifying bundles")).toBeVisible();

    // Send a live agent_upsert event with WAITING_QUESTION
    await sseServer.sendEvent({
      type: "agent_upsert",
      at: Date.now(),
      agent: fixture_agentStateWaitingQuestion,
    });

    // Verify dynamic update in active counter
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:2");
    await expect(page.getByTestId("App--agent-count")).toContainText("3 Total Agents");
    // Verify card rendered
    await expect(page.getByRole("heading", { name: "Implement user authentication flow" })).toBeVisible();

    // Send another live upsert with ERROR
    await sseServer.sendEvent({
      type: "agent_upsert",
      at: Date.now(),
      agent: fixture_agentStateError,
    });

    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:3");
    await expect(page.getByTestId("Header--attention-agents")).toContainText("Needs Attention:2");
  });
});
