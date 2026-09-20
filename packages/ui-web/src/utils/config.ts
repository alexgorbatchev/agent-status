import type { IWindowWithAgentStatus } from "../types";

export const DEFAULT_API_BASE_URL = "http://localhost:4502";
export const DEFAULT_DEV_TOKEN = "dev-secret-token";

export function resolveApiBaseUrl(overrideUrl?: string): string {
  if (overrideUrl) {
    return overrideUrl;
  }
  if (typeof window !== "undefined") {
    const windowWithStatus = window as unknown as IWindowWithAgentStatus;
    if (windowWithStatus.__AGENT_STATUS_API_URL__) {
      return windowWithStatus.__AGENT_STATUS_API_URL__;
    }
  }
  return DEFAULT_API_BASE_URL;
}
