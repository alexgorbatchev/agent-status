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

export interface IDomNode {
  nodeType: number;
  nodeValue?: string | null;
  textContent?: string | null;
}

export interface IDomElement extends IDomNode {
  nodeType: number;
  tagName: string;
  children: ArrayLike<IDomElement> | IDomElement[];
  childNodes?: ArrayLike<IDomNode> | IDomNode[];
  getAttribute(name: string): string | null;
  textContent: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

export function isDomElement(node: unknown): node is IDomElement {
  if (!isRecord(node)) {
    return false;
  }
  return (
    typeof node.nodeType === "number" &&
    node.nodeType === 1 &&
    typeof node.tagName === "string" &&
    typeof node.getAttribute === "function" &&
    "children" in node
  );
}

function cleanText(text: string | null | undefined): string {
  if (!text) {
    return "";
  }
  return text.replace(/\s+/g, " ").trim();
}

function getAccessibleText(element: IDomElement): string {
  if (element.getAttribute("aria-hidden") === "true") {
    return "";
  }
  if (element.childNodes && element.childNodes.length > 0) {
    const parts: string[] = [];
    for (const childNode of Array.from(element.childNodes)) {
      if (childNode.nodeType === 3) {
        const text = cleanText(childNode.nodeValue ?? childNode.textContent);
        if (text) {
          parts.push(text);
        }
      } else if (isDomElement(childNode)) {
        if (childNode.getAttribute("aria-hidden") !== "true") {
          const childText = getAccessibleText(childNode);
          if (childText) {
            parts.push(childText);
          }
        }
      }
    }
    return cleanText(parts.join(" "));
  }
  const children = Array.from(element.children);
  if (children.length === 0) {
    return cleanText(element.textContent);
  }
  const parts: string[] = [];
  for (const child of children) {
    if (child.getAttribute("aria-hidden") !== "true") {
      const childText = getAccessibleText(child);
      if (childText) {
        parts.push(childText);
      }
    }
  }
  return cleanText(parts.join(" "));
}

function findHeadingText(element: IDomElement): string | null {
  const children = Array.from(element.children);
  for (const child of children) {
    if (child.getAttribute("aria-hidden") === "true") {
      continue;
    }
    const tag = child.tagName.toUpperCase();
    const role = child.getAttribute("role");
    if (/^H[1-6]$/.test(tag) || role === "heading") {
      const text = getAccessibleText(child);
      if (text) {
        return text;
      }
    }
    const nested = findHeadingText(child);
    if (nested) {
      return nested;
    }
  }
  return null;
}

function getAccessibleName(element: IDomElement): string {
  const ariaLabel = element.getAttribute("aria-label");
  if (ariaLabel && cleanText(ariaLabel)) {
    return cleanText(ariaLabel);
  }
  const title = element.getAttribute("title");
  if (title && cleanText(title)) {
    return cleanText(title);
  }
  return getAccessibleText(element);
}

interface IClassificationResult {
  type: A11yNodeType;
  value: string;
  label?: string;
  isContainer: boolean;
}

function classifyElement(element: IDomElement): IClassificationResult | null {
  if (element.getAttribute("aria-hidden") === "true") {
    return null;
  }
  const role = element.getAttribute("role");
  const tagName = element.tagName.toUpperCase();

  // 1. Card container: role="article" or data-card or data-testid="AgentCard"
  if (
    role === "article" ||
    element.getAttribute("data-card") !== null ||
    element.getAttribute("data-testid") === "AgentCard"
  ) {
    const rawAria = element.getAttribute("aria-label")?.trim();
    const heading = findHeadingText(element);
    let accessibleName = "";
    if (rawAria) {
      accessibleName = rawAria.replace(/^View details for\s+/i, "");
    }
    if (!accessibleName && heading) {
      accessibleName = heading;
    }
    if (!accessibleName) {
      accessibleName = element.getAttribute("title")?.trim() || "Card";
    }
    return {
      type: "Card",
      value: cleanText(accessibleName),
      isContainer: true,
    };
  }

  // 2. Banner container: role="region" or role="banner" or data-banner or data-testid="AttentionBanner"
  if (
    role === "region" ||
    role === "banner" ||
    element.getAttribute("data-banner") !== null ||
    element.getAttribute("data-testid") === "AttentionBanner"
  ) {
    const accessibleName =
      element.getAttribute("aria-label")?.trim() ||
      element.getAttribute("title")?.trim() ||
      findHeadingText(element) ||
      "Banner";
    return {
      type: "Banner",
      value: cleanText(accessibleName),
      isContainer: true,
    };
  }

  // 3. Headings: h1-h6 or role="heading"
  if (/^H[1-6]$/.test(tagName) || role === "heading") {
    const text = getAccessibleText(element);
    if (text) {
      return {
        type: "Heading",
        value: text,
        isContainer: false,
      };
    }
  }

  // 4. Status badge: role="status" with data-lifecycle
  if (role === "status" && element.getAttribute("data-lifecycle") !== null) {
    const lifecycle = element.getAttribute("data-lifecycle")?.trim();
    return {
      type: "Status",
      value: lifecycle || cleanText(element.textContent),
      isContainer: false,
    };
  }

  // 5. Badges: role="status" or data-badge-type or data-badge
  if (
    role === "status" ||
    element.getAttribute("data-badge-type") !== null ||
    element.getAttribute("data-badge") !== null
  ) {
    const ariaLabel = element.getAttribute("aria-label");
    const val = ariaLabel?.trim() || cleanText(element.textContent);
    if (val) {
      return {
        type: "Badge",
        value: val,
        isContainer: false,
      };
    }
  }

  // 6. Action line: data-action or data-role="action" or data-testid="AgentCard--action"
  if (
    element.getAttribute("data-action") !== null ||
    element.getAttribute("data-role") === "action" ||
    element.getAttribute("data-testid") === "AgentCard--action"
  ) {
    const text = cleanText(element.textContent);
    if (text) {
      return {
        type: "Action",
        value: text,
        isContainer: false,
      };
    }
  }

  // 7. Callouts: role="alert" or role="alertdialog"
  if (role === "alert" || role === "alertdialog") {
    const text = cleanText(element.textContent);
    if (text) {
      const calloutType = element.getAttribute("data-callout-type");
      const isWarning =
        calloutType === "warning" || (!calloutType && (text.includes("?") || text.toLowerCase().includes("waiting")));
      return {
        type: "Callout",
        label: isWarning ? "warning" : "error",
        value: text,
        isContainer: false,
      };
    }
  }

  // 8. Cost metric: data-metric="cost" or data-testid="AgentCard--cost"
  if (element.getAttribute("data-metric") === "cost" || element.getAttribute("data-testid") === "AgentCard--cost") {
    const val = element.getAttribute("aria-label")?.trim() || cleanText(element.textContent);
    if (val) {
      return {
        type: "Cost",
        value: val,
        isContainer: false,
      };
    }
  }

  // 9. Usage metric: data-metric="usage" or data-testid="AgentCard--usage"
  if (element.getAttribute("data-metric") === "usage" || element.getAttribute("data-testid") === "AgentCard--usage") {
    const val = element.getAttribute("aria-label")?.trim() || cleanText(element.textContent);
    if (val) {
      return {
        type: "Usage",
        value: val,
        isContainer: false,
      };
    }
  }

  // 10. Buttons: button tag or role="button"
  if (tagName === "BUTTON" || role === "button") {
    const accessibleName = getAccessibleName(element);
    return {
      type: "Button",
      value: accessibleName,
      isContainer: true,
    };
  }

  return null;
}

function collectSemanticNodes(element: IDomElement): IA11yNode[] {
  const results: IA11yNode[] = [];
  const children = Array.from(element.children);

  for (const child of children) {
    if (child.getAttribute("aria-hidden") === "true") {
      continue;
    }
    const classified = classifyElement(child);

    if (classified) {
      const node: IA11yNode = {
        type: classified.type,
        value: classified.value,
      };
      if (classified.label) {
        node.label = classified.label;
      }

      if (classified.isContainer) {
        const subChildren = collectSemanticNodes(child);
        if (subChildren.length > 0) {
          node.children = subChildren;
        }
      }

      results.push(node);
    } else {
      // Recurse into layout containers (div, span, etc.) to collect semantic descendants
      const nested = collectSemanticNodes(child);
      results.push(...nested);
    }
  }

  return results;
}

function toElement(container: unknown): IDomElement | null {
  if (isDomElement(container)) {
    return container;
  }
  if (isRecord(container)) {
    if ("container" in container && isDomElement(container.container)) {
      return container.container;
    }
    if ("firstElementChild" in container && isDomElement(container.firstElementChild)) {
      return container.firstElementChild;
    }
  }
  return null;
}

export function parseA11yTree(container: unknown): IA11yNode | null {
  const rootEl = toElement(container);
  if (!rootEl) {
    return null;
  }

  const rootClassified = classifyElement(rootEl);
  if (rootClassified) {
    const rootNode: IA11yNode = {
      type: rootClassified.type,
      value: rootClassified.value,
    };
    if (rootClassified.label) {
      rootNode.label = rootClassified.label;
    }
    const children = collectSemanticNodes(rootEl);
    if (children.length > 0) {
      rootNode.children = children;
    }
    return rootNode;
  }

  const semanticNodes = collectSemanticNodes(rootEl);
  if (semanticNodes.length === 0) {
    return null;
  }
  if (semanticNodes.length === 1) {
    return semanticNodes[0];
  }

  return {
    type: "Banner",
    value: "Root",
    children: semanticNodes,
  };
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
      return `Callout (${node.label ?? "info"}): "${node.value}"`;
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

export function serializeA11yTree(root: IA11yNode): string {
  const lines = serializeNode(root, "", true, true);
  return lines.join("\n");
}

export function formatA11yTree(container: unknown): string {
  const tree = parseA11yTree(container);
  if (!tree) {
    return "";
  }
  return serializeA11yTree(tree);
}
