import { describe, it, expect } from "bun:test";
import { calculateTotalSpend, formatCurrency, filterActiveAgents, isAgentRequiringAttention } from "../agentUtils";
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
    it("returns only agents with RUNNING lifecycle", () => {
      const runningAgent = factory_createAgent({
        id: "running-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "RUNNING",
        },
      });
      const exitedAgent = factory_createAgent({
        id: "exited-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "EXITED",
        },
      });

      const active = filterActiveAgents([runningAgent, exitedAgent]);
      expect(active.length).toBe(1);
      expect(active[0]?.id).toBe("running-1");
    });

    it("returns empty array when no agents are running", () => {
      const exitedAgent = factory_createAgent({
        id: "exited-1",
        activity: {
          ...factory_createAgent().activity,
          lifecycle: "EXITED",
        },
      });

      const active = filterActiveAgents([exitedAgent]);
      expect(active).toEqual([]);
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
});
