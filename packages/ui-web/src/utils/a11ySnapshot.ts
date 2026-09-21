export type A11yNodeType =
  | "Card"
  | "Banner"
  | "Badge"
  | "Status"
  | "Button"
  | "Heading"
  | "Action"
  | "Callout"
  | "Cost"
  | "Usage"
  | "Text";

export interface IA11yNode {
  type: A11yNodeType;
  label?: string;
  value: string;
  children?: IA11yNode[];
}

export interface IElementLike {
  tagName: string;
  attributes: Record<string, string>;
  children: IElementLike[];
  textContent: string;
  getAttribute(name: string): string | null;
  querySelector(selector: string): IElementLike | null;
  querySelectorAll(selector: string): IElementLike[];
}

/**
 * Serializes an IA11yNode hierarchy into a deterministic indented text tree
 * using standard box-drawing characters (├─, └─, │).
 */
export function serializeA11yTree(root: IA11yNode): string {
  const lines = serializeNode(root, "", true, true);
  return lines.join("\n");
}

function formatNodeLabel(node: IA11yNode): string {
  switch (node.type) {
    case "Card":
      return `Card: "${node.value}"`;
    case "Banner":
      return `Banner: "${node.value}"`;
    case "Badge":
      return `Badge: (${node.value})`;
    case "Status":
      return `Status: (${node.value})`;
    case "Button":
      return `Button: [${node.value}]`;
    case "Heading":
      return `Heading: "${node.value}"`;
    case "Action":
      return `Action: "${node.value}"`;
    case "Callout":
      return `Callout (${node.label || "info"}): "${node.value}"`;
    case "Cost":
      return `Cost: "${node.value}"`;
    case "Usage":
      return `Usage: "${node.value}"`;
    case "Text":
      return `Text: "${node.value}"`;
    default:
      return `${node.type}: "${node.value}"`;
  }
}

function serializeNode(node: IA11yNode, prefix: string, isLast: boolean, isRoot: boolean): string[] {
  const line = formatNodeLabel(node);
  const children = node.children ?? [];

  if (isRoot) {
    const result = [line];
    for (let i = 0; i < children.length; i++) {
      const childIsLast = i === children.length - 1;
      result.push(...serializeNode(children[i], "  ", childIsLast, false));
    }
    return result;
  }

  const branch = isLast ? "└─ " : "├─ ";
  const currentLine = `${prefix}${branch}${line}`;
  const result = [currentLine];
  const nextPrefix = `${prefix}${isLast ? "   " : "│  "}`;

  for (let i = 0; i < children.length; i++) {
    const childIsLast = i === children.length - 1;
    result.push(...serializeNode(children[i], nextPrefix, childIsLast, false));
  }

  return result;
}

class ElementLike implements IElementLike {
  tagName: string;
  attributes: Record<string, string> = {};
  children: IElementLike[] = [];
  textParts: string[] = [];

  constructor(tagName: string) {
    this.tagName = tagName.toUpperCase();
  }

  getAttribute(name: string): string | null {
    return this.attributes[name.toLowerCase()] ?? this.attributes[name] ?? null;
  }

  get textContent(): string {
    if (this.children.length === 0) {
      return this.textParts.join("");
    }
    const parts: string[] = [];
    for (const child of this.children) {
      parts.push(child.textContent);
    }
    for (const text of this.textParts) {
      parts.push(text);
    }
    return parts.join(" ").replace(/\s+/g, " ").trim();
  }

  querySelector(selector: string): IElementLike | null {
    const all = this.querySelectorAll(selector);
    return all.length > 0 ? all[0] : null;
  }

  querySelectorAll(selector: string): IElementLike[] {
    const results: IElementLike[] = [];
    this.matchSelector(selector, results);
    return results;
  }

  private matchSelector(selector: string, results: IElementLike[]): void {
    const testIdMatch = selector.match(/^\[data-testid="([^"]+)"\]$/);
    const badgeTypeMatch = selector.match(/^\[data-badge-type="([^"]+)"\]$/);
    const roleMatch = selector.match(/^\[role="([^"]+)"\]$/);
    const tagMatch = selector.match(/^[a-zA-Z0-9]+$/);

    let isMatch = false;

    if (testIdMatch) {
      isMatch = this.getAttribute("data-testid") === testIdMatch[1];
    } else if (badgeTypeMatch) {
      isMatch = this.getAttribute("data-badge-type") === badgeTypeMatch[1];
    } else if (roleMatch) {
      isMatch = this.getAttribute("role") === roleMatch[1];
    } else if (tagMatch) {
      isMatch = this.tagName === tagMatch[0].toUpperCase();
    } else if (selector.includes(",")) {
      const parts = selector.split(",").map((s) => s.trim());
      for (const part of parts) {
        if (this.matchesSingle(part)) {
          isMatch = true;
          break;
        }
      }
    }

    if (isMatch) {
      results.push(this);
    }

    for (const child of this.children) {
      if (child instanceof ElementLike) {
        child.matchSelector(selector, results);
      }
    }
  }

