import { afterEach, describe, expect, it } from "bun:test";
import { dismissCompletion, focusAgent } from "../api";

type FetchUrl = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

describe("api service", () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  describe("focusAgent", () => {
    it("returns ok true with pane details on 200 response", async () => {
      globalThis.fetch = (async (url: FetchUrl, init?: FetchInit) => {
        expect(String(url)).toContain("/api/v1/agents/agent-123/focus");
        expect(init?.method).toBe("POST");
        expect(init?.headers).toEqual({
          Authorization: "Bearer dev-secret-token",
        });

        return new Response(
          JSON.stringify({
            ok: true,
            paneId: "w1:p1",
            workspaceId: "w1",
            tabId: "w1:t1",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }) as unknown as typeof fetch;

      const result = await focusAgent("agent-123");
      expect(result.ok).toBe(true);
      expect(result.paneId).toBe("w1:p1");
      expect(result.workspaceId).toBe("w1");
      expect(result.tabId).toBe("w1:t1");
      expect(result.error).toBeUndefined();
    });

    it("returns error on non-ok HTTP response", async () => {
      globalThis.fetch = (async () => {
        return new Response(JSON.stringify({ error: "agent is exited" }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        });
      }) as unknown as typeof fetch;

      const result = await focusAgent("agent-exited");
      expect(result.ok).toBe(false);
      expect(result.error).toBe("agent is exited");
    });

    it("handles network failure gracefully", async () => {
      globalThis.fetch = (() => Promise.reject(new Error("Connection refused"))) as unknown as typeof fetch;

      const result = await focusAgent("agent-err");
      expect(result.ok).toBe(false);
      expect(result.error).toBe("Connection refused");
    });
  });

  describe("dismissCompletion", () => {
    it("sends PATCH with dismissCompletion: true on 200 response", async () => {
      globalThis.fetch = (async (url: FetchUrl, init?: FetchInit) => {
        expect(String(url)).toContain("/api/v1/agents/agent-123");
        expect(init?.method).toBe("PATCH");
        expect(init?.headers).toEqual({
          "Content-Type": "application/json",
          Authorization: "Bearer dev-secret-token",
        });
        expect(init?.body).toBe(JSON.stringify({ dismissCompletion: true }));

        return new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }) as unknown as typeof fetch;

      const result = await dismissCompletion("agent-123");
      expect(result.ok).toBe(true);
      expect(result.error).toBeUndefined();
    });

    it("returns error on non-ok HTTP response", async () => {
      globalThis.fetch = (async () => {
        return new Response(JSON.stringify({ error: "agent not found" }), {
          status: 404,
          headers: { "Content-Type": "application/json" },
        });
      }) as unknown as typeof fetch;

      const result = await dismissCompletion("non-existent");
      expect(result.ok).toBe(false);
      expect(result.error).toBe("agent not found");
    });

    it("returns status text error when response body is not json", async () => {
      globalThis.fetch = (async () => {
        return new Response("Internal Server Error", {
          status: 500,
          statusText: "Internal Server Error",
        });
      }) as unknown as typeof fetch;

      const result = await dismissCompletion("agent-err");
      expect(result.ok).toBe(false);
      expect(result.error).toBe("HTTP 500: Internal Server Error");
    });

    it("handles network failure gracefully", async () => {
      globalThis.fetch = (() => Promise.reject(new Error("Network offline"))) as unknown as typeof fetch;

      const result = await dismissCompletion("agent-err");
      expect(result.ok).toBe(false);
      expect(result.error).toBe("Network offline");
    });
  });
});
