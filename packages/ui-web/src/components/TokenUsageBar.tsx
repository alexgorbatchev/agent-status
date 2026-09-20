import React from "react";
import type { ITokenUsageBarProps } from "./types";

export function TokenUsageBar({ usage }: ITokenUsageBarProps): React.JSX.Element {
  return (
    <div
      data-testid="TokenUsageBar"
      className="mt-1.5 pt-1.5 border-t border-slate-800/60 flex items-center gap-3 text-[10px] text-slate-500 font-mono"
    >
      <span data-testid="TokenUsageBar--input">In: {usage.input.toLocaleString()}</span>
      <span data-testid="TokenUsageBar--output">Out: {usage.output.toLocaleString()}</span>
      {usage.cacheRead > 0 && <span data-testid="TokenUsageBar--cache">Cache: {usage.cacheRead.toLocaleString()}</span>}
      <span data-testid="TokenUsageBar--total">Total: {usage.total.toLocaleString()}</span>
    </div>
  );
}
