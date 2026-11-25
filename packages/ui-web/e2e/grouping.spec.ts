import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_environmentInfo,
  fixture_staleEnvironmentInfo,
} from "../src/components/stories/fixtures";

test.describe("Grouping and View Switching", () => {
  test("toggles between Group by Project and Group by Environment", async ({ page }) => {
    await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [
          fixture_agentStateRunning,
          {
            ...fixture_agentStateWaitingQuestion,
            environment: fixture_staleEnvironmentInfo,
          },
        ],
        environments: [fixture_environmentInfo, fixture_staleEnvironmentInfo],
      },
    ]);

    await page.goto("/");

    // Default view: Group by Project
    const projectGroup = page.getByTestId("ProjectGroup");
    await expect(projectGroup).toBeVisible();
    await expect(projectGroup).toContainText("agent-status");

    // Switch to Group by Environment
    await page.getByTestId("App--group-by-environment").click();

    // Verify Environment Group headers
    const envGroups = page.getByTestId("EnvironmentGroup");
    await expect(envGroups.nth(0)).toBeVisible();
    await expect(envGroups.nth(1)).toBeVisible();

    await expect(envGroups.nth(0)).toContainText("work-macbook");
    await expect(envGroups.nth(1)).toContainText("cloud-builder");

    // Verify stale badge on stale environment
    await expect(page.getByTestId("App--main").getByText("Stale")).toBeVisible();
    await expect(page.getByTestId("App--main").getByText("Live")).toBeVisible();

    // Switch back to Group by Project
    await page.getByTestId("App--group-by-project").click();
    await expect(projectGroup).toBeVisible();
  });
});
