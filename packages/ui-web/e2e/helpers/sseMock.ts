import type { Page } from "@playwright/test";
import type { ServerEvent } from "../../src/types";

type EventHandler = (ev: Event) => void;
type MessageEventHandler = (ev: MessageEvent) => void;
type WindowWithEventSource = { EventSource: typeof EventSource };

export interface IMockSSEServer {
  sendEvent: (event: ServerEvent) => Promise<void>;
  simulateError: () => Promise<void>;
  close: () => Promise<void>;
}

/**
 * Sets up a mock SSE provider on the page that delivers initialEvents upon connection
 * and allows tests to dynamically push mock events (`snapshot`, `agent_upsert`, `agent_removed`, etc.).
 */
export async function setupMockSSERoute(page: Page, initialEvents: ServerEvent[] = []): Promise<IMockSSEServer> {
  let isClosed = false;

  await page.addInitScript((events) => {
    class MockEventSource extends EventTarget {
      url: string;
      readyState: number = 0; // CONNECTING
      onopen: EventHandler | null = null;
      onmessage: MessageEventHandler | null = null;
      onerror: EventHandler | null = null;
      private isClosed = false;

      constructor(url: string) {
        super();
        this.url = url;

        setTimeout(() => {
          if (this.isClosed) return;
          this.readyState = 1; // OPEN
          if (this.onopen) {
            this.onopen(new Event("open"));
          }
          if (this.url.includes("/events")) {
            for (const ev of events) {
              if (this.isClosed) return;
              if (this.onmessage) {
                this.onmessage(new MessageEvent("message", { data: JSON.stringify(ev) }));
              }
            }
          }
        }, 10);

        window.addEventListener("mock-sse-event", (e: Event) => {
          if (this.isClosed) return;
          if (!this.url.includes("/events")) return;
          const detail = (e as CustomEvent).detail;
          if (this.onmessage) {
            this.onmessage(new MessageEvent("message", { data: JSON.stringify(detail) }));
          }
        });

        window.addEventListener("mock-sse-error", () => {
          if (this.isClosed) return;
          if (!this.url.includes("/events")) return;
          this.readyState = 2; // CLOSED
          if (this.onerror) {
            this.onerror(new Event("error"));
          }
        });
      }

      close() {
        this.isClosed = true;
        this.readyState = 2; // CLOSED
      }
    }

    (window as unknown as WindowWithEventSource).EventSource = MockEventSource as unknown as typeof EventSource;
  }, initialEvents);

  return {
    async sendEvent(event: ServerEvent) {
      if (isClosed) return;
      await page.evaluate((eventData) => {
        window.dispatchEvent(new CustomEvent("mock-sse-event", { detail: eventData }));
      }, event);
    },
    async simulateError() {
      await page.evaluate(() => {
        window.dispatchEvent(new CustomEvent("mock-sse-error"));
      });
    },
    async close() {
      isClosed = true;
      await page.evaluate(() => {
        window.dispatchEvent(new CustomEvent("mock-sse-error"));
      });
    },
  };
}