  private matchesSingle(singleSelector: string): boolean {
    const testIdMatch = singleSelector.match(/^\[data-testid="([^"]+)"\]$/);
    if (testIdMatch) {
      return this.getAttribute("data-testid") === testIdMatch[1];
    }
    const badgeTypeMatch = singleSelector.match(/^\[data-badge-type="([^"]+)"\]$/);
    if (badgeTypeMatch) {
      return this.getAttribute("data-badge-type") === badgeTypeMatch[1];
    }
    const roleMatch = singleSelector.match(/^\[role="([^"]+)"\]$/);
    if (roleMatch) {
      return this.getAttribute("role") === roleMatch[1];
    }
    return false;
  }
}

const VOID_ELEMENTS = new Set([
  "AREA",
  "BASE",
  "BR",
  "COL",
  "EMBED",
  "HR",
  "IMG",
  "INPUT",
  "LINK",
  "META",
  "PARAM",
  "SOURCE",
  "TRACK",
  "WBR",
]);

/**
 * Parses raw HTML string into an IElementLike representation.
 */
function parseHtmlToElementLike(html: string): IElementLike | null {
  const trimmed = html.trim();
  if (!trimmed) return null;

  const root = new ElementLike("ROOT");
  const stack: ElementLike[] = [root];

  // Regex to match HTML comments, tags, or plain text
  const tokenRegex =
    /<!--[\s\S]*?-->|<(\/)?([a-zA-Z0-9\-:]+)((?:\s+[^=>\s]+(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^>\s]+))?)*)\s*(\/)?>|([^<]+)/g;

  let match: RegExpExecArray | null = tokenRegex.exec(trimmed);
  while (match !== null) {
    const [fullMatch, isClosing, tagName, rawAttrs, isSelfClosing, textContent] = match;

    if (fullMatch.startsWith("<!--")) {
      // Ignore HTML comment
      match = tokenRegex.exec(trimmed);
      continue;
    }

    if (textContent) {
      const current = stack[stack.length - 1];
      const cleanText = unescapeHtml(textContent).replace(/\s+/g, " ");
      if (cleanText.trim().length > 0) {
        current.textParts.push(cleanText);
      }
    } else if (tagName) {
      if (isClosing) {
        const upperTag = tagName.toUpperCase();
        for (let i = stack.length - 1; i > 0; i--) {
          if (stack[i].tagName === upperTag) {
            stack.splice(i);
            break;
          }
        }
      } else {
        const el = new ElementLike(tagName);
        if (rawAttrs) {
          parseAttributes(rawAttrs, el.attributes);
        }

        const current = stack[stack.length - 1];
        current.children.push(el);

        const isVoid = isSelfClosing === "/" || VOID_ELEMENTS.has(el.tagName);
        if (!isVoid) {
          stack.push(el);
        }
      }
    }

    match = tokenRegex.exec(trimmed);
  }

  if (root.children.length === 1) {
    return root.children[0];
  }
  return root;
}

function parseAttributes(raw: string, target: Record<string, string>): void {
  const attrRegex = /([a-zA-Z0-9_\-:]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^>\s]+)))?/g;
  let match: RegExpExecArray | null = attrRegex.exec(raw);
  while (match !== null) {
    const [, name, valDouble, valSingle, valBare] = match;
    const value = valDouble ?? valSingle ?? valBare ?? "";
    target[name.toLowerCase()] = unescapeHtml(value);
    match = attrRegex.exec(raw);
  }
}

