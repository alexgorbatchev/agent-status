import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import type { JSX } from "react";

type EventListenerFn = (event: unknown) => void;

interface ICustomDispatchEvent {
  type: string;
  key?: string;
  target?: unknown;
  currentTarget?: unknown;
  bubbles?: boolean;
  cancelBubble?: boolean;
  preventDefault?: () => void;
  stopPropagation?: () => void;
}

interface ITextNode {
  nodeType: number;
  nodeValue: string;
  ownerDocument: MockDocument | null;
  parentNode?: MockElement | null;
}

interface ICommentNode {
  nodeType: number;
  nodeValue: string;
  ownerDocument: MockDocument;
}

type MockDomNode = MockElement | ITextNode;

export class MockElement {
  nodeType = 1;
  nodeName = "DIV";
  tagName = "DIV";
  children: MockElement[] = [];
  childNodes: MockDomNode[] = [];
  style: Record<string, string> = {};
  ownerDocument: MockDocument | null = null;
  parentNode: MockElement | null = null;
  listeners: Record<string, EventListenerFn[]> = {};
  attributes: Record<string, string> = {};

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
    if (!event.target) {
      event.target = this;
    }
    event.currentTarget = this;
    const fns = this.listeners[event.type] ?? [];
    for (const fn of fns) {
      fn(event);
    }
    if (!event.cancelBubble && this.parentNode) {
      this.parentNode.dispatchEvent(event);
    }
    return true;
  }

  appendChild(child: MockDomNode): MockDomNode {
    this.childNodes.push(child);
    if ("tagName" in child) {
      this.children.push(child);
    }
    child.parentNode = this;
    return child;
  }

  removeChild(child: MockDomNode): MockDomNode {
    if ("tagName" in child) {
      this.children = this.children.filter((c) => c !== child);
    }
    this.childNodes = this.childNodes.filter((c) => c !== child);
    child.parentNode = null;
    return child;
  }

  insertBefore(child: MockDomNode, _ref: MockDomNode | null): MockDomNode {
    return this.appendChild(child);
  }

  setAttribute(name: string, value: string): void {
    this.attributes[name] = value;
  }

  removeAttribute(name: string): void {
    delete this.attributes[name];
  }

  getAttribute(name: string): string | null {
    return this.attributes[name] ?? null;
  }

  click(): void {
    this.dispatchEvent({ type: "click" });
  }

  get textContent(): string {
    let text = "";
    for (const child of this.childNodes) {
      if ("nodeValue" in child && typeof child.nodeValue === "string") {
        text += child.nodeValue;
      } else if ("textContent" in child && typeof child.textContent === "string") {
        text += child.textContent;
      }
    }
    return text;
  }

  set textContent(value: string) {
    this.childNodes = [];
    this.children = [];
    if (value !== "") {
      const textNode: ITextNode = {
        nodeType: 3,
        nodeValue: value,
        ownerDocument: this.ownerDocument,
      };
      this.appendChild(textNode);
    }
  }

  querySelector(selector: string): MockElement | null {
    const testIdMatch = selector.match(/\[data-testid="([^"]+)"\]/);
    if (testIdMatch && testIdMatch[1]) {
      const targetTestId = testIdMatch[1];
      if (this.attributes["data-testid"] === targetTestId) {
        return this;
      }
      for (const child of this.children) {
        if ("querySelector" in child && typeof child.querySelector === "function") {
          const found = child.querySelector(selector);
          if (found) return found;
        }
      }
    }
    return null;
  }

  querySelectorAll(selector: string): MockElement[] {
    const results: MockElement[] = [];
    const testIdMatch = selector.match(/\[data-testid="([^"]+)"\]/);
    if (testIdMatch && testIdMatch[1]) {
      const targetTestId = testIdMatch[1];
      if (this.attributes["data-testid"] === targetTestId) {
        results.push(this);
      }
      for (const child of this.children) {
        if ("querySelectorAll" in child && typeof child.querySelectorAll === "function") {
          results.push(...child.querySelectorAll(selector));
        }
      }
    }
    return results;
  }
}

export class MockDocument {
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
  IS_REACT_ACT_ENVIRONMENT: boolean;
}

export interface IMockDomEnvironment {
  mockDoc: MockDocument;
  cleanup: () => void;
}

export function setupMockDomEnvironment(): IMockDomEnvironment {
  const mockDoc = new MockDocument();
  const globals = globalThis as unknown as IMockWindowGlobals;
  globals.HTMLIFrameElement = MockElement;
  globals.HTMLElement = MockElement;
  globals.HTMLDivElement = MockElement;
  globals.document = mockDoc;
  globals.window = globalThis;
  globals.IS_REACT_ACT_ENVIRONMENT = true;

  return {
    mockDoc,
    cleanup: () => {
      mockDoc.body = new MockElement();
    },
  };
}

export interface IRenderResult {
  container: MockElement;
  unmount: () => Promise<void>;
}

export async function mountComponent(element: JSX.Element, mockDoc: MockDocument): Promise<IRenderResult> {
  const container = mockDoc.createElement("div");
  const root: Root = createRoot(container as unknown as Element);

  await act(async () => {
    root.render(element);
  });

  return {
    container,
    unmount: async () => {
      await act(async () => {
        root.unmount();
      });
    },
  };
}
