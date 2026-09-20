import { describe, it, expect, beforeEach, afterEach } from "bun:test";
import assert from "node:assert";
import { act, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { jsx } from "react/jsx-runtime";
import { useEventStream, type IEventStreamState } from "../useEventStream";
import type { ServerEvent } from "../../types";
import {
  fixture_eventStreamEnvironment1,
  fixture_eventStreamEnvironment2,
  fixture_eventStreamAgent1,
  fixture_eventStreamAgent1Updated,
  fixture_eventStreamAgent2,
} from "./fixtures";

type EventListenerFn = (event: unknown) => void;
type StateUpdateCallback = (state: IEventStreamState) => void;
type EventSourceVoidFn = () => void;

interface IEventSourceMessage {
  data: string;
}

type EventSourceMessageFn = (msg: IEventSourceMessage) => void;

interface ICustomDispatchEvent {
  type: string;
}

interface ITextNode {
  nodeType: number;
  nodeValue: string;
  ownerDocument: MockDocument;
}

interface ICommentNode {
  nodeType: number;
  nodeValue: string;
  ownerDocument: MockDocument;
}

interface IHookHarnessProps {
  apiBaseUrl?: string;
  token?: string;
  onUpdate: StateUpdateCallback;
}

interface IGlobalActConfig {
  IS_REACT_ACT_ENVIRONMENT: boolean;
}

(globalThis as unknown as IGlobalActConfig).IS_REACT_ACT_ENVIRONMENT = true;

class MockElement {
  nodeType = 1;
  nodeName = "DIV";
  tagName = "DIV";
  children: MockElement[] = [];
  childNodes: MockElement[] = [];
  style = {};
  ownerDocument: MockDocument | null = null;
  parentNode: MockElement | null = null;
  listeners: Record<string, EventListenerFn[]> = {};

  addEventListener(event: string, fn: EventListenerFn): void {
    const list = this.listeners[event] ?? [];
    list.push(fn);
    this.listeners[event] = list;
  }

  removeEventListener(event: string, fn: EventListenerFn): void {
    const list = this.listeners[event] ?? [];
    this.listeners[event] = list.filter((f) => f !== fn);
  }

  dispatchEvent(event: ICustomDispatchEvent): boolean {
    const fns = this.listeners[event.type] ?? [];
    for (const fn of fns) {
      fn(event);
    }
    return true;
  }

  appendChild(child: MockElement): MockElement {
    this.children.push(child);
    this.childNodes.push(child);
    child.parentNode = this;
    return child;
  }

  removeChild(child: MockElement): MockElement {
    this.children = this.children.filter((c) => c !== child);
    this.childNodes = this.childNodes.filter((c) => c !== child);
    child.parentNode = null;
    return child;
  }

  insertBefore(child: MockElement, _ref: MockElement): MockElement {
    return this.appendChild(child);
  }

  setAttribute(): void {}
  removeAttribute(): void {}
  getAttribute(): string | null {
    return null;
  }
}

class MockDocument {
  nodeType = 9;
  createElement(tag: string): MockElement {
    const el = new MockElement();
    el.nodeName = tag.toUpperCase();
    el.tagName = tag.toUpperCase();
    el.ownerDocument = this;
    return el;
  }

  createElementNS(_ns: string, tag: string): MockElement {
    const el = new MockElement();
    el.nodeName = tag.toUpperCase();
    el.tagName = tag.toUpperCase();
    el.ownerDocument = this;
    return el;
  }

  createTextNode(text: string): ITextNode {
    return {
      nodeType: 3,
      nodeValue: text,
      ownerDocument: this,
    };
  }

  createComment(data: string): ICommentNode {
    return {
      nodeType: 8,
      nodeValue: data,
      ownerDocument: this,
    };
  }

  addEventListener(): void {}
  removeEventListener(): void {}
  ownerDocument = null;
  defaultView = globalThis;
  body = new MockElement();
  head = new MockElement();
  activeElement = null;
}

interface ICustomEventInit {
  detail?: unknown;
}

class MockCustomEvent {
  type: string;
  detail: unknown;
  constructor(type: string, init?: ICustomEventInit) {
    this.type = type;
    this.detail = init?.detail;
  }
}

class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onopen: EventSourceVoidFn | null = null;
  onmessage: EventSourceMessageFn | null = null;
  onerror: EventSourceVoidFn | null = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  close(): void {
    this.closed = true;
  }
}

