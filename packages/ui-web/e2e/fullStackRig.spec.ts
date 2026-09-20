import { test, expect } from "@playwright/test";
import { createTestStack, type ITestStack } from "./helpers/testStack";
import type { IWindowWithAgentStatus } from "../src/types";
import { resolve } from "node:path";

test.describe.serial("Full-Stack Rigged Ingestion E2E", () => {
  let stack: ITestStack;

  test.beforeAll(async () => {
    stack = await createTestStack("fullstack");
    await stack.start();
  });

  test.afterAll(async () => {
    if (stack) {
      await stack.stop();
    }
  });

  test("observes Claude Code session creation, transcript streaming, tool runs, and questions end-to-end", async ({
    page,
  }) => {
    // Expose API URL override to page
    await page.addInitScript((apiUrl) => {
      (window as unknown as IWindowWithAgentStatus).__AGENT_STATUS_API_URL__ = apiUrl;
    }, `http://localhost:${stack.serverPort}`);

    await page.goto("/");

    // 1. Initial Empty State
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:0");
    await expect(page.getByTestId("App--empty-state")).toBeVisible();

    // 2. Simulate Claude Code starting in test directory
    const testCwd = resolve(stack.testDir, "workspaces", "my-app");
    const sessionId = "sess-e2e-1001";
    await stack.createClaudeSession(sessionId, 99991, testCwd);

    // Initial transcript lines: user prompt and AI title
    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "user",
      uuid: "msg-1",
      sessionId,
      cwd: testCwd,
      gitBranch: "feat/search",
      timestamp: Date.now(),
      toolUseResult: null,
      message: {
        role: "user",
        content: "Search for database indexing bugs",
      },
    });

    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "ai-title",
      sessionId,
      aiTitle: "Investigate database query performance",
      timestamp: Date.now(),
    });

    // 3. UI automatically detects new agent without page reload
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:1", {
      timeout: 10_000,
    });
    await expect(page.getByText("Investigate database query performance")).toBeVisible({ timeout: 5_000 });

    // 4. Simulate Assistant Tool Execution in transcript
    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "assistant",
      uuid: "msg-2",
      sessionId,
      cwd: testCwd,
      gitBranch: "feat/search",
      timestamp: Date.now(),
      message: {
        role: "assistant",
        model: "claude-sonnet-5",
        stop_reason: "tool_use",
        usage: {
          input_tokens: 1500,
          output_tokens: 250,
          cache_read_input_tokens: 4000,
          cache_creation_input_tokens: 1000,
        },
        content: [
          {
            type: "thinking",
            thinking: "Looking for slow queries in database...",
          },
          {
            type: "tool_use",
            id: "call_tool_grep",
            name: "Grep",
            input: { query: "CREATE INDEX" },
          },
        ],
      },
    });

    // Verify tool execution reflected on card
    await expect(page.getByText("Searching CREATE INDEX")).toBeVisible({ timeout: 5_000 });

    // 5. Simulate AskUserQuestion -> transitions agent to WAITING_QUESTION
    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "assistant",
      uuid: "msg-3",
      sessionId,
      cwd: testCwd,
      gitBranch: "feat/search",
      timestamp: Date.now(),
      message: {
        role: "assistant",
        model: "claude-sonnet-5",
        stop_reason: "tool_use",
        content: [
          {
            type: "tool_use",
            id: "call_tool_ask",
            name: "AskUserQuestion",
            input: {
              questions: [
                {
                  question: "Would you like to rebuild indexes now?",
                },
              ],
            },
          },
        ],
      },
    });

    // Verify Attention Banner triggers automatically
    const banner = page.getByTestId("AttentionBanner");
    await expect(banner).toBeVisible({ timeout: 5_000 });
    await expect(
      page.getByTestId("AttentionBanner--grid").getByText("Would you like to rebuild indexes now?"),
    ).toBeVisible();

    // 6. Open LogDrawer from card and verify live tail logs
    await page.getByTestId("AgentCard--title").click();
    const drawer = page.getByTestId("LogDrawer");
    await expect(drawer).toBeVisible();
    await expect(page.getByTestId("LogDrawer--title")).toContainText("Investigate database query performance");

    // Verify tool frame rendered in stream
    await expect(page.getByTestId("LogDrawer--stream").getByText("Grep").first()).toBeVisible();

    // Close LogDrawer
    await page.getByTestId("LogDrawer--close-button").click();
    await expect(drawer).not.toBeVisible();
  });

  test("observes Pi session creation and telemetry streaming end-to-end", async ({ page }) => {
    await page.addInitScript((apiUrl) => {
      (window as unknown as IWindowWithAgentStatus).__AGENT_STATUS_API_URL__ = apiUrl;
    }, `http://localhost:${stack.serverPort}`);

    await page.goto("/");

    // 1. Simulate Pi session starting
    const piCwd = resolve(stack.testDir, "workspaces", "pi-service");
    const piSessionId = "pi-sess-5001";
    const piTranscriptPath = await stack.createPiSession(piSessionId, piCwd);

    // Initial Pi lines
    await stack.appendPiTranscript(piTranscriptPath, {
      type: "model_change",
      provider: "anthropic",
      modelId: "claude-sonnet-5",
      timestamp: Date.now(),
    });

    await stack.appendPiTranscript(piTranscriptPath, {
      type: "message",
      id: "pi-msg-1",
      parentId: null,
      timestamp: Date.now(),
      message: {
        role: "user",
        content: [{ type: "text", text: "Refactor API server router" }],
      },
    });

    await stack.appendPiTranscript(piTranscriptPath, {
      type: "message",
      id: "pi-msg-2",
      parentId: "pi-msg-1",
      timestamp: Date.now(),
      message: {
        role: "assistant",
        content: [
          {
            type: "toolCall",
            id: "pi_call_1",
            name: "readFile",
            arguments: { path: "src/router.ts" },
          },
        ],
        usage: {
          input: 800,
          output: 100,
          cacheRead: 2000,
          cacheWrite: 500,
        },
      },
    });

    // 2. Verify Pi Agent appears in UI
    await expect(page.getByText("Pi").first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Reading src/router.ts")).toBeVisible({ timeout: 5_000 });
  });
});
