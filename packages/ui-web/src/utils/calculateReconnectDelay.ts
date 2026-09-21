export const INITIAL_RECONNECT_DELAY_MS = 1000;
export const MAX_RECONNECT_DELAY_MS = 30000;
export const MAX_RECONNECT_JITTER_MS = 1000;

export type RandomNumberGenerator = () => number;

export function calculateReconnectDelay(attempt: number, randomFn: RandomNumberGenerator = Math.random): number {
  const safeAttempt = Math.max(0, attempt);
  const exponentialDelay = Math.min(INITIAL_RECONNECT_DELAY_MS * Math.pow(2, safeAttempt), MAX_RECONNECT_DELAY_MS);
  const jitter = Math.floor(randomFn() * MAX_RECONNECT_JITTER_MS);
  return exponentialDelay + jitter;
}
