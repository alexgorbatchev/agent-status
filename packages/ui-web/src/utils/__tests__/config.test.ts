import { describe, it, expect, afterEach } from "bun:test";
import { DEFAULT_API_BASE_URL, DEFAULT_DEV_TOKEN, resolveApiBaseUrl } from "../config";
import type { IWindowWithAgentStatus } from "../../types";

describe("config", () => {
  const originalWindow = globalThis.window;
  const originalApiUrl = process.env.AGENT_PUBLIC_API_URL;

  afterEach(() => {
    process.env.AGENT_PUBLIC_API_URL = originalApiUrl;
    Object.defineProperty(globalThis, "window", {
      value: originalWindow,
      configurable: true,
      writable: true,
    });
  });

  it("exports correct defaults", () => {
    expect(DEFAULT_API_BASE_URL).toBe("http://localhost:4502");
    expect(DEFAULT_DEV_TOKEN).toBe("dev-secret-token");
  });

  it("returns overrideUrl when provided", () => {
    const resolved = resolveApiBaseUrl("http://custom-host:8080");
    expect(resolved).toBe("http://custom-host:8080");
  });

  it("reads the public API URL supplied by devhost", () => {
    Object.defineProperty(globalThis, "window", { value: undefined, configurable: true, writable: true });
    process.env.AGENT_PUBLIC_API_URL = "https://dashboard.example";
    expect(resolveApiBaseUrl()).toBe("https://dashboard.example");
  });

  it("keeps explicit overrides ahead of the public API URL", () => {
    process.env.AGENT_PUBLIC_API_URL = "https://dashboard.example";
    expect(resolveApiBaseUrl("https://override.example")).toBe("https://override.example");
  });

  it("uses the local API default when the public API URL is empty", () => {
    Object.defineProperty(globalThis, "window", { value: undefined, configurable: true, writable: true });
    process.env.AGENT_PUBLIC_API_URL = "";
    expect(resolveApiBaseUrl()).toBe("http://localhost:4502");
  });

  it("reads window.__AGENT_STATUS_API_URL__ when no override is provided", () => {
    const mockWindow: IWindowWithAgentStatus = {
      __AGENT_STATUS_API_URL__: "http://injected-host:9999",
    };
    Object.defineProperty(globalThis, "window", {
      value: mockWindow,
      configurable: true,
      writable: true,
    });

    const resolved = resolveApiBaseUrl();
    expect(resolved).toBe("http://injected-host:9999");
  });

  it("falls back to DEFAULT_API_BASE_URL when window property is absent", () => {
    const mockWindow: IWindowWithAgentStatus = {};
    Object.defineProperty(globalThis, "window", {
      value: mockWindow,
      configurable: true,
      writable: true,
    });

    const resolved = resolveApiBaseUrl();
    expect(resolved).toBe(DEFAULT_API_BASE_URL);
  });
});
