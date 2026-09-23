import { describe, expect, it } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import { TokenUsageBar } from "../components/TokenUsageBar";
import type { ITokenUsage } from "../types";

describe("TokenUsageBar", () => {
  it("renders categorized token buckets including cache write, reasoning, and context fullness", () => {
    const usage: ITokenUsage = {
      input: 5000,
      output: 1000,
      cacheRead: 40000,
      cacheWrite: 4000,
      reasoning: 500,
      total: 50000,
    };

    const html = renderToString(<TokenUsageBar usage={usage} contextWindow={200000} contextFullness={0.25} />);

    expect(html).toContain('data-testid="TokenUsageBar--input"');
    expect(html).toContain("5,000");
    expect(html).toContain('data-testid="TokenUsageBar--output"');
    expect(html).toContain("1,000");
    expect(html).toContain('data-testid="TokenUsageBar--cache"');
    expect(html).toContain("40,000");
    expect(html).toContain('data-testid="TokenUsageBar--create"');
    expect(html).toContain("4,000");
    expect(html).toContain('data-testid="TokenUsageBar--reasoning"');
    expect(html).toContain("500");
    expect(html).toContain('data-testid="TokenUsageBar--total"');
    expect(html).toContain("50,000");
    expect(html).toContain('data-testid="TokenUsageBar--fullness"');
    expect(html).toContain("Context: 25.0%");
  });

  it("omits cache write, reasoning, and fullness when zero or not provided", () => {
    const usage: ITokenUsage = {
      input: 500,
      output: 100,
      cacheRead: 0,
      cacheWrite: 0,
      reasoning: 0,
      total: 600,
    };

    const html = renderToString(<TokenUsageBar usage={usage} />);

    expect(html).toContain('data-testid="TokenUsageBar--input"');
    expect(html).not.toContain('data-testid="TokenUsageBar--cache"');
    expect(html).not.toContain('data-testid="TokenUsageBar--create"');
    expect(html).not.toContain('data-testid="TokenUsageBar--reasoning"');
    expect(html).not.toContain('data-testid="TokenUsageBar--fullness"');
  });
});
