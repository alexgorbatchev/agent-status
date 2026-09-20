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
    expect(html).not.toContain("active");
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
    expect(html).not.toContain("active");
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

  it("delegates from ProjectGroup correctly", () => {
    const html = renderToString(
      <ProjectGroup
        projectName="agent-status"
        projectId="github.com/alexgorbatchev/agent-status"
        agents={[fixture_agentStateRunning, fixture_agentStateExited]}
        onSelectAgent={() => {}}
      />,
    );

    expect(html).toContain('data-testid="AgentGroupSection"');
    expect(html).toContain('data-testid="AgentGroupSection--title"');
    expect(html).toContain("agent-status");
    expect(html).toContain("(github.com/alexgorbatchev/agent-status)");
    expect(html).toContain("1 active");
    expect(html).toContain('data-testid="AgentGroupSection--grid"');
  });
});
