import React, { useState, useMemo } from "react";
import { useEventStream } from "../hooks/useEventStream";
import type { IAgentState, GroupByOption } from "../types";
import { Header } from "../components/Header";
import { AttentionBanner } from "../components/AttentionBanner";
import { ProjectGroup } from "../components/ProjectGroup";
import { EnvironmentGroup } from "../components/EnvironmentGroup";
import { LogDrawer } from "../components/LogDrawer";
import { groupAgentsByProject, groupAgentsByEnvironment } from "../utils/groupingUtils";
import { ViewSwitcher } from "../components/ViewSwitcher";
import { Layers, Server } from "lucide-react";

export function App(): React.JSX.Element {
  const { agents, environments, isConnected, error } = useEventStream();
  const [groupBy, setGroupBy] = useState<GroupByOption>("project");
  const [selectedAgent, setSelectedAgent] = useState<IAgentState | null>(null);

  const projectGroups = useMemo(() => groupAgentsByProject(agents), [agents]);
  const environmentGroups = useMemo(() => groupAgentsByEnvironment(agents, environments), [agents, environments]);

  return (
    <div data-testid="App" className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-sans">
      <Header agents={agents} environments={environments} isConnected={isConnected} />

      <main data-testid="App--main" className="flex-1 max-w-7xl w-full mx-auto p-4 md:p-6">
        {error && (
          <div
            data-testid="App--error"
            className="mb-4 p-3 rounded-lg bg-red-950/50 border border-red-800/80 text-xs text-red-300 font-mono"
          >
            {error}
          </div>
        )}

        {/* Attention Banner */}
        <AttentionBanner agents={agents} onSelectAgent={setSelectedAgent} />

        {/* View Switcher Controls */}
        <div
          data-testid="App--controls"
          className="flex items-center justify-between mb-6 pb-2 border-b border-slate-800"
        >
<<<<<<< HEAD
          <ViewSwitcher<GroupByOption>
            options={[
              {
                id: "project",
                label: "Group by Project",
                icon: <Layers size={14} />,
                testId: "App--group-by-project",
              },
              {
                id: "environment",
                label: "Group by Environment",
                icon: <Server size={14} />,
                testId: "App--group-by-environment",
              },
            ]}
            value={groupBy}
            onChange={setGroupBy}
          />
=======
          <div className="flex items-center gap-2">
            <button
              type="button"
              aria-label="Group by project"
              data-testid="App--group-by-project"
              onClick={() => setGroupBy("project")}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors ${
                groupBy === "project"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "bg-slate-900 text-slate-400 hover:text-slate-200 border border-slate-800"
              }`}
            >
              <span className="w-3.5 h-3.5 flex items-center justify-center">
                <Layers size={14} />
              </span>
              Group by Project
            </button>

            <button
              type="button"
              aria-label="Group by environment"
              data-testid="App--group-by-environment"
              onClick={() => setGroupBy("environment")}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors ${
                groupBy === "environment"
                  ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                  : "bg-slate-900 text-slate-400 hover:text-slate-200 border border-slate-800"
              }`}
            >
              <span className="w-3.5 h-3.5 flex items-center justify-center">
                <Server size={14} />
              </span>
              Group by Environment
            </button>
          </div>
>>>>>>> issue-21

          <span data-testid="App--agent-count" className="text-xs text-slate-500 font-mono">
            {agents.length} Total Agent{agents.length !== 1 ? "s" : ""}
          </span>
        </div>

        {/* Content Listing */}
        {agents.length === 0 ? (
          <div
            data-testid="App--empty-state"
            className="text-center py-20 border border-dashed border-slate-800 rounded-2xl"
          >
            <p className="text-sm text-slate-400 font-medium">No active or recent agent runs detected.</p>
            <p className="text-xs text-slate-600 mt-1">Start Claude Code or Pi in any observed environment.</p>
          </div>
        ) : groupBy === "project" ? (
          projectGroups.map((group) => (
            <ProjectGroup
              key={group.id}
              projectName={group.name}
              projectId={group.id}
              agents={group.agents}
              onSelectAgent={setSelectedAgent}
            />
          ))
        ) : (
          environmentGroups.map((group) => (
            <EnvironmentGroup
              key={group.environment.id}
              environment={group.environment}
              agents={group.agents}
              onSelectAgent={setSelectedAgent}
            />
          ))
        )}
      </main>

      {/* Slide-over Log Drawer */}
      <LogDrawer agent={selectedAgent} onClose={() => setSelectedAgent(null)} />
    </div>
  );
}
