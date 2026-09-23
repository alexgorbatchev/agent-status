import React from "react";
import type { ITokenUsageBarProps } from "./types";

export function TokenUsageBar({ usage, contextWindow, contextFullness }: ITokenUsageBarProps): React.JSX.Element {
  const fullnessPercent =
    typeof contextFullness === "number"
      ? (contextFullness * 100).toFixed(1)
      : typeof contextWindow === "number" && contextWindow > 0
        ? ((usage.total / contextWindow) * 100).toFixed(1)
        : null;

  return (
    <div
      data-testid="TokenUsageBar"
      className="mt-1.5 pt-1.5 border-t border-slate-800/60 flex items-center gap-3 text-[10px] text-slate-500 font-mono"
    >
      <span data-testid="TokenUsageBar--input">In: {usage.input.toLocaleString()}</span>
      <span data-testid="TokenUsageBar--output">Out: {usage.output.toLocaleString()}</span>
      {usage.cacheRead > 0 && <span data-testid="TokenUsageBar--cache">Cache: {usage.cacheRead.toLocaleString()}</span>}
      {usage.cacheWrite > 0 && (
        <span data-testid="TokenUsageBar--create">Create: {usage.cacheWrite.toLocaleString()}</span>
      )}
      {usage.reasoning > 0 && (
        <span data-testid="TokenUsageBar--reasoning">Reasoning: {usage.reasoning.toLocaleString()}</span>
      )}
      <span data-testid="TokenUsageBar--total">Total: {usage.total.toLocaleString()}</span>
      {fullnessPercent !== null && <span data-testid="TokenUsageBar--fullness">{`Context: ${fullnessPercent}%`}</span>}
    </div>
  );
}
