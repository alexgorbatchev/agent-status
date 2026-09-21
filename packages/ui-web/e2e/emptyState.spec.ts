import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";

test.describe("Empty State & Initial Connection", () => {
  test("displays empty fleet message and connected status when 0 agents are active", async ({ page }) => {
    // Intercept SSE endpoint with an empty snapshot
    await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [],
        environments: [],
      },
    ]);

    await page.goto("/");

    // Verify Title & Header
    await expect(page.getByTestId("Header--title")).toContainText("Agent Status");
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:0");
    await expect(page.getByTestId("Header--total-spend")).toContainText("$0.00");

    // Verify empty state container and text
    const emptyState = page.getByTestId("App--empty-state");
    await expect(emptyState).toBeVisible();
    await expect(emptyState).toContainText("No active or recent agent runs detected.");
    await expect(emptyState).toContainText("Start Claude Code or Pi in any observed environment.");

    // Verify agent counter shows 0 Agents
    await expect(page.getByTestId("App--agent-count")).toContainText("0 Agents");
  });
});
