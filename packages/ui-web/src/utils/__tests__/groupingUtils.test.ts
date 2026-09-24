import { describe, it, expect } from "bun:test";
import {
  groupAgentsByProject,
  groupAgentsByEnvironment,
  getAgentCategory,
  groupAgentsByCategory,
} from "../groupingUtils";
import { factory_createAgent, fixture_testEnvironment, fixture_testStaleEnvironment } from "./fixtures";
import type { IEnvironmentInfo } from "../../types";

describe("groupingUtils", () => {
  describe("getAgentCategory", () => {
    it("returns finished for EXITED lifecycle regardless of activity state", () => {
      const exitedIdle = factory_createAgent({
        id: "exited-idle",
        activity: { ...factory_createAgent().activity, lifecycle: "EXITED", activity: "IDLE" },
      });
      const exitedBusy = factory_createAgent({
        id: "exited-busy",
        activity: { ...factory_createAgent().activity, lifecycle: "EXITED", activity: "BUSY" },
      });
      const exitedError = factory_createAgent({
        id: "exited-error",
        activity: { ...factory_createAgent().activity, lifecycle: "EXITED", activity: "ERROR" },
      });

      expect(getAgentCategory(exitedIdle)).toBe("finished");
      expect(getAgentCategory(exitedBusy)).toBe("finished");
      expect(getAgentCategory(exitedError)).toBe("finished");
    });

    it("returns idle for RUNNING lifecycle with IDLE activity", () => {
      const runningIdle = factory_createAgent({
        id: "running-idle",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "IDLE" },
      });

      expect(getAgentCategory(runningIdle)).toBe("idle");
    });

    it("returns busy for RUNNING lifecycle with BUSY, WAITING_QUESTION, or ERROR activity", () => {
      const runningBusy = factory_createAgent({
        id: "running-busy",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "BUSY" },
      });
      const runningWaiting = factory_createAgent({
        id: "running-waiting",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "WAITING_QUESTION" },
      });
      const runningError = factory_createAgent({
        id: "running-error",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "ERROR" },
      });

      expect(getAgentCategory(runningBusy)).toBe("busy");
      expect(getAgentCategory(runningWaiting)).toBe("busy");
      expect(getAgentCategory(runningError)).toBe("busy");
    });
  });

  describe("groupAgentsByCategory", () => {
    it("returns empty arrays for all categories when no agents provided", () => {
      const result = groupAgentsByCategory([]);
      expect(result).toEqual({ busy: [], idle: [], finished: [] });
    });

    it("groups agents by busy, idle, and finished while preserving arrival order without reordering on updatedAt", () => {
      const busy1 = factory_createAgent({
        id: "busy-1",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "BUSY", updatedAt: 1000 },
      });
      const idle1 = factory_createAgent({
        id: "idle-1",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "IDLE", updatedAt: 9000 },
      });
      const finished1 = factory_createAgent({
        id: "finished-1",
        activity: { ...factory_createAgent().activity, lifecycle: "EXITED", activity: "IDLE", updatedAt: 2000 },
      });
      const busy2 = factory_createAgent({
        id: "busy-2",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "BUSY", updatedAt: 5000 },
      });
      const idle2 = factory_createAgent({
        id: "idle-2",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "IDLE", updatedAt: 3000 },
      });

      const result = groupAgentsByCategory([busy1, idle1, finished1, busy2, idle2]);
      expect(result.busy.map((a) => a.id)).toEqual(["busy-1", "busy-2"]);
      expect(result.idle.map((a) => a.id)).toEqual(["idle-1", "idle-2"]);
      expect(result.finished.map((a) => a.id)).toEqual(["finished-1"]);
    });

    it("sorts idle agents descending by updatedAt so fresher idle agents appear above stale idle agents", () => {
      const staleIdle = factory_createAgent({
        id: "idle-stale",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "IDLE", updatedAt: 1000 },
      });
      const freshIdle = factory_createAgent({
        id: "idle-fresh",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "IDLE", updatedAt: 9000 },
      });
      const midIdle = factory_createAgent({
        id: "idle-mid",
        activity: { ...factory_createAgent().activity, lifecycle: "RUNNING", activity: "IDLE", updatedAt: 5000 },
      });

      const result = groupAgentsByCategory([staleIdle, freshIdle, midIdle]);
      expect(result.idle.map((a) => a.id)).toEqual(["idle-fresh", "idle-mid", "idle-stale"]);
    });
  });

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

    it("preserves incoming agent order within each project group without reordering on recent activity", () => {
      const baseProject = { id: "proj-1", name: "Project One", canonicalRoot: "/path/1" };
      const agentFirst = factory_createAgent({
        id: "agent-first",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "IDLE",
          updatedAt: 1000,
        },
      });
      const agentSecond = factory_createAgent({
        id: "agent-second",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "BUSY",
          updatedAt: 9000,
        },
      });
      const agentThird = factory_createAgent({
        id: "agent-third",
        project: baseProject,
        activity: {
          ...factory_createAgent().activity,
          activity: "WAITING_QUESTION",
          updatedAt: 500,
        },
      });

      const groups = groupAgentsByProject([agentFirst, agentSecond, agentThird]);
      expect(groups.length).toBe(1);

      const agentIds = groups[0]?.agents.map((a) => a.id);
      expect(agentIds).toEqual(["agent-first", "agent-second", "agent-third"]);
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

    it("preserves incoming agent order within each environment group without reordering on recent activity", () => {
      const idleOlder = factory_createAgent({
        id: "env1-idle-older",
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          activity: "IDLE",
          updatedAt: 1000,
        },
      });
      const busyNewer = factory_createAgent({
        id: "env1-busy-newer",
        environment: fixture_testEnvironment,
        activity: {
          ...factory_createAgent().activity,
          activity: "BUSY",
          updatedAt: 9000,
        },
      });

      const groups = groupAgentsByEnvironment([idleOlder, busyNewer], [fixture_testEnvironment]);
      const agentIds = groups[0]?.agents.map((a) => a.id);
      expect(agentIds).toEqual(["env1-idle-older", "env1-busy-newer"]);
    });
  });
});
