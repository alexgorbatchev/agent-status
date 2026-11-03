import { describe, it, expect, afterEach } from "bun:test";
import { DEFAULT_API_BASE_URL, DEFAULT_DEV_TOKEN, resolveApiBaseUrl } from "../config";
import type { IWindowWithAgentStatus } from "../../types";

describe("config", () => {
  const originalWindow = globalThis.window;

  afterEach(() => {
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

  it("uses the page origin for HTTPS so API requests stay on the routed host", () => {
    Object.defineProperty(globalThis, "window", {
      value: { location: new URL("https://dashboard.example.test/") },
      configurable: true,
      writable: true,
    });

    expect(resolveApiBaseUrl()).toBe("https://dashboard.example.test");
  });

  it("keeps explicit API overrides ahead of the HTTPS page origin", () => {
    Object.defineProperty(globalThis, "window", {
      value: {
        location: new URL("https://dashboard.example.test/"),
        __AGENT_STATUS_API_URL__: "https://api.example.test",
      },
      configurable: true,
      writable: true,
    });

    expect(resolveApiBaseUrl()).toBe("https://api.example.test");
    expect(resolveApiBaseUrl("https://override.example.test")).toBe("https://override.example.test");
  });

  it("keeps the localhost API default for plain HTTP development", () => {
    Object.defineProperty(globalThis, "window", {
      value: { location: new URL("http://localhost:3000/") },
      configurable: true,
      writable: true,
    });

    expect(resolveApiBaseUrl()).toBe("http://localhost:4502");
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
