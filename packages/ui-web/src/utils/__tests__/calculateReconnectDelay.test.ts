import { describe, it, expect } from "bun:test";
import {
  calculateReconnectDelay,
  INITIAL_RECONNECT_DELAY_MS,
  MAX_RECONNECT_DELAY_MS,
  MAX_RECONNECT_JITTER_MS,
} from "../calculateReconnectDelay";

interface IDelayTestCase {
  attempt: number;
  expected: number;
}

describe("calculateReconnectDelay", () => {
  it("calculates exponential backoff capped at 30000ms with zero jitter", () => {
    const zeroJitter = () => 0;
    const testCases: IDelayTestCase[] = [
      { attempt: 0, expected: 1000 },
      { attempt: 1, expected: 2000 },
      { attempt: 2, expected: 4000 },
      { attempt: 3, expected: 8000 },
      { attempt: 4, expected: 16000 },
      { attempt: 5, expected: 30000 },
      { attempt: 6, expected: 30000 },
      { attempt: 10, expected: 30000 },
    ];

    for (const testCase of testCases) {
      expect(calculateReconnectDelay(testCase.attempt, zeroJitter)).toBe(testCase.expected);
    }
  });

  it("adds random jitter within [0, 1000)ms range", () => {
    expect(calculateReconnectDelay(0, () => 0)).toBe(INITIAL_RECONNECT_DELAY_MS);
    expect(calculateReconnectDelay(0, () => 0.5)).toBe(INITIAL_RECONNECT_DELAY_MS + 500);
    expect(calculateReconnectDelay(0, () => 0.999)).toBe(INITIAL_RECONNECT_DELAY_MS + 999);

    expect(calculateReconnectDelay(5, () => 0.25)).toBe(MAX_RECONNECT_DELAY_MS + 250);
    expect(calculateReconnectDelay(5, () => 0.75)).toBe(MAX_RECONNECT_DELAY_MS + 750);
  });

  it("clamps negative attempt counts to 0", () => {
    expect(calculateReconnectDelay(-1, () => 0)).toBe(INITIAL_RECONNECT_DELAY_MS);
    expect(calculateReconnectDelay(-10, () => 0)).toBe(INITIAL_RECONNECT_DELAY_MS);
  });

  it("uses Math.random by default producing valid delay range", () => {
    for (let attempt = 0; attempt <= 6; attempt++) {
      const delay = calculateReconnectDelay(attempt);
      const base = Math.min(INITIAL_RECONNECT_DELAY_MS * Math.pow(2, attempt), MAX_RECONNECT_DELAY_MS);
      expect(delay).toBeGreaterThanOrEqual(base);
      expect(delay).toBeLessThan(base + MAX_RECONNECT_JITTER_MS);
    }
  });
});
