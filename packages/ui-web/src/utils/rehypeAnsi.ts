import {
  createAnsiCsiRegex,
  createNonSgrAnsiRegex,
  parseAnsiCodes,
  stateToStyleString,
  hasActiveStyle,
  type IAnsiStyle,
} from "./ansiUtils";

export interface IHastTextNode {
  type: "text";
  value: string;
}

export interface IHastElementNode {
  type: "element";
  tagName: string;
  properties?: Record<string, unknown>;
  children: HastNode[];
}

export interface IHastRootNode {
  type: "root";
  children: HastNode[];
}

export type HastNode = IHastTextNode | IHastElementNode | IHastRootNode;

export type RehypePlugin = (tree: HastNode) => void;

export function isHastTextNode(node: HastNode): node is IHastTextNode {
  return node.type === "text" && "value" in node;
}

export function isHastElementNode(node: HastNode): node is IHastElementNode {
  return node.type === "element" && "tagName" in node && Array.isArray(node.children);
}

export function isHastRootNode(node: HastNode): node is IHastRootNode {
  return node.type === "root" && Array.isArray(node.children);
}

export function rehypeAnsi(): RehypePlugin {
  return (tree: HastNode): void => {
    const state: IAnsiStyle = {};

    function transformChildren(children: HastNode[]): HastNode[] {
      const result: HastNode[] = [];

      for (const child of children) {
        if (isHastTextNode(child)) {
          const nonSgrRegex = createNonSgrAnsiRegex();
          const ansiCsiRegex = createAnsiCsiRegex();
          const cleanedText = child.value.replace(nonSgrRegex, "");
          let lastIndex = 0;
          let match: RegExpExecArray | null = ansiCsiRegex.exec(cleanedText);

          while (match !== null) {
            const rawText = cleanedText.slice(lastIndex, match.index);
            if (rawText) {
              const hasVisibleWhitespaceStyle = Boolean(state.backgroundColor || state.textDecoration);
              if (hasActiveStyle(state) && (rawText.trim() || hasVisibleWhitespaceStyle)) {
                result.push({
                  type: "element",
                  tagName: "span",
                  properties: { style: stateToStyleString(state) },
                  children: [{ type: "text", value: rawText }],
                });
              } else {
                result.push({ type: "text", value: rawText });
              }
            }
            parseAnsiCodes(match[1], state);
            lastIndex = ansiCsiRegex.lastIndex;
            match = ansiCsiRegex.exec(cleanedText);
          }

          const remainingText = cleanedText.slice(lastIndex);
          if (remainingText) {
            const hasVisibleWhitespaceStyle = Boolean(state.backgroundColor || state.textDecoration);
            if (hasActiveStyle(state) && (remainingText.trim() || hasVisibleWhitespaceStyle)) {
              result.push({
                type: "element",
                tagName: "span",
                properties: { style: stateToStyleString(state) },
                children: [{ type: "text", value: remainingText }],
              });
            } else {
              result.push({ type: "text", value: remainingText });
            }
          }
        } else if (isHastElementNode(child)) {
          child.children = transformChildren(child.children);
          result.push(child);
        } else {
          result.push(child);
        }
      }

      return result;
    }

    if (isHastRootNode(tree) || isHastElementNode(tree)) {
      tree.children = transformChildren(tree.children);
    }
  };
}
