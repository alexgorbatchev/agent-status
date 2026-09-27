import { describe, it, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import {
  calculateTotalSpend,
  formatCurrency,
  filterActiveAgents,
  filterRunningAgents,
  isAgentRequiringAttention,
  formatRelativeTime,
  formatExactTime,
  getAgentDebugCommand,
  getToolExecutionDuration,
  getToolStallTier,
  formatDuration,
  isSessionStalled,
  trimProjectTitlePrefix,
  getIdleFreshness,
  isAgentStaleIdle,
  getAgentIdleFreshness,
  type IdleFreshness,
  formatRateLimitCallout,
} from "../agentUtils";
import { factory_createAgent, fixture_testEnvironment, fixture_testStaleEnvironment } from "./fixtures";
import type { ActivityState, IAgentActivity, IAgentState, LifecycleState } from "../../types";

interface IIdleFreshnessFixture {
  name: string;
  lifecycle: LifecycleState;
  activity: ActivityState;
  hasUnseenCompletion: boolean;
  nowMs: number;
  updatedAt: number;
  expectedTier: IdleFreshness;
  expectedStaleIdle: boolean;
}

interface IProjectPrefixFixture {
  name: string;
  title: string;
  projectName: string;
  expected: string;
}

describe("agentUtils", () => {
  describe("calculateTotalSpend", () => {
    it("returns 0 for an empty agent list", () => {
      const spend = calculateTotalSpend([]);
      expect(spend).toBe(0);
    });

    it("sums costs across multiple agents correctly", () => {
      const agent1 = factory_createAgent({
        id: "a1",
        activity: {
          ...factory_createAgent().activity,
          cost: { currency: "USD", total: 0.12, isEstimated: false },
        },
      });
      const agent2 = factory_createAgent({
        id: "a2",
        activity: {
          ...factory_createAgent().activity,
          cost: { currency: "USD", total: 0.38, isEstimated: false },
        },
      });
      const agent3 = factory_createAgent({
        id: "a3",
        activity: {
          ...factory_createAgent().activity,
          cost: null,
        },
      });

      const spend = calculateTotalSpend([agent1, agent2, agent3]);
      expect(spend).toBeCloseTo(0.5, 4);
    });
  });

  describe("formatCurrency", () => {
    it("formats amounts with default precision of 2", () => {
      expect(formatCurrency(12.3456)).toBe("$12.35");
      expect(formatCurrency(0)).toBe("$0.00");
      expect(formatCurrency(5)).toBe("$5.00");
    });

    it("formats amounts with custom precision", () => {
      expect(formatCurrency(12.3456, 3)).toBe("$12.346");
      expect(formatCurrency(0.12, 4)).toBe("$0.1200");
      expect(formatCurrency(10, 0)).toBe("$10");
    });
  });

  describe("filterActiveAgents", () => {
    it("returns only agents with RUNNING lifecycle and BUSY activity", () => {
      const busyAgent = factory_createAgent({
        id: "busy-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
        },
      });
      const idleAgent = factory_createAgent({
        id: "idle-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
        },
      });
      const exitedAgent = factory_createAgent({
        id: "exited-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "EXITED",
          activity: "IDLE",
        },
      });

      const active = filterActiveAgents([busyAgent, idleAgent, exitedAgent]);
      expect(active.length).toBe(1);
      expect(active[0]?.id).toBe("busy-1");
    });

    it("returns empty array when no agents are busy", () => {
      const idleAgent = factory_createAgent({
        id: "idle-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
        },
      });
      const exitedAgent = factory_createAgent({
        id: "exited-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "EXITED",
          activity: "IDLE",
        },
      });

      const active = filterActiveAgents([idleAgent, exitedAgent]);
      expect(active).toEqual([]);
    });
  });

  describe("filterRunningAgents", () => {
    it("returns all agents with RUNNING lifecycle regardless of activity", () => {
      const busyAgent = factory_createAgent({
        id: "busy-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
        },
      });
      const idleAgent = factory_createAgent({
        id: "idle-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
        },
      });
      const exitedAgent = factory_createAgent({
        id: "exited-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "EXITED",
          activity: "IDLE",
        },
      });

      const running = filterRunningAgents([busyAgent, idleAgent, exitedAgent]);
      expect(running.length).toBe(2);
      expect(running.map((a) => a.id)).toEqual(["busy-1", "idle-1"]);
    });
  });

  describe("isAgentRequiringAttention", () => {
    it("returns true for RUNNING agent with WAITING_QUESTION in active environment", () => {
      const agent = factory_createAgent({
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "WAITING_QUESTION",
        },
      });
      expect(isAgentRequiringAttention(agent)).toBe(true);
    });

    it("returns true for RUNNING agent with ERROR in active environment", () => {
      const agent = factory_createAgent({
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "ERROR",
        },
      });
      expect(isAgentRequiringAttention(agent)).toBe(true);
    });

    it("returns false for RUNNING agent with WAITING_QUESTION in stale environment", () => {
      const agent = factory_createAgent({
        environment: fixture_testStaleEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "WAITING_QUESTION",
        },
      });
      expect(isAgentRequiringAttention(agent)).toBe(false);
    });

    it("returns false for RUNNING agent with ERROR in stale environment", () => {
      const agent = factory_createAgent({
        environment: fixture_testStaleEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "ERROR",
        },
      });
      expect(isAgentRequiringAttention(agent)).toBe(false);
    });

    it("returns false for recently updated running RATE_LIMITED agent", () => {
      const now = Date.now();
      const agent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "RATE_LIMITED",
          updatedAt: now - 10000,
        },
      });
      expect(isAgentRequiringAttention(agent, now)).toBe(false);
    });

    it("returns true for running RATE_LIMITED agent stalled for more than 2 minutes", () => {
      const now = Date.now();
      const agent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "RATE_LIMITED",
          updatedAt: now - 130000,
        },
      });
      expect(isAgentRequiringAttention(agent, now)).toBe(true);
    });

    it("suppresses Act Now attention when rateLimitResetAt is in the future even if stalled (> 2m)", () => {
      const now = Date.now();
      const agent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "RATE_LIMITED",
          updatedAt: now - 130000,
          rateLimitResetAt: now + 30000,
        },
      });
      expect(isAgentRequiringAttention(agent, now)).toBe(false);
    });

    it("suppresses Act Now attention when rateLimitResetAt is null but retryAfterSeconds is in the future", () => {
      const now = Date.now();
      const agent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "RATE_LIMITED",
          updatedAt: now - 150000, // 150s ago (> 120s stall threshold)
          rateLimitResetAt: null,
          retryAfterSeconds: 300, // 300s retryAfter (150s remaining)
        },
      });
      expect(isAgentRequiringAttention(agent, now)).toBe(false);
    });

    it("returns true for attention when rateLimitResetAt is null and retryAfterSeconds has elapsed", () => {
      const now = Date.now();
      const agent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "RATE_LIMITED",
          updatedAt: now - 350000, // 350s ago
          rateLimitResetAt: null,
          retryAfterSeconds: 300, // 300s retryAfter (elapsed 50s ago)
        },
      });
      expect(isAgentRequiringAttention(agent, now)).toBe(true);
    });

    it("returns false for RUNNING agent with BUSY or IDLE activity", () => {
      const now = Date.now();
      const busyAgent: IAgentState = factory_createAgent({
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          updatedAt: now,
        },
      });
      const idleAgent: IAgentState = factory_createAgent({
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          updatedAt: now,
        },
      });
      expect(isAgentRequiringAttention(busyAgent)).toBe(false);
      expect(isAgentRequiringAttention(idleAgent)).toBe(false);
    });

    it("returns false for EXITED agent even if ERROR", () => {
      const agent = factory_createAgent({
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "EXITED",
          activity: "ERROR",
        },
      });
      expect(isAgentRequiringAttention(agent)).toBe(false);
    });
  });

  describe("formatRelativeTime", () => {
    const baseTime = 1789850000000;

    it("returns '—' for missing, non-positive or NaN timestamps", () => {
      expect(formatRelativeTime(0, baseTime)).toBe("—");
      expect(formatRelativeTime(-1000, baseTime)).toBe("—");
      expect(formatRelativeTime(Number.NaN, baseTime)).toBe("—");
    });

    it("returns 'just now' for future timestamps or difference < 5s", () => {
      expect(formatRelativeTime(baseTime + 10000, baseTime)).toBe("just now");
      expect(formatRelativeTime(baseTime, baseTime)).toBe("just now");
      expect(formatRelativeTime(baseTime - 2000, baseTime)).toBe("just now");
      expect(formatRelativeTime(baseTime - 4999, baseTime)).toBe("just now");
    });

    it("formats seconds ago for difference between 5s and 59s", () => {
      expect(formatRelativeTime(baseTime - 5000, baseTime)).toBe("5s ago");
      expect(formatRelativeTime(baseTime - 12000, baseTime)).toBe("12s ago");
      expect(formatRelativeTime(baseTime - 59000, baseTime)).toBe("59s ago");
    });

    it("formats minutes ago for difference between 1m and 59m", () => {
      expect(formatRelativeTime(baseTime - 60000, baseTime)).toBe("1m ago");
      expect(formatRelativeTime(baseTime - 120000, baseTime)).toBe("2m ago");
      expect(formatRelativeTime(baseTime - 3540000, baseTime)).toBe("59m ago");
    });

    it("formats hours ago for difference between 1h and 23h", () => {
      expect(formatRelativeTime(baseTime - 3600000, baseTime)).toBe("1h ago");
      expect(formatRelativeTime(baseTime - 7200000, baseTime)).toBe("2h ago");
      expect(formatRelativeTime(baseTime - 82800000, baseTime)).toBe("23h ago");
    });

    it("formats days ago for difference >= 24h", () => {
      expect(formatRelativeTime(baseTime - 86400000, baseTime)).toBe("1d ago");
      expect(formatRelativeTime(baseTime - 172800000, baseTime)).toBe("2d ago");
    });
  });

  describe("formatExactTime", () => {
    it("returns 'Never' for missing, non-positive or NaN timestamps", () => {
      expect(formatExactTime(0)).toBe("Never");
      expect(formatExactTime(-500)).toBe("Never");
      expect(formatExactTime(Number.NaN)).toBe("Never");
    });

    it("formats valid timestamp as locale date time string", () => {
      const ts = 1789850000000;
      expect(formatExactTime(ts)).toBe(new Date(ts).toLocaleString());
    });
  });

  describe("getAgentDebugCommand", () => {
    it("builds the correct CLI command for claude-code", () => {
      const agent = factory_createAgent({
        harness: "claude-code",
        sessionId: "sess-claude-123",
      });
      expect(getAgentDebugCommand(agent)).toBe("just debug session replay --harness claude-code sess-claude-123");
    });

    it("builds the correct CLI command for pi", () => {
      const agent = factory_createAgent({
        harness: "pi",
        sessionId: "sess-pi-456",
      });
      expect(getAgentDebugCommand(agent)).toBe("just debug session replay --harness pi sess-pi-456");
    });

    it("builds the correct CLI command for codex", () => {
      const agent = factory_createAgent({
        harness: "codex",
        sessionId: "sess-codex-789",
      });
      expect(getAgentDebugCommand(agent)).toBe("just debug session replay --harness codex sess-codex-789");
    });

    it("builds the correct CLI command for opencode", () => {
      const agent = factory_createAgent({
        harness: "opencode",
        sessionId: "sess-opencode-012",
      });
      expect(getAgentDebugCommand(agent)).toBe("just debug session replay --harness opencode sess-opencode-012");
    });
  });

  describe("getToolExecutionDuration", () => {
    it("returns null if agent is not RUNNING or not BUSY", () => {
      const idleAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          activeTool: "bash",
          activeToolStartedAt: 1000,
        },
      });
      expect(getToolExecutionDuration(idleAgent.activity, 2000)).toBeNull();

      const exitedAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "EXITED",
          activity: "BUSY",
          activeTool: "bash",
          activeToolStartedAt: 1000,
        },
      });
      expect(getToolExecutionDuration(exitedAgent.activity, 2000)).toBeNull();
    });

    it("returns null if activeTool or activeToolStartedAt is missing or non-positive", () => {
      const noToolAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: null,
          activeToolStartedAt: 1000,
        },
      });
      expect(getToolExecutionDuration(noToolAgent.activity, 2000)).toBeNull();

      const noStartAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: "bash",
          activeToolStartedAt: null,
        },
      });
      expect(getToolExecutionDuration(noStartAgent.activity, 2000)).toBeNull();
    });

    it("computes elapsed duration accurately", () => {
      const activity = {
        ...factory_createAgent().activity,
        lifecycle: "RUNNING" as const,
        activity: "BUSY" as const,
        activeTool: "bash",
        activeToolStartedAt: 10000,
      };
      expect(getToolExecutionDuration(activity, 15000)).toBe(5000);
      expect(getToolExecutionDuration(activity, 70000)).toBe(60000);
      expect(getToolExecutionDuration(activity, 9000)).toBe(0); // clock skew floor
    });
  });

  describe("getToolStallTier", () => {
    it("evaluates healthy for < 60s or null", () => {
      expect(getToolStallTier(null)).toBe("healthy");
      expect(getToolStallTier(0)).toBe("healthy");
      expect(getToolStallTier(59999)).toBe("healthy");
    });

    it("evaluates notice for 60s - 119s", () => {
      expect(getToolStallTier(60000)).toBe("notice");
      expect(getToolStallTier(75000)).toBe("notice");
      expect(getToolStallTier(119999)).toBe("notice");
    });

    it("evaluates warning for 120s - 179s", () => {
      expect(getToolStallTier(120000)).toBe("warning");
      expect(getToolStallTier(150000)).toBe("warning");
      expect(getToolStallTier(179999)).toBe("warning");
    });

    it("evaluates stalled for >= 180s (3m+)", () => {
      expect(getToolStallTier(180000)).toBe("stalled");
      expect(getToolStallTier(202000)).toBe("stalled");
      expect(getToolStallTier(600000)).toBe("stalled");
    });
  });

  describe("formatDuration", () => {
    it("formats durations under 60s as seconds", () => {
      expect(formatDuration(45000)).toBe("45s");
      expect(formatDuration(5000)).toBe("5s");
    });

    it("formats durations 60s and above as minutes and seconds", () => {
      expect(formatDuration(75000)).toBe("1m 15s");
      expect(formatDuration(135000)).toBe("2m 15s");
      expect(formatDuration(202000)).toBe("3m 22s");
    });

    it("formats exact minutes without trailing 0s", () => {
      expect(formatDuration(600000)).toBe("10m");
    });

    it("formats hours and hours with minutes", () => {
      expect(formatDuration(7200000)).toBe("2h");
      expect(formatDuration(8100000)).toBe("2h 15m");
    });
  });

  describe("isAgentRequiringAttention with stalled tools", () => {
    it("returns true for a running busy agent with a tool running >= 180s", () => {
      const now = 200000;
      const stalledAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: "bash",
          activeToolStartedAt: now - 185000,
        },
      });
      expect(isAgentRequiringAttention(stalledAgent, now)).toBe(true);
    });

    it("returns false for a running busy agent with a tool running < 180s", () => {
      const now = 200000;
      const warningAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: "bash",
          activeToolStartedAt: now - 150000,
        },
      });
      expect(isAgentRequiringAttention(warningAgent, now)).toBe(false);
    });

    it("returns true for a running busy agent with silence duration >= 120s", () => {
      const now = 200000;
      const silentBusyAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: null,
          activeToolStartedAt: null,
          updatedAt: now - 130000,
        },
      });
      expect(isAgentRequiringAttention(silentBusyAgent, now)).toBe(true);
    });

    it("returns false for a running busy agent with silence duration < 120s", () => {
      const now = 200000;
      const activeBusyAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
          activeTool: null,
          activeToolStartedAt: null,
          updatedAt: now - 30000,
        },
      });
      expect(isAgentRequiringAttention(activeBusyAgent, now)).toBe(false);
    });

    it("returns false for a running IDLE agent with silence duration >= 120s", () => {
      const now = 200000;
      const idleAgent = factory_createAgent({
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
          updatedAt: now - 130000,
        },
      });
      expect(isAgentRequiringAttention(idleAgent, now)).toBe(false);
    });
  });

  describe("isSessionStalled", () => {
    it("returns false when silence duration is below threshold", () => {
      const now = 200000;
      expect(isSessionStalled(now - 30000, now)).toBe(false);
      expect(isSessionStalled(now - 119999, now)).toBe(false);
    });

    it("returns true when silence duration is at or exceeds threshold", () => {
      const now = 1000000;
      expect(isSessionStalled(now - 120000, now)).toBe(true);
      expect(isSessionStalled(now - 300000, now)).toBe(true);
    });

    it("respects custom thresholdMs", () => {
      const now = 200000;
      expect(isSessionStalled(now - 50000, now, 60000)).toBe(false);
      expect(isSessionStalled(now - 65000, now, 60000)).toBe(true);
    });

    it("returns false for invalid timestamps", () => {
      const now = 200000;
      expect(isSessionStalled(0, now)).toBe(false);
      expect(isSessionStalled(-100, now)).toBe(false);
      expect(isSessionStalled(Number.NaN, now)).toBe(false);
    });
  });

  describe("trimProjectTitlePrefix", () => {
    const fixturePath = resolve(__dirname, "../../../../agent-common/testdata/project-prefix-fixtures.json");
    const fixtures: IProjectPrefixFixture[] = JSON.parse(readFileSync(fixturePath, "utf-8"));

    for (const fixture of fixtures) {
      it(fixture.name, () => {
        expect(trimProjectTitlePrefix(fixture.title, fixture.projectName)).toBe(fixture.expected);
      });
    }

    it("handles undefined projectName gracefully", () => {
      expect(trimProjectTitlePrefix("my title", undefined)).toBe("my title");
    });
  });

  describe("getIdleFreshness", () => {
    it("returns 'fresh' when duration is less than 15 minutes", () => {
      const now = 10_000_000;
      expect(getIdleFreshness(now + 5_000, now)).toBe("fresh");
      expect(getIdleFreshness(now - 0, now)).toBe("fresh");
      expect(getIdleFreshness(now - 60_000, now)).toBe("fresh");
      expect(getIdleFreshness(now - (14 * 60 * 1000 + 59 * 1000), now)).toBe("fresh");
      expect(getIdleFreshness(now - (15 * 60 * 1000 - 1), now)).toBe("fresh");
    });

    it("returns 'standard' when duration is between 15 minutes and 2 hours", () => {
      const now = 10_000_000;
      expect(getIdleFreshness(now - 15 * 60 * 1000, now)).toBe("standard");
      expect(getIdleFreshness(now - 30 * 60 * 1000, now)).toBe("standard");
      expect(getIdleFreshness(now - 60 * 60 * 1000, now)).toBe("standard");
      expect(getIdleFreshness(now - (2 * 60 * 60 * 1000 - 1), now)).toBe("standard");
      expect(getIdleFreshness(now - 2 * 60 * 60 * 1000, now)).toBe("standard");
    });

    it("returns 'stale' when duration exceeds 2 hours", () => {
      const now = 10_000_000;
      expect(getIdleFreshness(now - (2 * 60 * 60 * 1000 + 1), now)).toBe("stale");
      expect(getIdleFreshness(now - 3 * 60 * 60 * 1000, now)).toBe("stale");
      expect(getIdleFreshness(now - 72 * 60 * 60 * 1000, now)).toBe("stale");
    });

    it("returns 'stale' for missing, zero, negative, or NaN timestamps", () => {
      const now = 10_000_000;
      expect(getIdleFreshness(0, now)).toBe("stale");
      expect(getIdleFreshness(-1000, now)).toBe("stale");
      expect(getIdleFreshness(Number.NaN, now)).toBe("stale");
      expect(getIdleFreshness(undefined as unknown as number, now)).toBe("stale");
      expect(getIdleFreshness(null as unknown as number, now)).toBe("stale");
    });

    it("uses Date.now() when now parameter is omitted", () => {
      const fiveMinutesAgo = Date.now() - 5 * 60 * 1000;
      expect(getIdleFreshness(fiveMinutesAgo)).toBe("fresh");

      const threeHoursAgo = Date.now() - 3 * 60 * 60 * 1000;
      expect(getIdleFreshness(threeHoursAgo)).toBe("stale");
    });
  });

  describe("isAgentStaleIdle", () => {
    it("returns true only for RUNNING IDLE agents without unseen completion and age > 2h", () => {
      const now = 20_000_000;
      const staleActivity: IAgentActivity = {
        ...factory_createAgent().activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        updatedAt: now - 3 * 3600 * 1000,
      };
      expect(isAgentStaleIdle(staleActivity, now)).toBe(true);

      const warmActivity: IAgentActivity = {
        ...staleActivity,
        updatedAt: now - 5 * 60 * 1000,
      };
      expect(isAgentStaleIdle(warmActivity, now)).toBe(false);

      const standardActivity: IAgentActivity = {
        ...staleActivity,
        updatedAt: now - 2 * 3600 * 1000,
      };
      expect(isAgentStaleIdle(standardActivity, now)).toBe(false);

      const unseenActivity: IAgentActivity = {
        ...staleActivity,
        hasUnseenCompletion: true,
      };
      expect(isAgentStaleIdle(unseenActivity, now)).toBe(false);
      expect(isAgentStaleIdle(unseenActivity, now, true)).toBe(true);

      const busyActivity: IAgentActivity = {
        ...staleActivity,
        activity: "BUSY",
      };
      expect(isAgentStaleIdle(busyActivity, now)).toBe(false);

      const exitedActivity: IAgentActivity = {
        ...staleActivity,
        lifecycle: "EXITED",
      };
      expect(isAgentStaleIdle(exitedActivity, now)).toBe(false);
    });

    it("treats has_unseen_completion: true as unacknowledged completion and prevents stale idle classification even if updatedAt > 2h", () => {
      const now = 20_000_000;
      const snakeCaseUnseenActivity: IAgentActivity = {
        ...factory_createAgent().activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        updatedAt: now - 3 * 3600 * 1000,
        has_unseen_completion: true,
      };
      expect(isAgentStaleIdle(snakeCaseUnseenActivity, now)).toBe(false);
      expect(isAgentStaleIdle(snakeCaseUnseenActivity, now, true)).toBe(true);
    });
  });

  describe("getAgentIdleFreshness", () => {
    it("returns 'fresh' for non-eligible sessions and proper tier for eligible sessions", () => {
      const now = 20_000_000;
      const eligibleFresh: IAgentActivity = {
        ...factory_createAgent().activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        updatedAt: now - 5 * 60 * 1000,
      };
      expect(getAgentIdleFreshness(eligibleFresh, now)).toBe("fresh");

      const eligibleStandard: IAgentActivity = {
        ...eligibleFresh,
        updatedAt: now - 60 * 60 * 1000,
      };
      expect(getAgentIdleFreshness(eligibleStandard, now)).toBe("standard");

      const eligibleStale: IAgentActivity = {
        ...eligibleFresh,
        updatedAt: now - 3 * 3600 * 1000,
      };
      expect(getAgentIdleFreshness(eligibleStale, now)).toBe("stale");

      const unseenActivity: IAgentActivity = {
        ...eligibleStale,
        hasUnseenCompletion: true,
      };
      expect(getAgentIdleFreshness(unseenActivity, now)).toBe("fresh");
      expect(getAgentIdleFreshness(unseenActivity, now, true)).toBe("stale");

      const exitedActivity: IAgentActivity = {
        ...eligibleStale,
        lifecycle: "EXITED",
      };
      expect(getAgentIdleFreshness(exitedActivity, now)).toBe("fresh");
    });

    it("treats has_unseen_completion: true as unacknowledged completion preventing stale tier", () => {
      const now = 20_000_000;
      const snakeCaseUnseenActivity: IAgentActivity = {
        ...factory_createAgent().activity,
        lifecycle: "RUNNING",
        activity: "IDLE",
        updatedAt: now - 3 * 3600 * 1000,
        has_unseen_completion: true,
      };
      expect(getAgentIdleFreshness(snakeCaseUnseenActivity, now)).toBe("fresh");
      expect(getAgentIdleFreshness(snakeCaseUnseenActivity, now, true)).toBe("stale");
    });
  });

  describe("idle freshness shared fixtures", () => {
    const fixturePath = resolve(__dirname, "../../../../agent-common/testdata/idle-freshness-fixtures.json");
    const fixtures: IIdleFreshnessFixture[] = JSON.parse(readFileSync(fixturePath, "utf-8"));

    for (const fixture of fixtures) {
      it(fixture.name, () => {
        const activity: IAgentActivity = {
          ...factory_createAgent().activity,
          lifecycle: fixture.lifecycle,
          activity: fixture.activity,
          hasUnseenCompletion: fixture.hasUnseenCompletion,
          updatedAt: fixture.updatedAt,
        };

        expect(getAgentIdleFreshness(activity, fixture.nowMs)).toBe(fixture.expectedTier);
        expect(isAgentStaleIdle(activity, fixture.nowMs)).toBe(fixture.expectedStaleIdle);
      });
    }
  });

  describe("formatRateLimitCallout", () => {
    it("formats remaining seconds when resetAt is in the future", () => {
      const now = 1_000_000;
      const activity = {
        ...factory_createAgent().activity,
        activity: "RATE_LIMITED" as const,
        lastError: "Rate limit exceeded",
        rateLimitResetAt: now + 45_000,
      };
      expect(formatRateLimitCallout(activity, now)).toBe("Rate limit exceeded (retry in 45s)");
    });

    it("does not format wait string or fall through to retryAfter when resetAt is in the past", () => {
      const now = 1_000_000;
      const activity = {
        ...factory_createAgent().activity,
        activity: "RATE_LIMITED" as const,
        lastError: "Rate limit exceeded",
        rateLimitResetAt: now - 5000,
        retryAfterSeconds: 60,
      };
      // Must NOT fall through to retryAfterSeconds (the backoff has already elapsed)
      expect(formatRateLimitCallout(activity, now)).toBe("Rate limit exceeded");
    });

    it("computes remaining seconds relative to updatedAt when only retryAfter is present", () => {
      const now = 1_000_000;
      const activity = {
        ...factory_createAgent().activity,
        activity: "RATE_LIMITED" as const,
        lastError: "Rate limit exceeded",
        retryAfterSeconds: 60,
        updatedAt: now - 15_000, // 15 seconds elapsed
      };
      expect(formatRateLimitCallout(activity, now)).toBe("Rate limit exceeded (retry in 45s)");
    });

    it("does not format wait string when only retryAfter is present and has already elapsed", () => {
      const now = 1_000_000;
      const activity = {
        ...factory_createAgent().activity,
        activity: "RATE_LIMITED" as const,
        lastError: "Rate limit exceeded",
        retryAfterSeconds: 30,
        updatedAt: now - 45_000, // 45 seconds elapsed (> 30s)
      };
      expect(formatRateLimitCallout(activity, now)).toBe("Rate limit exceeded");
    });

    it("formats fallback message when lastError is not set and wait is remaining", () => {
      const now = 1_000_000;
      const activity = {
        ...factory_createAgent().activity,
        activity: "RATE_LIMITED" as const,
        rateLimitResetAt: now + 30_000,
      };
      expect(formatRateLimitCallout(activity, now)).toBe("Rate limit reached — retry in 30s");
    });

    it("formats fallback message without wait when lastError is not set and resetAt has elapsed", () => {
      const now = 1_000_000;
      const activity = {
        ...factory_createAgent().activity,
        activity: "RATE_LIMITED" as const,
        rateLimitResetAt: now - 1000,
      };
      expect(formatRateLimitCallout(activity, now)).toBe("Rate limit reached — backoff/retry in progress");
    });

    it("formats durations <= 120s in seconds and durations > 120s humanized", () => {
      const now = 1_000_000;
      const baseActivity = {
        ...factory_createAgent().activity,
        activity: "RATE_LIMITED" as const,
        lastError: "Rate limit exceeded",
      };

      // Exactly 120s remaining -> "retry in 120s"
      expect(formatRateLimitCallout({ ...baseActivity, rateLimitResetAt: now + 120_000 }, now)).toBe(
        "Rate limit exceeded (retry in 120s)",
      );

      // 121s remaining (2m 1s) -> "retry in 2m 1s"
      expect(formatRateLimitCallout({ ...baseActivity, rateLimitResetAt: now + 121_000 }, now)).toBe(
        "Rate limit exceeded (retry in 2m 1s)",
      );

      // 600s remaining (10m) -> "retry in 10m"
      expect(formatRateLimitCallout({ ...baseActivity, rateLimitResetAt: now + 600_000 }, now)).toBe(
        "Rate limit exceeded (retry in 10m)",
      );

      // 7200s remaining (2h) -> "retry in 2h"
      expect(formatRateLimitCallout({ ...baseActivity, rateLimitResetAt: now + 7_200_000 }, now)).toBe(
        "Rate limit exceeded (retry in 2h)",
      );

      // retryAfterSeconds exceeding 120s (600s, 0 elapsed) -> "retry in 10m"
      expect(formatRateLimitCallout({ ...baseActivity, retryAfterSeconds: 600, updatedAt: now }, now)).toBe(
        "Rate limit exceeded (retry in 10m)",
      );
    });
  });
});
