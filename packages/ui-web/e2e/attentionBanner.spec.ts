import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";
import {
  fixture_agentStateWaitingQuestion,
  fixture_agentStateError,
  fixture_environmentInfo,
} from "../src/components/stories/fixtures";

test.describe("Attention Banner & Alert Actions", () => {
  test("displays prompt question and error alerts in attention banner", async ({ page }) => {
    const sseServer = await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateWaitingQuestion, fixture_agentStateError],
        environments: [fixture_environmentInfo],
      },
    ]);

    await page.goto("/");

    // Verify Attention Banner container rendered
    const banner = page.getByTestId("AttentionBanner");
    await expect(banner).toBeVisible();

    // Verify attention banner header
    await expect(page.getByTestId("AttentionBanner--title")).toContainText("Act Now — 2 Agents Require Attention");

    // Verify WAITING_QUESTION card in attention banner
    await expect(
      page.getByTestId("AttentionBanner--grid").getByText("Should we use SQLite or PostgreSQL for auth tokens?"),
    ).toBeVisible();

    // Verify ERROR card in attention banner
    await expect(
      page
        .getByTestId("AttentionBanner--grid")
        .getByText("Database migration returned exit code 1: syntax error near 'INDEX'"),
    ).toBeVisible();

    // Click on the attention card for WAITING_QUESTION to open drawer
    await page
      .getByTestId("AttentionBanner--grid")
      .getByText("Should we use SQLite or PostgreSQL for auth tokens?")
      .click();

    // Verify LogDrawer opened
    const drawer = page.getByTestId("LogDrawer");
    await expect(drawer).toBeVisible();
    await expect(page.getByTestId("LogDrawer--title")).toContainText("Implement user authentication flow");

    // Close drawer
    await page.getByTestId("LogDrawer--close-button").click();
    await expect(drawer).not.toBeVisible();

    // When agents resolve attention, banner disappears
    await sseServer.sendEvent({
      type: "snapshot",
      at: Date.now(),
      agents: [],
      environments: [fixture_environmentInfo],
    });

    await expect(page.getByTestId("AttentionBanner")).not.toBeVisible();
  });
});
