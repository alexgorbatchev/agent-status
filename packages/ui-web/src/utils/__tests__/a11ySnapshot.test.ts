import { describe, it, expect } from "bun:test";
import { formatA11yTree, serializeA11yTree, isDomElement, type IA11yNode } from "../a11ySnapshot";
import { MockDocument, type MockElement } from "../../__tests__/helpers";

interface IMockWrapper {
  container: MockElement;
}

describe("a11ySnapshot utility", () => {
  describe("isDomElement type guard", () => {
    it("identifies valid MockElement and rejects invalid objects", () => {
      const doc = new MockDocument();
      const el = doc.createElement("div");

      expect(isDomElement(el)).toBe(true);
      expect(isDomElement(null)).toBe(false);
      expect(isDomElement(undefined)).toBe(false);
      expect(isDomElement("string")).toBe(false);
      expect(isDomElement(123)).toBe(false);
      expect(isDomElement({})).toBe(false);
      expect(isDomElement({ nodeType: 2 })).toBe(false);
    });
  });

  describe("serializeA11yTree", () => {
    it("serializes a single root card without children", () => {
      const node: IA11yNode = {
        type: "Card",
        value: "Sample Task",
      };

      const tree = serializeA11yTree(node);
      expect(tree).toBe('Card: "Sample Task"');
    });

    it("serializes a tree with buttons, badges, status, callouts, and metrics", () => {
      const node: IA11yNode = {
        type: "Card",
        value: "Untitled Agent Session",
        children: [
          { type: "Badge", value: "local-dev" },
          { type: "Badge", value: "Claude Code" },
          { type: "Status", value: "RUNNING" },
          { type: "Button", value: "Hide agent session" },
          { type: "Heading", value: "Untitled Agent Session" },
          { type: "Action", value: "Executing Edit" },
          { type: "Callout", label: "warning", value: "Do you want to run this command?" },
          { type: "Badge", value: "branch: main" },
          { type: "Badge", value: "2 subagents" },
          { type: "Cost", value: "$0.0045" },
          { type: "Usage", value: "1,800 tokens" },
        ],
      };

      const expected = [
        'Card: "Untitled Agent Session"',
        "  ├─ Badge: (local-dev)",
        "  ├─ Badge: (Claude Code)",
        "  ├─ Status: (RUNNING)",
        "  ├─ Button: [Hide agent session]",
        '  ├─ Heading: "Untitled Agent Session"',
        '  ├─ Action: "Executing Edit"',
        '  ├─ Callout (warning): "Do you want to run this command?"',
        "  ├─ Badge: (branch: main)",
        "  ├─ Badge: (2 subagents)",
        '  ├─ Cost: "$0.0045"',
        '  └─ Usage: "1,800 tokens"',
      ].join("\n");

      expect(serializeA11yTree(node)).toBe(expected);
    });

    it("serializes multi-level nested trees with correct Unicode box drawing characters", () => {
      const node: IA11yNode = {
        type: "Banner",
        value: "Act Now — 2 Agents Require Attention",
        children: [
          {
            type: "Heading",
            value: "Act Now — 2 Agents Require Attention",
          },
          {
            type: "Button",
            value: "View agent needing attention: Task One",
            children: [
              { type: "Badge", value: "local-dev" },
              { type: "Callout", label: "warning", value: "Needs confirmation" },
            ],
          },
          {
            type: "Button",
            value: "View agent needing attention: Task Two",
            children: [
              { type: "Badge", value: "remote-prod" },
              { type: "Callout", label: "error", value: "Process failed" },
            ],
          },
        ],
      };

      const expected = [
        'Banner: "Act Now — 2 Agents Require Attention"',
        '  ├─ Heading: "Act Now — 2 Agents Require Attention"',
        "  ├─ Button: [View agent needing attention: Task One]",
        "  │  ├─ Badge: (local-dev)",
        '  │  └─ Callout (warning): "Needs confirmation"',
        "  └─ Button: [View agent needing attention: Task Two]",
        "     ├─ Badge: (remote-prod)",
        '     └─ Callout (error): "Process failed"',
      ].join("\n");

      expect(serializeA11yTree(node)).toBe(expected);
    });
  });

  describe("formatA11yTree with native DOM elements", () => {
    it("formats an agent card structure from native MockElement DOM tree", () => {
      const doc = new MockDocument();

      const card = doc.createElement("div");
      card.setAttribute("role", "article");
      card.setAttribute("aria-label", "Untitled Agent Session");

      const header = doc.createElement("div");
      const envBadge = doc.createElement("span");
      envBadge.setAttribute("role", "status");
      envBadge.textContent = "local-dev";
      header.appendChild(envBadge);

      const harnessBadge = doc.createElement("span");
      harnessBadge.setAttribute("role", "status");
      harnessBadge.textContent = "Claude Code";
      header.appendChild(harnessBadge);

      const statusBadge = doc.createElement("span");
      statusBadge.setAttribute("role", "status");
      statusBadge.setAttribute("data-lifecycle", "RUNNING");
      statusBadge.textContent = "Busy";
      header.appendChild(statusBadge);

      const hideButton = doc.createElement("button");
      hideButton.setAttribute("title", "Hide agent session");
      hideButton.textContent = "Hide";
      header.appendChild(hideButton);

      card.appendChild(header);

      const heading = doc.createElement("h3");
      heading.textContent = "Untitled Agent Session";
      card.appendChild(heading);

      const action = doc.createElement("p");
      action.setAttribute("data-action", "true");
      action.textContent = "Executing Edit";
      card.appendChild(action);

      const callout = doc.createElement("div");
      callout.setAttribute("role", "alert");
      callout.setAttribute("data-callout-type", "warning");
      callout.textContent = "Do you want to run this command?";
      card.appendChild(callout);

      const badgesContainer = doc.createElement("div");
      const branchBadge = doc.createElement("div");
      branchBadge.setAttribute("role", "status");
      branchBadge.setAttribute("aria-label", "branch: main");
      branchBadge.textContent = "main";
      badgesContainer.appendChild(branchBadge);

      const subagentsBadge = doc.createElement("div");
      subagentsBadge.setAttribute("role", "status");
      subagentsBadge.setAttribute("aria-label", "2 subagents");
      subagentsBadge.textContent = "2 subagents";
      badgesContainer.appendChild(subagentsBadge);

      const activeBadge = doc.createElement("div");
      activeBadge.setAttribute("role", "status");
      activeBadge.setAttribute("aria-label", "active: 2m ago");
      activeBadge.textContent = "2m ago";
      badgesContainer.appendChild(activeBadge);

      card.appendChild(badgesContainer);

      const footer = doc.createElement("div");
      const costEl = doc.createElement("div");
      costEl.setAttribute("data-metric", "cost");
      costEl.textContent = "$0.0045";
      footer.appendChild(costEl);

      const usageEl = doc.createElement("div");
      usageEl.setAttribute("data-metric", "usage");
      usageEl.textContent = "1,800 tokens";
      footer.appendChild(usageEl);

      card.appendChild(footer);

      const tree = formatA11yTree(card);
      const expected = [
        'Card: "Untitled Agent Session"',
        "  ├─ Badge: (local-dev)",
        "  ├─ Badge: (Claude Code)",
        "  ├─ Status: (RUNNING)",
        "  ├─ Button: [Hide agent session]",
        '  ├─ Heading: "Untitled Agent Session"',
        '  ├─ Action: "Executing Edit"',
        '  ├─ Callout (warning): "Do you want to run this command?"',
        "  ├─ Badge: (branch: main)",
        "  ├─ Badge: (2 subagents)",
        "  ├─ Badge: (active: 2m ago)",
        '  ├─ Cost: "$0.0045"',
        '  └─ Usage: "1,800 tokens"',
      ].join("\n");

      expect(tree).toBe(expected);
    });

    it("formats an attention banner structure from native DOM elements", () => {
      const doc = new MockDocument();

      const banner = doc.createElement("div");
      banner.setAttribute("role", "region");
      banner.setAttribute("aria-label", "Act Now — 1 Agent Requires Attention");

      const title = doc.createElement("h2");
      title.textContent = "Act Now — 1 Agent Requires Attention";
      banner.appendChild(title);

      const grid = doc.createElement("div");

      const agentButton = doc.createElement("div");
      agentButton.setAttribute("role", "button");
      agentButton.setAttribute("aria-label", "View agent needing attention: Fix database migrations");

      const envSpan = doc.createElement("span");
      envSpan.setAttribute("role", "status");
      envSpan.textContent = "local-dev";
      agentButton.appendChild(envSpan);

      const message = doc.createElement("p");
      message.setAttribute("role", "alert");
      message.setAttribute("data-callout-type", "error");
      message.textContent = "Database migration failed on step 3";
      agentButton.appendChild(message);

      grid.appendChild(agentButton);
      banner.appendChild(grid);

      const tree = formatA11yTree(banner);
      const expected = [
        'Banner: "Act Now — 1 Agent Requires Attention"',
        '  ├─ Heading: "Act Now — 1 Agent Requires Attention"',
        "  └─ Button: [View agent needing attention: Fix database migrations]",
        "     ├─ Badge: (local-dev)",
        '     └─ Callout (error): "Database migration failed on step 3"',
      ].join("\n");

      expect(tree).toBe(expected);
    });

    it("returns empty string when given null, undefined, or empty container", () => {
      expect(formatA11yTree(null)).toBe("");
      expect(formatA11yTree(undefined)).toBe("");
      const doc = new MockDocument();
      const emptyDiv = doc.createElement("div");
      expect(formatA11yTree(emptyDiv)).toBe("");
    });

    it("handles wrapper container with container property or firstElementChild", () => {
      const doc = new MockDocument();
      const wrapper: IMockWrapper = {
        container: doc.createElement("div"),
      };
      wrapper.container.setAttribute("role", "article");
      wrapper.container.setAttribute("aria-label", "Wrapped Card");

      const heading = doc.createElement("h2");
      heading.textContent = "Wrapped Heading";
      wrapper.container.appendChild(heading);

      const tree = formatA11yTree(wrapper);
      expect(tree).toBe('Card: "Wrapped Card"\n  └─ Heading: "Wrapped Heading"');
    });

    it("extracts accessible name from nested heading when card lacks aria-label and title", () => {
      const doc = new MockDocument();
      const card = doc.createElement("div");
      card.setAttribute("role", "article");

      const nested = doc.createElement("div");
      const h4 = doc.createElement("h4");
      h4.textContent = "Inferred Card Title";
      nested.appendChild(h4);
      card.appendChild(nested);

      const tree = formatA11yTree(card);
      expect(tree).toContain('Card: "Inferred Card Title"');
    });

    it("handles data-card, data-banner, data-badge-type, role=alertdialog, and data-role=action", () => {
      const doc = new MockDocument();
      const banner = doc.createElement("div");
      banner.setAttribute("data-banner", "true");
      banner.setAttribute("aria-label", "Custom Banner");

      const card = doc.createElement("div");
      card.setAttribute("data-card", "true");

      const action = doc.createElement("p");
      action.setAttribute("data-role", "action");
      action.textContent = "Running custom action";
      card.appendChild(action);

      const alert = doc.createElement("div");
      alert.setAttribute("role", "alertdialog");
      alert.textContent = "Critical confirmation needed";
      card.appendChild(alert);

      const badge = doc.createElement("span");
      badge.setAttribute("data-badge-type", "tag");
      badge.textContent = "v1.0.0";
      card.appendChild(badge);

      banner.appendChild(card);

      const tree = formatA11yTree(banner);
      expect(tree).toContain('Banner: "Custom Banner"');
      expect(tree).toContain('Card: "Card"');
      expect(tree).toContain('Action: "Running custom action"');
      expect(tree).toContain('Callout (error): "Critical confirmation needed"');
      expect(tree).toContain("Badge: (v1.0.0)");
    });

    it("parses headings and buttons when container has multiple semantic items", () => {
      const doc = new MockDocument();
      const host = doc.createElement("div");

      const heading = doc.createElement("h1");
      heading.textContent = "Title 1";
      host.appendChild(heading);

      const button = doc.createElement("button");
      button.textContent = "Click Me";
      host.appendChild(button);

      const tree = formatA11yTree(host);
      expect(tree).toContain('Heading: "Title 1"');
      expect(tree).toContain("Button: [Click Me]");
    });
  });
});
