import { describe, it, expect } from "bun:test";
import { formatA11yTree, serializeA11yTree, type IA11yNode } from "../a11ySnapshot";

describe("a11ySnapshot utility", () => {
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
            type: "Card",
            value: "Task One",
            children: [
              { type: "Heading", value: "Task One" },
              { type: "Badge", value: "local-dev" },
              { type: "Callout", label: "warning", value: "Needs confirmation" },
            ],
          },
          {
            type: "Card",
            value: "Task Two",
            children: [
              { type: "Heading", value: "Task Two" },
              { type: "Badge", value: "remote-prod" },
              { type: "Callout", label: "error", value: "Process failed" },
            ],
          },
        ],
      };

      const expected = [
        'Banner: "Act Now — 2 Agents Require Attention"',
        '  ├─ Heading: "Act Now — 2 Agents Require Attention"',
        '  ├─ Card: "Task One"',
        '  │  ├─ Heading: "Task One"',
        "  │  ├─ Badge: (local-dev)",
        '  │  └─ Callout (warning): "Needs confirmation"',
        '  └─ Card: "Task Two"',
        '     ├─ Heading: "Task Two"',
        "     ├─ Badge: (remote-prod)",
        '     └─ Callout (error): "Process failed"',
      ].join("\n");

      expect(serializeA11yTree(node)).toBe(expected);
    });
  });

  describe("formatA11yTree from HTML strings", () => {
    it("formats an agent card structure from HTML markup", () => {
      const html = `
        <div data-testid="AgentCard" role="button" aria-label="View details for Untitled Agent Session">
          <div data-testid="AgentCard--header">
            <span data-testid="AgentCard--env-badge" role="status">local-dev</span>
            <span data-testid="AgentCard--harness-badge" role="status">Claude Code</span>
            <span data-testid="StatusBadge" data-lifecycle="RUNNING">
              <span data-testid="StatusBadge--label">Busy</span>
            </span>
            <button data-testid="AgentCard--hide-button" title="Hide agent session" aria-label="Hide agent session">Hide</button>
          </div>
          <h3 data-testid="AgentCard--title">Untitled Agent Session</h3>
          <p data-testid="AgentCard--action">Executing Edit</p>
          <div data-testid="AgentCard--question" role="alert">Do you want to run this command?</div>
          <div data-testid="AgentCard--badges">
            <div data-badge-type="branch">main</div>
            <div data-badge-type="subagents">2 subagents</div>
          </div>
          <div data-testid="AgentCard--footer">
            <div data-testid="AgentCard--cost">$0.0045</div>
            <div data-testid="AgentCard--usage">1,800 tokens</div>
          </div>
        </div>
      `;

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

      expect(formatA11yTree(html)).toBe(expected);
    });

    it("formats an attention banner structure from HTML markup", () => {
      const html = `
        <div data-testid="AttentionBanner">
          <h2 data-testid="AttentionBanner--title">Act Now — 1 Agent Requires Attention</h2>
          <div data-testid="AttentionBanner--grid">
            <div data-testid="AttentionBanner--agent-card" role="button" aria-label="View agent needing attention: Fix database migrations">
              <span data-testid="AttentionBanner--agent-title">Fix database migrations</span>
              <span data-testid="AttentionBanner--agent-env">local-dev</span>
              <p data-testid="AttentionBanner--agent-message">Database migration failed on step 3</p>
            </div>
          </div>
        </div>
      `;

      const tree = formatA11yTree(html);
      expect(tree).toContain('Banner: "Act Now — 1 Agent Requires Attention"');
      expect(tree).toContain('Heading: "Act Now — 1 Agent Requires Attention"');
      expect(tree).toContain('Card: "Fix database migrations"');
      expect(tree).toContain("Badge: (local-dev)");
      expect(tree).toContain('Callout (error): "Database migration failed on step 3"');
    });

    it("returns empty string when given null, undefined, or empty markup", () => {
      expect(formatA11yTree("")).toBe("");
      expect(formatA11yTree(null as unknown as string)).toBe("");
      expect(formatA11yTree(undefined as unknown as string)).toBe("");
    });

    it("parses generic DOM elements with headings, buttons, and status", () => {
      const html = `
        <div aria-label="Settings Card">
          <h2>Project Settings</h2>
          <span role="status">Active</span>
          <button title="Save settings">Save</button>
          <div role="alert">Configuration warning</div>
          <p>Unsaved changes</p>
        </div>
      `;

      const tree = formatA11yTree(html);
      expect(tree).toContain('Card: "Settings Card"');
      expect(tree).toContain('Heading: "Project Settings"');
      expect(tree).toContain("Badge: (Active)");
      expect(tree).toContain("Button: [Save settings]");
      expect(tree).toContain('Callout (error): "Configuration warning"');
      expect(tree).toContain('Text: "Unsaved changes"');
    });

    it("parses DOM objects with MockElement-like structures", () => {
      const mockObj = {
        tagName: "DIV",
        attributes: {
          "data-testid": "AgentCard",
          role: "button",
        },
        children: [
          {
            tagName: "H3",
            attributes: { "data-testid": "AgentCard--title" },
            textContent: "Mock Agent",
            children: [],
          },
          {
            tagName: "SPAN",
            attributes: { "data-testid": "AgentCard--env-badge" },
            textContent: "test-env",
            children: [],
          },
          {
            tagName: "SPAN",
            attributes: { "data-testid": "AgentCard--harness-badge" },
            textContent: "Pi",
            children: [],
          },
          {
            tagName: "SPAN",
            attributes: { "data-testid": "StatusBadge", "data-lifecycle": "EXITED" },
            textContent: "Finished",
            children: [],
          },
          {
            tagName: "P",
            attributes: { "data-testid": "AgentCard--action" },
            textContent: "Idle",
            children: [],
          },
        ],
        textContent: "Mock Agent test-env Pi Finished Idle",
      };

      const tree = formatA11yTree(mockObj);
      expect(tree).toContain('Card: "Mock Agent"');
      expect(tree).toContain("Badge: (test-env)");
      expect(tree).toContain("Badge: (Pi)");
      expect(tree).toContain("Status: (EXITED)");
      expect(tree).toContain('Heading: "Mock Agent"');
      expect(tree).toContain('Action: "Idle"');
    });
  });
});
