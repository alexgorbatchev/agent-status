import { describe, it, expect } from "bun:test";
import React from "react";
import { AttentionBanner } from "../components/AttentionBanner";
import { formatA11yTree } from "../utils/a11ySnapshot";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import { fixture_agentStateWaitingQuestion, fixture_agentStateError } from "./fixtures";

const noop = (): void => {};

describe("AttentionBanner Semantic Accessibility Snapshots", () => {
  it("matches accessibility snapshot for multiple attention agents (question and error)", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const { container, unmount } = await mountComponent(
        <AttentionBanner agents={[fixture_agentStateWaitingQuestion, fixture_agentStateError]} onSelectAgent={noop} />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Banner: "Act Now — 2 Agents Require Attention"',
          '  ├─ Heading: "Act Now — 2 Agents Require Attention"',
          "  ├─ Button: [View agent needing attention: Implement user authentication flow]",
          "  │  ├─ Badge: (work-macbook)",
          '  │  └─ Callout (warning): "Should we use SQLite or PostgreSQL for auth tokens?"',
          "  └─ Button: [View agent needing attention: Database schema migration]",
          "     ├─ Badge: (work-macbook)",
          "     └─ Callout (error): \"Database migration returned exit code 1: syntax error near 'INDEX'\"",
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("matches accessibility snapshot for single attention agent", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const { container, unmount } = await mountComponent(
        <AttentionBanner agents={[fixture_agentStateWaitingQuestion]} onSelectAgent={noop} />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();

      expect(tree).toBe(
        [
          'Banner: "Act Now — 1 Agent Requires Attention"',
          '  ├─ Heading: "Act Now — 1 Agent Requires Attention"',
          "  └─ Button: [View agent needing attention: Implement user authentication flow]",
          "     ├─ Badge: (work-macbook)",
          '     └─ Callout (warning): "Should we use SQLite or PostgreSQL for auth tokens?"',
        ].join("\n"),
      );
    } finally {
      cleanup();
    }
  });

  it("returns empty snapshot when no agents require attention", async () => {
    const { mockDoc, cleanup } = setupMockDomEnvironment();
    try {
      const { container, unmount } = await mountComponent(
        <AttentionBanner agents={[]} onSelectAgent={noop} />,
        mockDoc,
      );
      const tree = formatA11yTree(container);
      await unmount();
      expect(tree).toBe("");
    } finally {
      cleanup();
    }
  });
});
