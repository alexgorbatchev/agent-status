import type { IAgentState, IAgentActivity } from "../types";

export function calculateTotalSpend(agents: IAgentState[]): number {
  return agents.reduce((acc, a) => acc + (a.activity.cost?.total || 0), 0);
}

export function formatCurrency(amount: number, precision: number = 2): string {
  return `$${amount.toFixed(precision)}`;
}

export function filterActiveAgents(agents: IAgentState[]): IAgentState[] {
  return agents.filter(
    (a) =>
      a.activity.lifecycle === "RUNNING" && (a.activity.activity === "BUSY" || a.activity.activity === "RATE_LIMITED"),
  );
}

export function filterRunningAgents(agents: IAgentState[]): IAgentState[] {
  return agents.filter((a) => a.activity.lifecycle === "RUNNING");
}

export type ToolStallTier = "healthy" | "notice" | "warning" | "stalled";

export type IdleFreshness = "fresh" | "standard" | "stale";

const FIFTEEN_MINUTES_MS = 15 * 60 * 1000;
const TWO_HOURS_MS = 2 * 60 * 60 * 1000;

export function getIdleFreshness(updatedAt: number, now: number = Date.now()): IdleFreshness {
  if (!updatedAt || updatedAt <= 0 || Number.isNaN(updatedAt)) {
    return "stale";
  }
  const diffMs = now - updatedAt;
  if (diffMs < FIFTEEN_MINUTES_MS) {
    return "fresh";
  }
  if (diffMs < TWO_HOURS_MS) {
    return "standard";
  }
  return "stale";
}

export function getToolExecutionDuration(activity: IAgentActivity, now: number): number | null {
  if (
    activity.lifecycle !== "RUNNING" ||
    activity.activity !== "BUSY" ||
    !activity.activeTool ||
    !activity.activeToolStartedAt ||
    activity.activeToolStartedAt <= 0
  ) {
    return null;
  }
  const diffMs = now - activity.activeToolStartedAt;
  return diffMs >= 0 ? diffMs : 0;
}

export function getToolStallTier(durationMs: number | null): ToolStallTier {
  if (durationMs === null || durationMs < 60000) {
    return "healthy";
  }
  if (durationMs < 120000) {
    return "notice";
  }
  if (durationMs < 180000) {
    return "warning";
  }
  return "stalled";
}

export function formatDuration(durationMs: number): string {
  if (durationMs < 60000) {
    const seconds = Math.floor(durationMs / 1000);
    return `${seconds}s`;
  }
  const mins = Math.floor(durationMs / 60000);
  const secs = Math.floor((durationMs % 60000) / 1000);
  return `${mins}m ${secs}s`;
}

export function isSessionStalled(updatedAt: number, now: number = Date.now(), thresholdMs: number = 120_000): boolean {
  if (!updatedAt || updatedAt <= 0 || Number.isNaN(updatedAt)) {
    return false;
  }
  return now - updatedAt >= thresholdMs;
}

export function isAgentRequiringAttention(agent: IAgentState, now: number = Date.now()): boolean {
  if (agent.activity.lifecycle !== "RUNNING" || agent.environment.isStale) {
    return false;
  }
  if (agent.activity.activity === "WAITING_QUESTION" || agent.activity.activity === "ERROR") {
    return true;
  }
  if (agent.activity.activity === "RATE_LIMITED") {
    return isSessionStalled(agent.activity.updatedAt, now);
  }
  const durationMs = getToolExecutionDuration(agent.activity, now);
  if (getToolStallTier(durationMs) === "stalled") {
    return true;
  }
  return agent.activity.activity === "BUSY" && isSessionStalled(agent.activity.updatedAt, now);
}

export function formatRelativeTime(timestamp: number, now: number = Date.now()): string {
  if (!timestamp || timestamp <= 0 || Number.isNaN(timestamp)) {
    return "—";
  }

  const diffMs = now - timestamp;
  if (diffMs < 5000) {
    return "just now";
  }

  if (diffMs < 60000) {
    const seconds = Math.floor(diffMs / 1000);
    return `${seconds}s ago`;
  }

  if (diffMs < 3600000) {
    const minutes = Math.floor(diffMs / 60000);
    return `${minutes}m ago`;
  }

  if (diffMs < 86400000) {
    const hours = Math.floor(diffMs / 3600000);
    return `${hours}h ago`;
  }

  const days = Math.floor(diffMs / 86400000);
  return `${days}d ago`;
}

export function formatExactTime(timestamp: number): string {
  if (!timestamp || timestamp <= 0 || Number.isNaN(timestamp)) {
    return "Never";
  }
  return new Date(timestamp).toLocaleString();
}

export function getAgentDebugCommand(agent: IAgentState): string {
  return `just debug session replay --harness ${agent.harness} ${agent.sessionId}`;
}

function extractProjectCandidates(projectName?: string): string[] {
  if (!projectName) {
    return [];
  }
  const trimmed = projectName.trim();
  if (!trimmed) {
    return [];
  }

  const candidates: string[] = [trimmed];

  // Extract repo basename: strip trailing slashes, then take last segment after / or \
  const normalized = trimmed.replace(/[/\\]+$/, "");
  const lastSlashIndex = Math.max(normalized.lastIndexOf("/"), normalized.lastIndexOf("\\"));
  let basename = lastSlashIndex !== -1 ? normalized.slice(lastSlashIndex + 1) : normalized;
  if (basename.toLowerCase().endsWith(".git")) {
    basename = basename.slice(0, -4);
  }
  if (basename && !candidates.includes(basename)) {
    candidates.push(basename);
  }

  return candidates.sort((a, b) => b.length - a.length);
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function trimProjectTitlePrefix(title: string, projectName?: string): string {
  if (!title) {
    return "";
  }
  const candidates = extractProjectCandidates(projectName);
  if (candidates.length === 0) {
    return title;
  }

  for (const candidate of candidates) {
    const escaped = escapeRegExp(candidate);

    // Pattern 1: Bracketed project prefix e.g. "[agent-status]" or "[agent-status]:"
    const bracketedRegex = new RegExp(`^\\s*\\[\\s*${escaped}\\s*\\]\\s*(?:[:·|–—]|-+)?\\s*`, "i");
    const bracketedMatch = title.match(bracketedRegex);
    if (bracketedMatch) {
      const remainder = title.slice(bracketedMatch[0].length).trim();
      const substantive = remainder.replace(/^[:·|–—\-\s]+/, "").trim();
      if (substantive.length > 0) {
        return remainder;
      }
      return title;
    }

    // Pattern 2: Non-bracketed project prefix followed by separator
    // Separators: ':', '·', '|', ']', or '-' (with whitespace boundary to avoid matching kebab-case words)
    const nonBracketedRegex = new RegExp(`^\\s*${escaped}(?:\\s*[:·|–—\\]]\\s*|\\s+[-–—]+\\s*|\\s*[-–—]+\\s+)`, "i");
    const nonBracketedMatch = title.match(nonBracketedRegex);
    if (nonBracketedMatch) {
      const remainder = title.slice(nonBracketedMatch[0].length).trim();
      const substantive = remainder.replace(/^[:·|–—\-\s]+/, "").trim();
      if (substantive.length > 0) {
        return remainder;
      }
      return title;
    }
  }

  return title;
}