function unescapeHtml(str: string): string {
  return str
    .replace(/&quot;/g, '"')
    .replace(/&#34;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&#39;/g, "'")
    .replace(/&#x27;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&#60;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&#62;/g, ">")
    .replace(/&amp;/g, "&")
    .replace(/&#38;/g, "&");
}

function toElementLike(input: unknown): IElementLike | null {
  if (!input) return null;

  if (typeof input === "string") {
    return parseHtmlToElementLike(input);
  }

  if (typeof input === "object" && input !== null) {
    const obj = input as Record<string, unknown>;

    if (typeof obj.outerHTML === "string") {
      return parseHtmlToElementLike(obj.outerHTML);
    }
    if (typeof obj.innerHTML === "string") {
      return parseHtmlToElementLike(obj.innerHTML);
    }

    // MockElement support
    if ("tagName" in obj && typeof obj.tagName === "string") {
      const el = new ElementLike(obj.tagName);
      if ("attributes" in obj && typeof obj.attributes === "object" && obj.attributes !== null) {
        Object.assign(el.attributes, obj.attributes);
      }
      if ("children" in obj && Array.isArray(obj.children)) {
        for (const child of obj.children) {
          const converted = toElementLike(child);
          if (converted) el.children.push(converted);
        }
      }
      if ("textContent" in obj && typeof obj.textContent === "string") {
        el.textParts.push(obj.textContent);
      }
      return el;
    }
  }

  return null;
}

/**
 * Extracts semantic IA11yNode hierarchy from a component element or container.
 */
export function parseA11yTree(container: unknown): IA11yNode | null {
  const el = toElementLike(container);
  if (!el) return null;

  // 1. AgentCard Pattern
  const cardEl = el.getAttribute("data-testid") === "AgentCard" ? el : el.querySelector('[data-testid="AgentCard"]');

  if (cardEl) {
    return parseAgentCard(cardEl);
  }

  // 2. AttentionBanner Pattern
  const bannerEl =
    el.getAttribute("data-testid") === "AttentionBanner" ? el : el.querySelector('[data-testid="AttentionBanner"]');

  if (bannerEl) {
    return parseAttentionBanner(bannerEl);
  }

  // 3. Fallback generic container
  return parseGenericElement(el);
}

function parseAgentCard(card: IElementLike): IA11yNode {
  const titleEl = card.querySelector('[data-testid="AgentCard--title"]');
  const title = titleEl ? titleEl.textContent.trim() : "Untitled Agent Session";

  const children: IA11yNode[] = [];

  // 1. Environment badge
  const envEl = card.querySelector('[data-testid="AgentCard--env-badge"]');
  if (envEl) {
    children.push({ type: "Badge", value: envEl.textContent.trim() });
  }

  // 2. Harness badge
  const harnessEl = card.querySelector('[data-testid="AgentCard--harness-badge"]');
  if (harnessEl) {
    children.push({ type: "Badge", value: harnessEl.textContent.trim() });
  }

  // 3. Status badge
  const statusEl = card.querySelector('[data-testid="StatusBadge"]');
  if (statusEl) {
    const attrLifecycle = statusEl.getAttribute("data-lifecycle");
    if (attrLifecycle) {
      children.push({ type: "Status", value: attrLifecycle });
    } else {
      const labelEl = statusEl.querySelector('[data-testid="StatusBadge--label"]') ?? statusEl;
      const label = labelEl.textContent.trim();
      const isExited = label === "Finished" || label === "Abandoned prompt" || label === "Died on error";
      children.push({ type: "Status", value: isExited ? "EXITED" : "RUNNING" });
    }
  }

  // 4. Hidden badge
  const hiddenEl = card.querySelector('[data-testid="AgentCard--hidden-badge"]');
  if (hiddenEl) {
    children.push({ type: "Badge", value: "Hidden" });
  }

  // 5. Hide / Unhide button
  const hideBtn = card.querySelector(
    '[data-testid="AgentCard--hide-button"], [data-testid="AgentCard--unhide-button"]',
  );
  if (hideBtn) {
    const btnLabel = hideBtn.getAttribute("title") || hideBtn.getAttribute("aria-label") || hideBtn.textContent.trim();
    children.push({ type: "Button", value: btnLabel });
  }

  // 6. Heading
  children.push({ type: "Heading", value: title });

  // 7. Action
  const actionEl = card.querySelector('[data-testid="AgentCard--action"]');
  if (actionEl) {
    children.push({ type: "Action", value: actionEl.textContent.trim() });
  }

  // 8. Callouts (question or error)
  const questionEl = card.querySelector('[data-testid="AgentCard--question"]');
  if (questionEl) {
    children.push({ type: "Callout", label: "warning", value: questionEl.textContent.trim() });
  }

  const errorEl = card.querySelector('[data-testid="AgentCard--error"]');
  if (errorEl) {
    children.push({ type: "Callout", label: "error", value: errorEl.textContent.trim() });
  }

  // 9. Badges
  const branchEl = card.querySelector('[data-testid="AgentCard--branch-badge"], [data-badge-type="branch"]');
  if (branchEl) {
    children.push({ type: "Badge", value: `branch: ${branchEl.textContent.trim()}` });
  }

  const worktreeEl = card.querySelector('[data-testid="AgentCard--worktree-badge"], [data-badge-type="worktree"]');
  if (worktreeEl) {
    children.push({ type: "Badge", value: `worktree: ${worktreeEl.textContent.trim()}` });
  }

  const subagentsEl = card.querySelector('[data-testid="AgentCard--subagents-badge"], [data-badge-type="subagents"]');
  if (subagentsEl) {
    children.push({ type: "Badge", value: subagentsEl.textContent.trim() });
  }

  // 10. Cost
  const costEl = card.querySelector('[data-testid="AgentCard--cost"]');
  if (costEl) {
    children.push({ type: "Cost", value: costEl.textContent.trim() });
  }

  // 11. Usage
  const usageEl = card.querySelector('[data-testid="AgentCard--usage"]');
  if (usageEl) {
    children.push({ type: "Usage", value: usageEl.textContent.trim() });
  }

  return {
    type: "Card",
    value: title,
    children,
  };
}

function parseAttentionBanner(banner: IElementLike): IA11yNode {
  const headerEl = banner.querySelector('[data-testid="AttentionBanner--title"]');
  const bannerTitle = headerEl ? headerEl.textContent.trim() : "Attention Required";

  const children: IA11yNode[] = [{ type: "Heading", value: bannerTitle }];

  const agentCards = banner.querySelectorAll('[data-testid="AttentionBanner--agent-card"]');
  for (const card of agentCards) {
    const titleEl = card.querySelector('[data-testid="AttentionBanner--agent-title"]');
    const ariaLabel = card.getAttribute("aria-label");
    let cardTitle = "Untitled Agent";

    if (titleEl) {
      cardTitle = titleEl.textContent.trim();
    } else if (ariaLabel) {
      cardTitle = ariaLabel.replace(/^View agent needing attention:\s*/, "");
    }

    const cardChildren: IA11yNode[] = [{ type: "Heading", value: cardTitle }];

    const envEl =
      card.querySelector('[data-testid="AttentionBanner--agent-env"]') ??
      card.children.find(
        (c) => c.tagName === "SPAN" && c.getAttribute("data-testid") !== "AttentionBanner--agent-title",
      );
    if (envEl) {
      cardChildren.push({ type: "Badge", value: envEl.textContent.trim() });
    }

    const messageEl =
      card.querySelector('[data-testid="AttentionBanner--agent-message"]') ??
      card.children.find((c) => c.tagName === "P");
    if (messageEl) {
      const text = messageEl.textContent.trim();
      const isQuestion = text.includes("Waiting on user question") || text.includes("?");
      cardChildren.push({
        type: "Callout",
        label: isQuestion ? "warning" : "error",
        value: text,
      });
    }

    cardChildren.push({
      type: "Button",
      value: ariaLabel || `View agent needing attention: ${cardTitle}`,
    });

    children.push({
      type: "Card",
      value: cardTitle,
      children: cardChildren,
    });
  }

  return {
    type: "Banner",
    value: bannerTitle,
    children,
  };
}

function parseGenericElement(el: IElementLike): IA11yNode {
  const children: IA11yNode[] = [];

  for (const child of el.children) {
    const role = child.getAttribute("role");
    const testId = child.getAttribute("data-testid") || "";
    const text = child.textContent.trim();

    if (!text) continue;

    if (child.tagName === "BUTTON" || role === "button") {
      const btnTitle = child.getAttribute("title") || child.getAttribute("aria-label") || text;
      children.push({ type: "Button", value: btnTitle });
    } else if (role === "status" || testId.includes("badge")) {
      children.push({ type: "Badge", value: text });
    } else if (role === "alert") {
      children.push({ type: "Callout", label: "error", value: text });
    } else if (/^H[1-6]$/.test(child.tagName) || role === "heading") {
      children.push({ type: "Heading", value: text });
    } else if (child.children.length > 0) {
      const nested = parseGenericElement(child);
      if (nested.children && nested.children.length > 0) {
        children.push(...nested.children);
      } else {
        children.push(nested);
      }
    } else {
      children.push({ type: "Text", value: text });
    }
  }

  const rootTitle = el.getAttribute("aria-label") || el.getAttribute("title") || el.tagName;
  return {
    type: "Card",
    value: rootTitle,
    children,
  };
}

/**
 * Traverses an HTMLElement, MockElement, or HTML markup string and serializes
 * its semantic accessibility tree using [buttons] and (badges) notation.
 */
export function formatA11yTree(container: unknown): string {
  const tree = parseA11yTree(container);
  if (!tree) return "";
  return serializeA11yTree(tree);
}
