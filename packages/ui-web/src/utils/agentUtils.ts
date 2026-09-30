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

export const ENVIRONMENT_STALE_THRESHOLD_MS = 30_000;
export const SESSION_SILENCE_THRESHOLD_MS = 120_000;
export const TOOL_STALL_THRESHOLD_MS = 180_000;

const FIFTEEN_MINUTES_MS = 15 * 60 * 1000;
const TWO_HOURS_MS = 2 * 60 * 60 * 1000;

export function getIdleFreshness(updatedAt: number, now: number = Date.now()): IdleFreshness {
  if (!updatedAt || updatedAt <= 0 || Number.isNaN(updatedAt)) {
    return "stale";
  }
  const diffMs = Math.max(0, now - updatedAt);
  if (diffMs < FIFTEEN_MINUTES_MS) {
    return "fresh";
  }
  if (diffMs <= TWO_HOURS_MS) {
    return "standard";
  }
  return "stale";
}

export function isAgentStaleIdle(
  activity: IAgentActivity,
  now: number = Date.now(),
  acknowledgedCompletion: boolean = false,
): boolean {
  if (activity.lifecycle !== "RUNNING" || activity.activity !== "IDLE") {
    return false;
  }
  const effectiveHasUnseenCompletion = Boolean(activity.hasUnseenCompletion) && !acknowledgedCompletion;
  if (effectiveHasUnseenCompletion) {
    return false;
  }
  return getIdleFreshness(activity.updatedAt, now) === "stale";
}

export function getAgentIdleFreshness(activity: IAgentActivity, now: number = Date.now()): IdleFreshness {
  if (activity.lifecycle !== "RUNNING" || activity.activity !== "IDLE") {
    return "fresh";
  }
  const effectiveHasUnseenCompletion = Boolean(activity.hasUnseenCompletion);
  if (effectiveHasUnseenCompletion) {
    return "fresh";
  }
  return getIdleFreshness(activity.updatedAt, now);
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
  if (durationMs < TOOL_STALL_THRESHOLD_MS) {
    return "warning";
  }
  return "stalled";
}

export function formatDuration(durationMs: number): string {
  if (durationMs < 60000) {
    const seconds = Math.floor(durationMs / 1000);
    return `${seconds}s`;
  }
  const hours = Math.floor(durationMs / 3600000);
  const mins = Math.floor((durationMs % 3600000) / 60000);
  const secs = Math.floor((durationMs % 60000) / 1000);

  if (hours > 0) {
    if (mins > 0) {
      return `${hours}h ${mins}m`;
    }
    return `${hours}h`;
  }
  if (secs > 0) {
    return `${mins}m ${secs}s`;
  }
  return `${mins}m`;
}

export function isSessionStalled(
  updatedAt: number,
  now: number = Date.now(),
  thresholdMs: number = SESSION_SILENCE_THRESHOLD_MS,
): boolean {
  if (!updatedAt || updatedAt <= 0 || Number.isNaN(updatedAt)) {
    return false;
  }
  return now - updatedAt >= thresholdMs;
}

export function isEnvironmentStale(
  lastHeartbeatAt: number,
  now: number = Date.now(),
  thresholdMs: number = ENVIRONMENT_STALE_THRESHOLD_MS,
): boolean {
  if (!lastHeartbeatAt || lastHeartbeatAt <= 0 || Number.isNaN(lastHeartbeatAt)) {
    return true;
  }
  return now - lastHeartbeatAt > thresholdMs;
}

export function isAgentRequiringAttention(agent: IAgentState, now: number = Date.now()): boolean {
  if (
    agent.activity.lifecycle !== "RUNNING" ||
    agent.environment.isStale ||
    isEnvironmentStale(agent.environment.lastHeartbeatAt, now)
  ) {
    return false;
  }
  if (agent.activity.activity === "WAITING_QUESTION" || agent.activity.activity === "ERROR") {
    return true;
  }
  if (agent.activity.activity === "RATE_LIMITED") {
    const resetAt = agent.activity.rateLimitResetAt;
    if (resetAt !== undefined && resetAt !== null) {
      if (resetAt > now) {
        return false;
      }
    } else {
      const retryAfter = agent.activity.retryAfterSeconds;
      if (retryAfter !== undefined && retryAfter !== null) {
        const remainingSec = retryAfter - Math.floor((now - (agent.activity.updatedAt || 0)) / 1000);
        if (remainingSec > 0) {
          return false;
        }
      }
    }
    return isSessionStalled(agent.activity.updatedAt, now);
  }
  const durationMs = getToolExecutionDuration(agent.activity, now);
  if (getToolStallTier(durationMs) === "stalled") {
    return true;
  }
  return agent.activity.activity === "BUSY" && isSessionStalled(agent.activity.updatedAt, now);
}

export function formatRateLimitCallout(activity: IAgentActivity, now: number = Date.now()): string {
  const baseMsg = activity.lastError || "Rate limit reached — backoff/retry in progress";
  const resetAt = activity.rateLimitResetAt;
  const retryAfter = activity.retryAfterSeconds;

  let waitStr = "";
  if (resetAt !== undefined && resetAt !== null) {
    if (resetAt > now) {
      const remainingSec = Math.max(1, Math.ceil((resetAt - now) / 1000));
      waitStr = remainingSec > 120 ? `retry in ${formatDuration(remainingSec * 1000)}` : `retry in ${remainingSec}s`;
    }
  } else if (retryAfter !== undefined && retryAfter !== null && retryAfter > 0) {
    const elapsedSec = Math.floor((now - (activity.updatedAt || 0)) / 1000);
    const remainingSec = Math.max(0, retryAfter - elapsedSec);
    if (remainingSec > 0) {
      waitStr = remainingSec > 120 ? `retry in ${formatDuration(remainingSec * 1000)}` : `retry in ${remainingSec}s`;
    }
  }

  if (waitStr && !baseMsg.includes(waitStr)) {
    if (activity.lastError) {
      return `${baseMsg} (${waitStr})`;
    }
    return `Rate limit reached — ${waitStr}`;
  }

  return baseMsg;
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

    // Pattern 1: Bracketed project prefix e.g. "[agent-status]" followed by separator or whitespace
    const bracketedRegex = new RegExp(`^\\s*\\[\\s*${escaped}\\s*\\](?:\\s*[:·|]\\s+|\\s+[-–—/]+\\s+|\\s+)`);
    const bracketedMatch = title.match(bracketedRegex);
    if (bracketedMatch) {
      const remainder = title.slice(bracketedMatch[0].length).trim();
      const substantive = remainder.replace(/^[:·|/–—\-\s]+/, "").trim();
      if (substantive.length > 0 && !remainder.startsWith("]")) {
        return remainder;
      }
      return title;
    }

    // Pattern 2: Non-bracketed project prefix followed by separator requiring whitespace
    const nonBracketedRegex = new RegExp(`^\\s*${escaped}(?:\\s*[:·|]\\s+|\\s+[-–—/]+\\s+)`);
    const nonBracketedMatch = title.match(nonBracketedRegex);
    if (nonBracketedMatch) {
      const remainder = title.slice(nonBracketedMatch[0].length).trim();
      const substantive = remainder.replace(/^[:·|/–—\-\s]+/, "").trim();
      if (substantive.length > 0 && !remainder.startsWith("]")) {
        return remainder;
      }
      return title;
    }
  }

  return title;
}
