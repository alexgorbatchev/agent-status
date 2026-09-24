import React from "react";
import { describe, it, expect } from "bun:test";
import { renderToString } from "react-dom/server";
import { AgentGroupSection } from "../components/AgentGroupSection";
import { EnvironmentGroup } from "../components/EnvironmentGroup";
import { ProjectGroup } from "../components/ProjectGroup";
import {
  fixture_agentStateRunning,
  fixture_agentStateWaitingQuestion,
  fixture_agentStateExited,
  fixture_environmentInfo,
  fixture_staleEnvironmentInfo,
} from "./fixtures";
import { FolderGit2 } from "lucide-react";
import type { IAgentState } from "../types";

describe("AgentGroupSection Component", () => {
  it("renders with title, subtitle, active count and spend", () => {
    const html = renderToString(
      <AgentGroupSection
        icon={<FolderGit2 size={16} />}
        iconColorClass="text-blue-400"
        title="Custom Project"
        subtitle="(custom-id)"
        agents={[fixture_agentStateRunning, fixture_agentStateWaitingQuestion]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="AgentGroupSection"');
    expect(html).toContain('data-testid="AgentGroupSection--header"');
    expect(html).toContain('data-testid="AgentGroupSection--title"');
    expect(html).toContain("Custom Project");
    expect(html).toContain("(custom-id)");
    expect(html).toContain('data-testid="AgentGroupSection--spend"');
    expect(html).toContain("1 active");
    expect(html).toContain('data-testid="AgentGroupSection--grid"');
  });

  it("renders with default values and empty agents list", () => {
    const html = renderToString(
      <AgentGroupSection
        icon={<FolderGit2 size={16} />}
        title="Default Project"
        subtitle="(default-id)"
        agents={[]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="AgentGroupSection"');
    expect(html).toContain('data-testid="AgentGroupSection--header"');
    expect(html).toContain('data-testid="AgentGroupSection--title"');
    expect(html).toContain("Default Project");
    expect(html).toContain('data-testid="AgentGroupSection--spend"');
    expect(html).toContain("$0.000");
    expect(html).toContain("invisible");
    expect(html).toContain("0 active");
  });

  it("renders custom badges slot when provided", () => {
    const customBadge = <span data-testid="custom-badge">Special Tag</span>;
    const html = renderToString(
      <AgentGroupSection
        icon={<FolderGit2 size={16} />}
        title="Badge Project"
        subtitle="(badge-id)"
        badges={customBadge}
        agents={[fixture_agentStateExited]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="custom-badge"');
    expect(html).toContain("Special Tag");
    expect(html).toContain("invisible");
    expect(html).toContain("0 active");
  });

  it("delegates from EnvironmentGroup correctly for live and stale environments", () => {
    const liveHtml = renderToString(
      <EnvironmentGroup
        environment={fixture_environmentInfo}
        agents={[fixture_agentStateRunning]}
        onSelectAgent={() => {}}
      />,
    );

    expect(liveHtml).toContain('data-testid="AgentGroupSection"');
    expect(liveHtml).toContain('data-testid="AgentGroupSection--title"');
    expect(liveHtml).toContain("work-macbook");
    expect(liveHtml).toContain("(alex@macbook.local)");
    expect(liveHtml).toContain("Live");
    expect(liveHtml).toContain("1 active");

    const staleHtml = renderToString(
      <EnvironmentGroup environment={fixture_staleEnvironmentInfo} agents={[]} onSelectAgent={() => {}} />,
    );

    expect(staleHtml).toContain('data-testid="AgentGroupSection"');
    expect(staleHtml).toContain("cloud-builder");
    expect(staleHtml).toContain("(ubuntu@builder.internal)");
    expect(staleHtml).toContain("Stale");
  });

  it("renders ProjectGroup with project header correctly", () => {
    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[fixture_agentStateRunning, fixture_agentStateExited]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="ProjectGroup"');
    expect(html).toContain('data-testid="ProjectGroup--title"');
    expect(html).toContain("agent-status");
    expect(html).toContain("(github.com/alexgorbatchev/agent-status)");
    expect(html).toContain("1 active");
    expect(html).toContain('data-testid="ProjectGroup--grid"');
  });

  it("groups cards into busy, idle, and finished sections in order with double vertical gap and no subtitles", () => {
    const busyAgent1: IAgentState = {
      ...fixture_agentStateRunning,
      id: "busy-1",
      activity: { ...fixture_agentStateRunning.activity, title: "Busy Title 1" },
    };
    const busyAgent2: IAgentState = {
      ...fixture_agentStateRunning,
      id: "busy-2",
      activity: { ...fixture_agentStateRunning.activity, title: "Busy Title 2" },
    };
    const idleAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "idle-1",
      activity: { ...fixture_agentStateRunning.activity, activity: "IDLE", title: "Idle Title 1" },
    };
    const finishedAgent: IAgentState = {
      ...fixture_agentStateExited,
      id: "finished-1",
      activity: { ...fixture_agentStateExited.activity, title: "Finished Title 1" },
    };

    const html = renderToString(
      <AgentGroupSection
        icon={<FolderGit2 size={16} />}
        title="Multi Status Project"
        subtitle="(multi-status)"
        agents={[idleAgent, finishedAgent, busyAgent1, busyAgent2]}
        onSelectAgent={() => {}}
      />,
    );

    // Double vertical gap on container
    expect(html).toContain('class="flex flex-col gap-8"');

    // Category grids present in order: busy, then idle, then finished
    expect(html).toContain('data-testid="AgentGroupSection--grid-busy"');
    expect(html).toContain('data-testid="AgentGroupSection--grid-idle"');
    expect(html).toContain('data-testid="AgentGroupSection--grid-finished"');

    const busyIndex = html.indexOf('data-testid="AgentGroupSection--grid-busy"');
    const idleIndex = html.indexOf('data-testid="AgentGroupSection--grid-idle"');
    const finishedIndex = html.indexOf('data-testid="AgentGroupSection--grid-finished"');

    expect(busyIndex).toBeLessThan(idleIndex);
    expect(idleIndex).toBeLessThan(finishedIndex);

    // No category subtitles rendered between grids (only the category grid divs exist as children)
    expect(html).not.toContain('data-testid="AgentGroupSection--category-title"');
    expect(html).not.toContain('data-testid="AgentGroupSection--subtitle-');

    // Busy cards are inside busy grid, idle card is inside idle grid
    const busySectionHtml = html.slice(busyIndex, idleIndex);
    expect(busySectionHtml).toContain("Busy Title 1");
    expect(busySectionHtml).toContain("Busy Title 2");
    expect(busySectionHtml).not.toContain("Idle Title 1");

    const idleSectionHtml = html.slice(idleIndex, finishedIndex);
    expect(idleSectionHtml).toContain("Idle Title 1");
    expect(idleSectionHtml).not.toContain("Busy Title 1");
  });

  it("starts idle cards in their own grid row when there are 5 busy session cards", () => {
    const busyAgents: IAgentState[] = Array.from({ length: 5 }, (_, i) => ({
      ...fixture_agentStateRunning,
      id: `busy-${i + 1}`,
      activity: { ...fixture_agentStateRunning.activity, title: `Five Busy ${i + 1}` },
    }));
    const idleAgent: IAgentState = {
      ...fixture_agentStateRunning,
      id: "idle-1",
      activity: { ...fixture_agentStateRunning.activity, activity: "IDLE", title: "Five Idle 1" },
    };

    const html = renderToString(
      <AgentGroupSection
        icon={<FolderGit2 size={16} />}
        title="Five Busy Project"
        subtitle="(five-busy)"
        agents={[...busyAgents, idleAgent]}
        onSelectAgent={() => {}}
      />,
    );

    // Both busy and idle grids exist separately
    expect(html).toContain('data-testid="AgentGroupSection--grid-busy"');
    expect(html).toContain('data-testid="AgentGroupSection--grid-idle"');
    expect(html).not.toContain('data-testid="AgentGroupSection--grid-finished"');

    const busyIndex = html.indexOf('data-testid="AgentGroupSection--grid-busy"');
    const idleIndex = html.indexOf('data-testid="AgentGroupSection--grid-idle"');
    const busySectionHtml = html.slice(busyIndex, idleIndex);
    const idleSectionHtml = html.slice(idleIndex);

    // All 5 busy cards are in the busy grid
    for (let i = 1; i <= 5; i++) {
      expect(busySectionHtml).toContain(`Five Busy ${i}`);
    }
    expect(busySectionHtml).not.toContain("Five Idle 1");

    // Idle card is in its own idle grid, not continuing on row 2 of busy
    expect(idleSectionHtml).toContain("Five Idle 1");
  });
});
