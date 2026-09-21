import { describe, it, expect } from "bun:test";
import { groupAgentsByProject, groupAgentsByEnvironment, compareAgentsByActivity, sortAgents } from "../groupingUtils";
import { factory_createAgent, fixture_testEnvironment, fixture_testStaleEnvironment } from "./fixtures";
import type { IEnvironmentInfo } from "../../types";

describe("groupingUtils", () => {
  describe("groupAgentsByProject", () => {
    it("returns empty array when agents list is empty", () => {
      const groups = groupAgentsByProject([]);
      expect(groups).toEqual([]);
    });

    it("groups agents by project ID", () => {
      const agent1 = factory_createAgent({
        id: "a1",
        project: { id: "proj-1", name: "Project One", canonicalRoot: "/path/1" },
      });
      const agent2 = factory_createAgent({
        id: "a2",
        project: { id: "proj-1", name: "Project One", canonicalRoot: "/path/1" },
      });
      const agent3 = factory_createAgent({
        id: "a3",
        project: { id: "proj-2", name: "Project Two", canonicalRoot: "/path/2" },
      });

      const groups = groupAgentsByProject([agent1, agent2, agent3]);
      expect(groups.length).toBe(2);

      const group1 = groups.find((g) => g.id === "proj-1");
      expect(group1?.name).toBe("Project One");
      expect(group1?.agents.length).toBe(2);

      const group2 = groups.find((g) => g.id === "proj-2");
      expect(group2?.name).toBe("Project Two");
      expect(group2?.agents.length).toBe(1);
    });

    it("handles fallback project id and name when missing", () => {
      const agentWithoutId = factory_createAgent({
        id: "a1",
        project: { id: "", name: "", canonicalRoot: "/path" },
      });

      const groups = groupAgentsByProject([agentWithoutId]);
      expect(groups.length).toBe(1);
      expect(groups[0]?.id).toBe("unknown");
      expect(groups[0]?.name).toBe("unknown");
    });

    it("sorts agents within each project group by activity priority (BUSY > WAITING_QUESTION > ERROR > IDLE) and updatedAt desc", () => {
      const baseProject = { id: "proj-1", name: "Project One", canonicalRoot: "/path/1" };
      const idleOlder = factory_createAgent({
        id: "idle-old",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "IDLE",
          updatedAt: 1000,
        },
      });
      const idleNewer = factory_createAgent({
        id: "idle-new",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "IDLE",
          updatedAt: 5000,
        },
      });
      const errorAgent = factory_createAgent({
        id: "error",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "ERROR",
          updatedAt: 2000,
        },
      });
      const waitingAgent = factory_createAgent({
        id: "waiting",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "WAITING_QUESTION",
          updatedAt: 3000,
        },
      });
      const busyOlder = factory_createAgent({
        id: "busy-old",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "BUSY",
          updatedAt: 4000,
        },
      });
      const busyNewer = factory_createAgent({
        id: "busy-new",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "BUSY",
          updatedAt: 6000,
        },
      });

      // Pass in intentionally non-sorted order
      const groups = groupAgentsByProject([idleOlder, idleNewer, errorAgent, waitingAgent, busyOlder, busyNewer]);
      expect(groups.length).toBe(1);

      const agentIds = groups[0]?.agents.map((a) => a.id);
      expect(agentIds).toEqual(["busy-new", "busy-old", "waiting", "error", "idle-new", "idle-old"]);
    });
  });

  describe("groupAgentsByEnvironment", () => {
    it("returns environments with empty agent lists when no agents provided", () => {
      const environments: IEnvironmentInfo[] = [fixture_testEnvironment, fixture_testStaleEnvironment];
      const groups = groupAgentsByEnvironment([], environments);

      expect(groups.length).toBe(2);
      expect(groups[0]?.environment.id).toBe(fixture_testEnvironment.id);
      expect(groups[0]?.agents).toEqual([]);
      expect(groups[1]?.environment.id).toBe(fixture_testStaleEnvironment.id);
      expect(groups[1]?.agents).toEqual([]);
    });

    it("groups agents by environment id and preserves environment order", () => {
      const agentEnv1 = factory_createAgent({
        id: "a1",
        environment: fixture_testEnvironment,
      });
      const agentEnv2 = factory_createAgent({
        id: "a2",
        environment: fixture_testStaleEnvironment,
      });
      const agentEnv1Second = factory_createAgent({
        id: "a3",
        environment: fixture_testEnvironment,
      });

      const environments: IEnvironmentInfo[] = [fixture_testEnvironment, fixture_testStaleEnvironment];
      const groups = groupAgentsByEnvironment([agentEnv1, agentEnv2, agentEnv1Second], environments);

      expect(groups.length).toBe(2);
      expect(groups[0]?.environment.id).toBe(fixture_testEnvironment.id);
      expect(groups[0]?.agents.length).toBe(2);
      expect(groups[1]?.environment.id).toBe(fixture_testStaleEnvironment.id);
      expect(groups[1]?.agents.length).toBe(1);
    });

    it("includes environments referenced by agents even if not present in initial environments array", () => {
      const unseenEnv: IEnvironmentInfo = {
        id: "env-unseen",
        label: "unseen-box",
        hostname: "unseen.internal",
        platform: "linux",
        user: "root",
        isStale: false,
        lastHeartbeatAt: Date.now(),
      };
      const agent = factory_createAgent({
        id: "a-unseen",
        environment: unseenEnv,
      });

      const groups = groupAgentsByEnvironment([agent], [fixture_testEnvironment]);
      expect(groups.length).toBe(2);
      expect(groups[0]?.environment.id).toBe(fixture_testEnvironment.id);
      expect(groups[0]?.agents).toEqual([]);
      expect(groups[1]?.environment.id).toBe("env-unseen");
      expect(groups[1]?.agents.length).toBe(1);
    });

    it("sorts agents within each environment group by activity priority and updatedAt desc", () => {
      const idleAgent = factory_createAgent({
        id: "env1-idle",
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          activity: "IDLE",
          updatedAt: 9000,
        },
      });
      const busyAgent = factory_createAgent({
        id: "env1-busy",
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          activity: "BUSY",
          updatedAt: 1000,
        },
      });

      const groups = groupAgentsByEnvironment([idleAgent, busyAgent], [fixture_testEnvironment]);
      const agentIds = groups[0]?.agents.map((a) => a.id);
      expect(agentIds).toEqual(["env1-busy", "env1-idle"]);
    });
  });

  describe("compareAgentsByActivity and sortAgents", () => {
    it("sorts agents strictly by activity priority then updatedAt descending", () => {
      const aIdle = factory_createAgent({
        id: "idle",
        activity: { ...factory_createAgent().activity, activity: "IDLE", updatedAt: 5000 },
      });
      const aWaiting = factory_createAgent({
        id: "waiting",
        activity: { ...factory_createAgent().activity, activity: "WAITING_QUESTION", updatedAt: 2000 },
      });
      const aError = factory_createAgent({
        id: "error",
        activity: { ...factory_createAgent().activity, activity: "ERROR", updatedAt: 3000 },
      });
      const aBusy1 = factory_createAgent({
        id: "busy-1",
        activity: { ...factory_createAgent().activity, activity: "BUSY", updatedAt: 1000 },
      });
      const aBusy2 = factory_createAgent({
        id: "busy-2",
        activity: { ...factory_createAgent().activity, activity: "BUSY", updatedAt: 4000 },
      });

      const sorted = sortAgents([aIdle, aWaiting, aError, aBusy1, aBusy2]);
      expect(sorted.map((a) => a.id)).toEqual(["busy-2", "busy-1", "waiting", "error", "idle"]);
    });

    it("handles identical activity and timestamp safely", () => {
      const a1 = factory_createAgent({
        id: "a1",
        activity: { ...factory_createAgent().activity, activity: "BUSY", updatedAt: 1000 },
      });
      const a2 = factory_createAgent({
        id: "a2",
        activity: { ...factory_createAgent().activity, activity: "BUSY", updatedAt: 1000 },
      });

      expect(compareAgentsByActivity(a1, a2)).toBe(0);
    });

    it("falls back to lower priority for unknown activity states", () => {
      const aUnknown = factory_createAgent({
        id: "unknown",
        // @ts-expect-error test unknown activity state fallback
        activity: { ...factory_createAgent().activity, activity: "SOMETHING_ELSE", updatedAt: 5000 },
      });
      const aIdle = factory_createAgent({
        id: "idle",
        activity: { ...factory_createAgent().activity, activity: "IDLE", updatedAt: 1000 },
      });

      const sorted = sortAgents([aUnknown, aIdle]);
      expect(sorted.map((a) => a.id)).toEqual(["idle", "unknown"]);
    });
  });
});
