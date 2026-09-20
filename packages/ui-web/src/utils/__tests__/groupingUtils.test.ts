import { describe, it, expect } from "bun:test";
import { groupAgentsByProject, groupAgentsByEnvironment } from "../groupingUtils";
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
  });
});
