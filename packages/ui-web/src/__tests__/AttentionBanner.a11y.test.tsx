import { describe, it, expect } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { AttentionBanner } from "../components/AttentionBanner";
import { formatA11yTree } from "../utils/a11ySnapshot";
import { fixture_agentStateWaitingQuestion, fixture_agentStateError } from "./fixtures";

const noop = (): void => {};

describe("AttentionBanner Semantic Accessibility Snapshots", () => {
  it("matches accessibility snapshot for multiple attention agents (question and error)", () => {
    const markup = renderToStaticMarkup(
      <AttentionBanner agents={[fixture_agentStateWaitingQuestion, fixture_agentStateError]} onSelectAgent={noop} />,
    );
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Banner: "Act Now — 2 Agents Require Attention"',
        '  ├─ Heading: "Act Now — 2 Agents Require Attention"',
        '  ├─ Card: "Implement user authentication flow"',
        '  │  ├─ Heading: "Implement user authentication flow"',
        "  │  ├─ Badge: (work-macbook)",
        '  │  ├─ Callout (warning): "Should we use SQLite or PostgreSQL for auth tokens?"',
        "  │  └─ Button: [View agent needing attention: Implement user authentication flow]",
        '  └─ Card: "Database schema migration"',
        '     ├─ Heading: "Database schema migration"',
        "     ├─ Badge: (work-macbook)",
        "     ├─ Callout (error): \"Database migration returned exit code 1: syntax error near 'INDEX'\"",
        "     └─ Button: [View agent needing attention: Database schema migration]",
      ].join("\n"),
    );
  });

  it("matches accessibility snapshot for single attention agent", () => {
    const markup = renderToStaticMarkup(
      <AttentionBanner agents={[fixture_agentStateWaitingQuestion]} onSelectAgent={noop} />,
    );
    const tree = formatA11yTree(markup);

    expect(tree).toBe(
      [
        'Banner: "Act Now — 1 Agent Require Attention"',
        '  ├─ Heading: "Act Now — 1 Agent Require Attention"',
        '  └─ Card: "Implement user authentication flow"',
        '     ├─ Heading: "Implement user authentication flow"',
        "     ├─ Badge: (work-macbook)",
        '     ├─ Callout (warning): "Should we use SQLite or PostgreSQL for auth tokens?"',
        "     └─ Button: [View agent needing attention: Implement user authentication flow]",
      ].join("\n"),
    );
  });

  it("returns empty snapshot when no agents require attention", () => {
    const markup = renderToStaticMarkup(<AttentionBanner agents={[]} onSelectAgent={noop} />);
    const tree = formatA11yTree(markup);
    expect(tree).toBe("");
  });
});