interface IMockWindowGlobals {
  HTMLIFrameElement: typeof MockElement;
  HTMLElement: typeof MockElement;
  HTMLDivElement: typeof MockElement;
  document: MockDocument;
  window: typeof globalThis;
  CustomEvent: typeof MockCustomEvent;
  EventSource: typeof MockEventSource;
  addEventListener: (evt: string, fn: EventListenerFn) => void;
  removeEventListener: (evt: string, fn: EventListenerFn) => void;
  dispatchEvent: (evt: ICustomDispatchEvent) => boolean;
}

function HookHarness({ apiBaseUrl, token, onUpdate }: IHookHarnessProps) {
  const state = useEventStream(apiBaseUrl, token);
  useEffect(() => {
    onUpdate(state);
  }, [state, onUpdate]);
  return null;
}

describe("useEventStream Hook", () => {
  let mockDoc: MockDocument;
  let root: Root;
  let container: MockElement;
  let latestState: IEventStreamState | null = null;
  let windowListeners: Record<string, EventListenerFn[]> = {};

  const handleUpdate = (state: IEventStreamState) => {
    latestState = state;
  };

  beforeEach(() => {
    MockEventSource.instances = [];
    latestState = null;
    windowListeners = {};

    mockDoc = new MockDocument();
    container = mockDoc.createElement("div");
    root = createRoot(container as unknown as Element);

    const globals = globalThis as unknown as IMockWindowGlobals;
    globals.HTMLIFrameElement = MockElement;
    globals.HTMLElement = MockElement;
    globals.HTMLDivElement = MockElement;
    globals.document = mockDoc;
    globals.window = globalThis;
    globals.CustomEvent = MockCustomEvent;
    globals.EventSource = MockEventSource;

    globals.addEventListener = (evt: string, fn: EventListenerFn) => {
      const list = windowListeners[evt] ?? [];
      list.push(fn);
      windowListeners[evt] = list;
    };

    globals.removeEventListener = (evt: string, fn: EventListenerFn) => {
      const list = windowListeners[evt] ?? [];
      windowListeners[evt] = list.filter((f) => f !== fn);
    };

    globals.dispatchEvent = (evt: ICustomDispatchEvent) => {
      const fns = windowListeners[evt.type] ?? [];
      for (const fn of fns) {
        fn(evt);
      }
      return true;
    };
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
  });

  it("exports useEventStream hook function", () => {
    expect(typeof useEventStream).toBe("function");
  });

  it("initializes with empty state and connects to EventSource endpoint with token", async () => {
    await act(async () => {
      root.render(
        jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", token: "secret-abc", onUpdate: handleUpdate }),
      );
    });

    assert(latestState !== null);
    expect(latestState.agents).toEqual([]);
    expect(latestState.environments).toEqual([]);
    expect(latestState.isConnected).toBe(false);
    expect(latestState.error).toBeNull();

    expect(MockEventSource.instances.length).toBe(1);
    expect(MockEventSource.instances[0]?.url).toBe("http://localhost:4502/api/v1/events?token=secret-abc");
  });

  it("updates isConnected to true and clears error on EventSource open", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    const esInstance = MockEventSource.instances[0];
    assert(esInstance !== undefined);

    await act(async () => {
      esInstance.onopen?.();
    });

    assert(latestState !== null);
    expect(latestState.isConnected).toBe(true);
    expect(latestState.error).toBeNull();
  });

  it("populates agents and environments state on snapshot event", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    const esInstance = MockEventSource.instances[0];
    assert(esInstance !== undefined);

    const snapshotEvent: ServerEvent = {
      type: "snapshot",
      at: 1789850000000,
      agents: [fixture_eventStreamAgent1],
      environments: [fixture_eventStreamEnvironment1],
    };

    await act(async () => {
      esInstance.onmessage?.({ data: JSON.stringify(snapshotEvent) });
    });

    assert(latestState !== null);
    expect(latestState.agents.length).toBe(1);
    expect(latestState.agents[0]?.id).toBe("env-1:claude-code:session-1");
    expect(latestState.environments.length).toBe(1);
    expect(latestState.environments[0]?.id).toBe("env-1");
  });

  it("updates existing agent in state on agent_upsert event", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    const esInstance = MockEventSource.instances[0];
    assert(esInstance !== undefined);

    await act(async () => {
      esInstance.onmessage?.({
        data: JSON.stringify({
          type: "snapshot",
          agents: [fixture_eventStreamAgent1],
          environments: [fixture_eventStreamEnvironment1],
        }),
      });
    });

    const upsertEvent: ServerEvent = {
      type: "agent_upsert",
      at: 1789850000000,
      agent: fixture_eventStreamAgent1Updated,
    };

    await act(async () => {
      esInstance.onmessage?.({ data: JSON.stringify(upsertEvent) });
    });

    assert(latestState !== null);
    expect(latestState.agents.length).toBe(1);
    expect(latestState.agents[0]?.activity.title).toBe("Implement feature A (Completed)");
    expect(latestState.agents[0]?.activity.lifecycle).toBe("EXITED");
  });

  it("prepends new agent to state on agent_upsert event", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    const esInstance = MockEventSource.instances[0];
    assert(esInstance !== undefined);

    await act(async () => {
      esInstance.onmessage?.({
        data: JSON.stringify({
          type: "snapshot",
          agents: [fixture_eventStreamAgent1],
          environments: [fixture_eventStreamEnvironment1],
        }),
      });
    });

    const upsertNewAgent: ServerEvent = {
      type: "agent_upsert",
      at: 1789850000000,
      agent: fixture_eventStreamAgent2,
    };

    await act(async () => {
      esInstance.onmessage?.({ data: JSON.stringify(upsertNewAgent) });
    });

    assert(latestState !== null);
    expect(latestState.agents.length).toBe(2);
    expect(latestState.agents[0]?.id).toBe("env-2:pi:session-2");
    expect(latestState.agents[1]?.id).toBe("env-1:claude-code:session-1");
  });

  it("removes agent from state on agent_removed event", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    const esInstance = MockEventSource.instances[0];
    assert(esInstance !== undefined);

    await act(async () => {
      esInstance.onmessage?.({
        data: JSON.stringify({
          type: "snapshot",
          agents: [fixture_eventStreamAgent1, fixture_eventStreamAgent2],
          environments: [fixture_eventStreamEnvironment1, fixture_eventStreamEnvironment2],
        }),
      });
    });

    assert(latestState !== null);
    expect(latestState.agents.length).toBe(2);

    const removeEvent: ServerEvent = {
      type: "agent_removed",
      at: 1789850000000,
      agentId: "env-1:claude-code:session-1",
    };

    await act(async () => {
      esInstance.onmessage?.({ data: JSON.stringify(removeEvent) });
    });

    expect(latestState.agents.length).toBe(1);
    expect(latestState.agents[0]?.id).toBe("env-2:pi:session-2");
  });

  it("updates existing environment and appends new environment on environment_upsert", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    const esInstance = MockEventSource.instances[0];
    assert(esInstance !== undefined);

    await act(async () => {
      esInstance.onmessage?.({
        data: JSON.stringify({
          type: "snapshot",
          at: 1789850000000,
          agents: [],
          environments: [fixture_eventStreamEnvironment1],
        }),
      });
    });

    assert(latestState !== null);
    expect(latestState.environments.length).toBe(1);

    const updatedEnv1 = { ...fixture_eventStreamEnvironment1, label: "macbook-pro-renamed" };
    await act(async () => {
      esInstance.onmessage?.({
        data: JSON.stringify({
          type: "environment_upsert",
          at: 1789850000000,
          environment: updatedEnv1,
        }),
      });
    });

    expect(latestState.environments.length).toBe(1);
    expect(latestState.environments[0]?.label).toBe("macbook-pro-renamed");

    await act(async () => {
      esInstance.onmessage?.({
        data: JSON.stringify({
          type: "environment_upsert",
          at: 1789850000000,
          environment: fixture_eventStreamEnvironment2,
        }),
      });
    });

    expect(latestState.environments.length).toBe(2);
    expect(latestState.environments[1]?.id).toBe("env-2");
  });

  it("handles disconnection and sets reconnecting error message on EventSource stream error", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    const esInstance = MockEventSource.instances[0];
    assert(esInstance !== undefined);

    await act(async () => {
      esInstance.onopen?.();
    });

    assert(latestState !== null);
    expect(latestState.isConnected).toBe(true);

    await act(async () => {
      esInstance.onerror?.();
    });

    expect(latestState.isConnected).toBe(false);
    expect(latestState.error).toBe("Disconnected from agent-server stream. Reconnecting...");
    expect(esInstance.closed).toBe(true);
  });

  it("handles custom mock-sse-event window dispatch", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { apiBaseUrl: "http://localhost:4502", onUpdate: handleUpdate }));
    });

    await act(async () => {
      globalThis.dispatchEvent(
        new MockCustomEvent("mock-sse-event", {
          detail: {
            type: "snapshot",
            at: 1789850000000,
            agents: [fixture_eventStreamAgent2],
            environments: [fixture_eventStreamEnvironment2],
          },
        }) as unknown as Event,
      );
    });

    assert(latestState !== null);
    expect(latestState.agents.length).toBe(1);
    expect(latestState.agents[0]?.id).toBe("env-2:pi:session-2");
  });
});
