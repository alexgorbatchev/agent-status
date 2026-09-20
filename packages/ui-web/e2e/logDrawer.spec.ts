import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";
import {
  fixture_agentStateRunning,
  fixture_environmentInfo,
  fixture_telemetryFrame,
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

    // Verify log frame content rendered in drawer stream
    await expect(
      page.getByTestId("LogDrawer--stream").getByText("Executing test suite to verify changes.").first(),
    ).toBeVisible();
    await expect(page.getByTestId("LogDrawer--stream").getByText("Bash").first()).toBeVisible();

    // Close drawer via close button
    await page.getByTestId("LogDrawer--close-button").click();
    await expect(drawer).not.toBeVisible();
  });
});
