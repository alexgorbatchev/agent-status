import { test, expect } from "@playwright/test";
import { resolve } from "node:path";
import { createTestStack } from "./helpers/testStack";

test("observes native Antigravity transcripts through the full ingestion pipeline", async ({ page }) => {
  const stack = await createTestStack("agy");
  try {
    await stack.start();
    await page.addInitScript((apiUrl) => {
      Object.assign(window, { __AGENT_STATUS_API_URL__: apiUrl });
    }, `http://localhost:${stack.serverPort}`);
    await page.goto("/");
    const path = await stack.createAgySession("agy-session-1", resolve(stack.testDir, "workspace"));
    await stack.appendAgyTranscript(path, {
      step_index: 0,
      type: "USER_INPUT",
      created_at: new Date().toISOString(),
      content: "Inspect Antigravity telemetry",
    });
    const badge = page.getByTestId("AgentCard--harness-badge");
    await expect(badge).toHaveText("Antigravity", { timeout: 10000 });
    await stack.appendAgyTranscript(path, {
      step_index: 1,
      type: "PLANNER_RESPONSE",
      created_at: new Date().toISOString(),
      model: "gemini-2.5-pro",
      tool_calls: [{ id: "agy-tool-1", name: "run_command", args: { CommandLine: "pwd" } }],
      input_tokens: 100,
      output_tokens: 20,
    });
    await expect(page.getByTestId("AgentCard--title")).toHaveText("Inspect Antigravity telemetry", { timeout: 10000 });
    await expect(page.getByLabel("120 tokens", { exact: true })).toBeVisible({ timeout: 10000 });

    const patchResponse = page.waitForResponse((response) => response.request().method() === "PATCH");
    await page.getByRole("button", { name: "Hide agent session" }).click();
    expect((await patchResponse).status()).toBe(200);
    await page.reload();
    await expect(page.getByTestId("App--toggle-show-hidden")).toHaveText("Show 1 hidden");
    await expect(badge).toBeHidden();
  } finally {
    await stack.stop();
  }
});
