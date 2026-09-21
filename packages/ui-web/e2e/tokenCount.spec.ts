import { test, expect } from "@playwright/test";
import { setupMockSSERoute } from "./helpers/sseMock";
import { createTestStack, type ITestStack } from "./helpers/testStack";
import { fixture_agentStateRunning, fixture_environmentInfo } from "../src/components/stories/fixtures";
import type { IWindowWithAgentStatus } from "../src/types";
import { resolve } from "node:path";

test.describe("Token Count Live Updates (SSE Mock)", () => {
  test("reactively updates token count on AgentCard when agent_upsert arrives", async ({ page }) => {
    const sseServer = await setupMockSSERoute(page, [
      {
        type: "snapshot",
        at: Date.now(),
        agents: [fixture_agentStateRunning],
        environments: [fixture_environmentInfo],
      },
    ]);

    await page.goto("/");

    // 1. Initial token count from fixture: 7,300 tokens
    const agentCard = page.getByTestId("AgentCard");
    await expect(agentCard).toBeVisible();
    await expect(agentCard.getByTestId("AgentCard--footer")).toContainText("7,300 tokens");

    // 2. Dispatch live agent_upsert with incremented token usage
    const updatedAgentFirstIncrement = {
      ...fixture_agentStateRunning,
      activity: {
        ...fixture_agentStateRunning.activity,
        usage: {
          input: 3000,
          output: 500,
          cacheRead: 8000,
          cacheWrite: 1000,
          reasoning: 0,
          total: 12500,
        },
      },
    };

    await sseServer.sendEvent({
      type: "agent_upsert",
      at: Date.now(),
      agent: updatedAgentFirstIncrement,
    });

    // Verify token count dynamically updates to 12,500 tokens
    await expect(agentCard.getByTestId("AgentCard--footer")).toContainText("12,500 tokens");

    // 3. Dispatch second agent_upsert with further accumulated token usage
    const updatedAgentSecondIncrement = {
      ...fixture_agentStateRunning,
      activity: {
        ...fixture_agentStateRunning.activity,
        usage: {
          input: 10000,
          output: 2500,
          cacheRead: 20000,
          cacheWrite: 2500,
          reasoning: 0,
          total: 35000,
        },
      },
    };

    await sseServer.sendEvent({
      type: "agent_upsert",
      at: Date.now(),
      agent: updatedAgentSecondIncrement,
    });

    // Verify token count dynamically updates to 35,000 tokens
    await expect(agentCard.getByTestId("AgentCard--footer")).toContainText("35,000 tokens");
  });
});

test.describe.serial("Token Count Ingestion E2E (Full-Stack Rig)", () => {
  let stack: ITestStack;

  test.beforeAll(async () => {
    stack = await createTestStack("tokencount");
    await stack.start();
  });

  test.afterAll(async () => {
    if (stack) {
      await stack.stop();
    }
  });

  test("observes token count updates end-to-end as assistant turns arrive", async ({ page }) => {
    await page.addInitScript((apiUrl) => {
      (window as unknown as IWindowWithAgentStatus).__AGENT_STATUS_API_URL__ = apiUrl;
    }, `http://localhost:${stack.serverPort}`);

    await page.goto("/");

    // 1. Initial empty state
    await expect(page.getByTestId("Header--active-agents")).toContainText("Active:0");

    // 2. Start a Claude Code session
    const testCwd = resolve(stack.testDir, "workspaces", "token-app");
    const sessionId = "sess-token-e2e-1";
    await stack.createClaudeSession(sessionId, 99992, testCwd);

    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "user",
      uuid: "msg-token-1",
      sessionId,
      cwd: testCwd,
      gitBranch: "feat/token-tracking",
      timestamp: Date.now(),
      toolUseResult: null,
      message: {
        role: "user",
        content: "Analyze repository token usage",
      },
    });

    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "ai-title",
      sessionId,
      aiTitle: "Token Tracking Analysis",
      timestamp: Date.now(),
    });

    // 3. First assistant turn with usage: 1500 + 250 + 4000 + 1000 = 6,750 tokens
    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "assistant",
      uuid: "msg-token-2",
      sessionId,
      cwd: testCwd,
      gitBranch: "feat/token-tracking",
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
            type: "text",
            text: "Reading project configuration...",
          },
        ],
      },
    });

    // Verify agent card appears and shows initial token count: 6,750 tokens
    const agentCard = page.getByTestId("AgentCard");
    await expect(agentCard).toBeVisible({ timeout: 10_000 });
    await expect(agentCard.getByTestId("AgentCard--footer")).toContainText("6,750 tokens", { timeout: 10_000 });

    // 4. Second assistant turn with additional usage: 500 + 100 + 1000 + 0 = 1,600 tokens
    // Accumulated total: 6,750 + 1,600 = 8,350 tokens
    await stack.appendClaudeTranscript(sessionId, testCwd, {
      type: "assistant",
      uuid: "msg-token-3",
      sessionId,
      cwd: testCwd,
      gitBranch: "feat/token-tracking",
      timestamp: Date.now(),
      message: {
        role: "assistant",
        model: "claude-sonnet-5",
        stop_reason: "end_turn",
        usage: {
          input_tokens: 500,
          output_tokens: 100,
          cache_read_input_tokens: 1000,
          cache_creation_input_tokens: 0,
        },
        content: [
          {
            type: "text",
            text: "Analysis completed.",
          },
        ],
      },
    });

    // Verify token count dynamically updates on AgentCard without page reload: 8,350 tokens
    await expect(agentCard.getByTestId("AgentCard--footer")).toContainText("8,350 tokens", { timeout: 10_000 });
  });
});
