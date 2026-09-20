import type { Page, Route } from "@playwright/test";
import type { ServerEvent } from "../../src/types";

export interface IMockSSEServer {
  sendEvent: (event: ServerEvent) => Promise<void>;
  close: () => Promise<void>;
}

/**
 * Sets up an intercepted SSE route on `http://localhost:4502/api/v1/events*`
 * that allows tests to push mock events (`snapshot`, `agent_upsert`, etc.) directly to the page.
 */
export async function setupMockSSERoute(page: Page, initialEvents: ServerEvent[] = []): Promise<IMockSSEServer> {
  let activeRoute: Route | null = null;
  let isClosed = false;

  await page.route("**/api/v1/events*", async (route) => {
    activeRoute = route;

    // Build initial SSE body payload
    let initialBody = "";
    for (const event of initialEvents) {
      initialBody += `data: ${JSON.stringify(event)}\n\n`;
    }

    await route.fulfill({
      status: 200,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Credentials": "true",
      },
      body: initialBody,
    });
  });

  return {
    async sendEvent(event: ServerEvent) {
      if (isClosed || !activeRoute) return;
      // Evaluate custom EventSource message dispatch directly into the browser context
      await page.evaluate((eventData) => {
        // Dispatch synthetic MessageEvent on active EventSource if accessible or custom window hook
        window.dispatchEvent(new CustomEvent("mock-sse-event", { detail: eventData }));
      }, event);
    },
    async close() {
      isClosed = true;
      if (activeRoute) {
        try {
          await activeRoute.abort();
        } catch {
          // ignore already closed
        }
      }
    },
  };
}
