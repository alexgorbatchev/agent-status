import { describe, it, expect, beforeEach, afterEach } from "bun:test";
import assert from "node:assert";
import { act, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { jsx } from "react/jsx-runtime";
import { useRelativeTime } from "../useRelativeTime";

type EventListenerFn = (event: unknown) => void;
type StateUpdateCallback = (value: string) => void;

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
  timestamp: number;
  intervalMs?: number;
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

interface IMockWindowGlobals {
  HTMLIFrameElement: typeof MockElement;
  HTMLElement: typeof MockElement;
  HTMLDivElement: typeof MockElement;
  document: MockDocument;
  window: typeof globalThis;
}

function HookHarness({ timestamp, intervalMs, onUpdate }: IHookHarnessProps) {
  const formatted = useRelativeTime(timestamp, intervalMs);
  useEffect(() => {
    onUpdate(formatted);
  }, [formatted, onUpdate]);
  return null;
}

describe("useRelativeTime Hook", () => {
  let mockDoc: MockDocument;
  let root: Root;
  let container: MockElement;
  let latestValue: string | null = null;

  const handleUpdate = (value: string) => {
    latestValue = value;
  };

  beforeEach(() => {
    latestValue = null;
    mockDoc = new MockDocument();
    container = mockDoc.createElement("div");
    root = createRoot(container as unknown as Element);

    const globals = globalThis as unknown as IMockWindowGlobals;
    globals.HTMLIFrameElement = MockElement;
    globals.HTMLElement = MockElement;
    globals.HTMLDivElement = MockElement;
    globals.document = mockDoc;
    globals.window = globalThis;
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
  });

  it("exports useRelativeTime hook function", () => {
    expect(typeof useRelativeTime).toBe("function");
  });

  it("returns initial relative time formatted string", async () => {
    const now = Date.now();
    await act(async () => {
      root.render(jsx(HookHarness, { timestamp: now - 10000, onUpdate: handleUpdate }));
    });

    assert(latestValue !== null);
    expect(latestValue).toBe("10s ago");
  });

  it("returns '—' for 0 or negative timestamp without running timer", async () => {
    await act(async () => {
      root.render(jsx(HookHarness, { timestamp: 0, onUpdate: handleUpdate }));
    });

    assert(latestValue !== null);
    expect(latestValue).toBe("—");
  });

  it("updates formatted string after elapsed interval", async () => {
    const now = Date.now();
    await act(async () => {
      root.render(jsx(HookHarness, { timestamp: now - 4000, intervalMs: 50, onUpdate: handleUpdate }));
    });

    assert(latestValue !== null);
    expect(latestValue).toBe("just now");

    // Wait for timer to tick past 5s threshold wrapped in act
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 1100));
    });

    assert(latestValue !== null);
    expect(latestValue).toMatch(/^[5-6]s ago$/);
  });

  it("updates immediately when timestamp prop changes", async () => {
    const now = Date.now();
    await act(async () => {
      root.render(jsx(HookHarness, { timestamp: now - 60000, onUpdate: handleUpdate }));
    });

    assert(latestValue !== null);
    expect(latestValue).toBe("1m ago");

    await act(async () => {
      root.render(jsx(HookHarness, { timestamp: now - 120000, onUpdate: handleUpdate }));
    });

    expect(latestValue).toBe("2m ago");
  });
});
