import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";
import { fixture_agentStateAgy, fixture_environmentInfo } from "../src/components/stories/fixtures";

test("renders Antigravity sessions with a named harness badge", async ({ page }, testInfo) => {
  await setupMockSSERoute(page, [
    { type: "snapshot", at: Date.now(), agents: [fixture_agentStateAgy], environments: [fixture_environmentInfo] },
  ]);
  await page.goto("/");
  const badge = page.getByTestId("AgentCard--harness-badge");
  await expect(badge).toBeVisible();
  await page.screenshot({ path: process.env.AGY_SCREENSHOT_PATH ?? testInfo.outputPath("antigravity.png") });
  await expect(badge).toHaveText("Antigravity");
  await expect(badge).toHaveAttribute("aria-label", "Antigravity");
});
