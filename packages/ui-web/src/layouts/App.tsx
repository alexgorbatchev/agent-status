import React, { useState, useMemo, useCallback } from "react";
import { useEventStream } from "../hooks/useEventStream";
import type { IAgentState, GroupByOption } from "../types";
import { Header } from "../components/Header";
import { AttentionBanner } from "../components/AttentionBanner";
import { ProjectGroup } from "../components/ProjectGroup";
import { EnvironmentGroup } from "../components/EnvironmentGroup";
import { LogDrawer } from "../components/LogDrawer";
import { StateHelpDialog } from "../components/StateHelpDialog";
import { groupAgentsByProject, groupAgentsByEnvironment } from "../utils/groupingUtils";
import { filterActiveAgents } from "../utils/agentUtils";
import { ViewSwitcher } from "../components/ViewSwitcher";
import { IconWrapper } from "../components/IconWrapper";
import { Layers, Server, Eye, EyeOff, Activity } from "lucide-react";
import { DEFAULT_DEV_TOKEN, resolveApiBaseUrl } from "../utils/config";
import { clsx } from "clsx";

export interface IAppProps {
  apiBaseUrl?: string;
  token?: string;
}

export function App({ apiBaseUrl, token = DEFAULT_DEV_TOKEN }: IAppProps = {}): React.JSX.Element {
  const resolvedBaseUrl = resolveApiBaseUrl(apiBaseUrl);
  const { agents, environments, isConnected, error, updateAgent } = useEventStream(apiBaseUrl, token);
  const [groupBy, setGroupBy] = useState<GroupByOption>("project");
  const [selectedAgent, setSelectedAgent] = useState<IAgentState | null>(null);
  const [isHelpOpen, setIsHelpOpen] = useState(false);
  const [showHidden, setShowHidden] = useState<boolean>(false);
  const [showFinished, setShowFinished] = useState<boolean>(false);
  const [isActiveFilter, setIsActiveFilter] = useState<boolean>(false);

  const hiddenCount = useMemo(() => {
    return agents.filter((a) => a.isHidden).length;
  }, [agents]);

  const finishedCount = useMemo(() => {
    return agents.filter((a) => !a.isHidden && a.activity.lifecycle === "EXITED").length;
  }, [agents]);

  const visibleAgents = useMemo(() => {
    return agents.filter((a) => {
      if (a.isHidden) {
        return showHidden;
      }
      const isFinished = a.activity.lifecycle === "EXITED";
      if (isFinished) {
        return showFinished;
      }
      return true;
    });
  }, [agents, showHidden, showFinished]);

  const displayedAgents = useMemo(() => {
    if (!isActiveFilter) return visibleAgents;
    return filterActiveAgents(visibleAgents);
  }, [visibleAgents, isActiveFilter]);

  const handleToggleHide = useCallback(
    async (agent: IAgentState) => {
      const nextHidden = !agent.isHidden;
      updateAgent?.({ ...agent, isHidden: nextHidden });

      try {
        const response = await fetch(`${resolvedBaseUrl}/api/v1/agents/${encodeURIComponent(agent.id)}`, {
          method: "PATCH",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify({ isHidden: nextHidden }),
        });
        if (!response.ok) {
          throw new Error(`Failed to update visibility: ${response.statusText}`);
        }
      } catch (err) {
        console.error("Failed to toggle agent visibility:", err);
        // Revert optimistic update on error
        updateAgent?.({ ...agent, isHidden: agent.isHidden });
      }
    },
    [resolvedBaseUrl, token, updateAgent],
  );

  const projectGroups = useMemo(
    () => groupAgentsByProject(displayedAgents).filter((g) => g.agents.length > 0),
    [displayedAgents],
  );
  const environmentGroups = useMemo(
    () => groupAgentsByEnvironment(displayedAgents, environments).filter((g) => g.agents.length > 0),
    [displayedAgents, environments],
  );

  return (
    <div data-testid="App" className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-sans">
      <Header
        agents={visibleAgents}
        environments={environments}
        isConnected={isConnected}
        isActiveFilter={isActiveFilter}
        onToggleActiveFilter={() => setIsActiveFilter((prev) => !prev)}
        onOpenHelp={() => setIsHelpOpen(true)}
      />

      <main data-testid="App--main" className="flex-1 max-w-7xl w-full mx-auto p-4 md:p-6">
        {error && (
          <div data-testid="App--error" className="mb-4 p-3 rounded-lg bg-red-600 text-white text-xs font-mono">
            {error}
          </div>
        )}

        {/* Attention Banner */}
        <AttentionBanner agents={visibleAgents} onSelectAgent={setSelectedAgent} />

        {/* View Switcher Controls */}
        <div
          data-testid="App--controls"
          className="flex items-center justify-between mb-6 pb-2 border-b border-slate-800"
        >
          <div className="flex items-center gap-3">
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

            {hiddenCount > 0 && (
              <button
                type="button"
                data-testid="App--toggle-show-hidden"
                onClick={() => setShowHidden((prev) => !prev)}
                className={clsx(
                  "flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors cursor-pointer border",
                  showHidden
                    ? "bg-blue-600 text-white border-blue-500 shadow-md shadow-blue-600/30"
                    : "bg-slate-900 text-slate-400 hover:text-slate-200 border-slate-800",
                )}
              >
                <IconWrapper size="sm">{showHidden ? <Eye size={14} /> : <EyeOff size={14} />}</IconWrapper>
                <span>{showHidden ? `Hide ${hiddenCount} hidden` : `Show ${hiddenCount} hidden`}</span>
              </button>
            )}

            {finishedCount > 0 && (
              <button
                type="button"
                data-testid="App--toggle-show-finished"
                onClick={() => setShowFinished((prev) => !prev)}
                className={clsx(
                  "flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors cursor-pointer border",
                  showFinished
                    ? "bg-blue-600 text-white border-blue-500 shadow-md shadow-blue-600/30"
                    : "bg-slate-900 text-slate-400 hover:text-slate-200 border-slate-800",
                )}
              >
                <IconWrapper size="sm">{showFinished ? <Eye size={14} /> : <EyeOff size={14} />}</IconWrapper>
                <span>{showFinished ? `Hide ${finishedCount} finished` : `Show ${finishedCount} finished`}</span>
              </button>
            )}
          </div>

          <span data-testid="App--agent-count" className="text-xs text-slate-500 font-mono">
            {isActiveFilter
              ? `${displayedAgents.length} Active of ${visibleAgents.length} Agent${visibleAgents.length !== 1 ? "s" : ""}`
              : `${visibleAgents.length} Agent${visibleAgents.length !== 1 ? "s" : ""}`}
            {hiddenCount > 0 && !showHidden && !isActiveFilter ? ` (${hiddenCount} hidden)` : ""}
          </span>
        </div>

        {/* Content Listing */}
        {isActiveFilter && displayedAgents.length === 0 ? (
          <div
            data-testid="App--no-active-empty-state"
            className="flex flex-col items-center justify-center py-24 px-4 text-center border border-dashed border-slate-800/80 rounded-2xl bg-slate-950/40"
          >
            <div className="p-4 rounded-full bg-slate-900/80 border border-slate-800 text-slate-500 mb-4">
              <Activity size={36} />
            </div>
            <h2 className="text-2xl md:text-3xl font-extrabold text-slate-100 tracking-tight">Nothing to see here</h2>
            <p className="text-sm text-slate-400 mt-2 max-w-md">No agents are currently active in your fleet.</p>
            <button
              type="button"
              data-testid="App--reset-active-filter"
              onClick={() => setIsActiveFilter(false)}
              className="mt-6 px-4 py-2 text-xs font-medium rounded-lg bg-slate-900 border border-slate-700 text-slate-200 hover:bg-slate-800 hover:border-slate-600 transition-colors cursor-pointer"
            >
              Show all agents
            </button>
          </div>
        ) : visibleAgents.length === 0 ? (
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
              treeGroup={group}
              onSelectAgent={setSelectedAgent}
              onToggleHide={handleToggleHide}
            />
          ))
        ) : (
          environmentGroups.map((group) => (
            <EnvironmentGroup
              key={group.environment.id}
              environment={group.environment}
              agents={group.agents}
              onSelectAgent={setSelectedAgent}
              onToggleHide={handleToggleHide}
            />
          ))
        )}
      </main>

      {/* Slide-over Log Drawer */}
      <LogDrawer agent={selectedAgent} onClose={() => setSelectedAgent(null)} />

      {/* State & Lifecycle Help Modal */}
      {isHelpOpen && <StateHelpDialog onClose={() => setIsHelpOpen(false)} />}
    </div>
  );
}
