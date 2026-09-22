import { describe, it, expect } from "bun:test";
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
} from "../agentUtils";
import { factory_createAgent, fixture_testEnvironment, fixture_testStaleEnvironment } from "./fixtures";
import type { IAgentState } from "../../types";

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

    it("returns false for RUNNING agent with BUSY or IDLE activity", () => {
      const busyAgent: IAgentState = factory_createAgent({
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "BUSY",
        },
      });
      const idleAgent: IAgentState = factory_createAgent({
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
          activity: "IDLE",
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
  });
});
