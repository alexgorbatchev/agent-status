import React from "react";
import type { IProjectGroupProps } from "./types";
import { AgentGroupSection } from "./AgentGroupSection";
import { FolderGit2 } from "lucide-react";

export function ProjectGroup({
  projectName,
  projectId,
  agents,
  onSelectAgent,
  onToggleHide,
}: IProjectGroupProps): React.JSX.Element {
  return (
    <div data-testid="ProjectGroup">
      <AgentGroupSection
        icon={<FolderGit2 size={16} />}
        iconColorClass="text-blue-400"
        title={projectName}
        subtitle={`(${projectId})`}
        agents={agents}
        onSelectAgent={onSelectAgent}
        onToggleHide={onToggleHide}
      />
    </div>
  );
}
