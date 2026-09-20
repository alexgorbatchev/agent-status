import { describe, it, expect, mock } from "bun:test";
import type { KeyboardEvent } from "react";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentCard } from "../components/AgentCard";
import { AttentionBanner } from "../components/AttentionBanner";
import { LogDrawer } from "../components/LogDrawer";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion, fixture_agentStateError } from "./fixtures";

describe("Accessibility & ARIA semantics", () => {
  describe("AgentCard", () => {
    it("renders with role='button', tabIndex=0, and aria-label", () => {
      const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} />);

      expect(html).toContain('role="button"');
      expect(html).toContain('tabindex="0"');
      expect(html).toContain('aria-label="View details for Build agent-status dashboard"');
    });

    it("triggers onOpenDrawer callback on click", () => {
      const onOpenDrawer = mock();
      const element = AgentCard({ agent: fixture_agentStateRunning, onOpenDrawer });

      element.props.onClick();
      expect(onOpenDrawer).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledWith(fixture_agentStateRunning);
    });

    it("triggers onOpenDrawer callback on Enter key down", () => {
      const onOpenDrawer = mock();
      const element = AgentCard({ agent: fixture_agentStateRunning, onOpenDrawer });

      const preventDefault = mock();
      const event = {
        key: "Enter",
        preventDefault,
      } as unknown as KeyboardEvent<HTMLDivElement>;

      element.props.onKeyDown(event);

      expect(preventDefault).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledWith(fixture_agentStateRunning);
    });

    it("triggers onOpenDrawer callback on Space key down", () => {
      const onOpenDrawer = mock();
      const element = AgentCard({ agent: fixture_agentStateRunning, onOpenDrawer });

      const preventDefault = mock();
      const event = {
        key: " ",
        preventDefault,
      } as unknown as KeyboardEvent<HTMLDivElement>;

      element.props.onKeyDown(event);

      expect(preventDefault).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledWith(fixture_agentStateRunning);
    });

    it("does not trigger onOpenDrawer on other keys", () => {
      const onOpenDrawer = mock();
      const element = AgentCard({ agent: fixture_agentStateRunning, onOpenDrawer });

      const preventDefault = mock();
      const event = {
        key: "Tab",
        preventDefault,
      } as unknown as KeyboardEvent<HTMLDivElement>;

      element.props.onKeyDown(event);

      expect(preventDefault).toHaveBeenCalledTimes(0);
      expect(onOpenDrawer).toHaveBeenCalledTimes(0);
    });
  });

  describe("AttentionBanner", () => {
    it("renders agent cards with role='button', tabIndex=0, and descriptive aria-labels", () => {
      const html = renderToStaticMarkup(
        <AttentionBanner
          agents={[fixture_agentStateWaitingQuestion, fixture_agentStateError]}
          onSelectAgent={() => {}}
        />,
      );

      expect(html).toContain('role="button"');
      expect(html).toContain('tabindex="0"');
      expect(html).toContain('aria-label="View agent needing attention: Implement user authentication flow"');
      expect(html).toContain('aria-label="View agent needing attention: Database schema migration"');
    });

    it("triggers onSelectAgent callback on Enter key down", () => {
      const onSelectAgent = mock();
      const banner = AttentionBanner({
        agents: [fixture_agentStateWaitingQuestion],
        onSelectAgent,
      });

      expect(banner).not.toBeNull();
      const grid = banner?.props.children[1];
      const card = grid.props.children[0];

      const preventDefault = mock();
      const event = {
        key: "Enter",
        preventDefault,
      } as unknown as KeyboardEvent<HTMLDivElement>;

      card.props.onKeyDown(event);

      expect(preventDefault).toHaveBeenCalledTimes(1);
      expect(onSelectAgent).toHaveBeenCalledTimes(1);
      expect(onSelectAgent).toHaveBeenCalledWith(fixture_agentStateWaitingQuestion);
    });

    it("triggers onSelectAgent callback on Space key down", () => {
      const onSelectAgent = mock();
      const banner = AttentionBanner({
        agents: [fixture_agentStateWaitingQuestion],
        onSelectAgent,
      });

      expect(banner).not.toBeNull();
      const grid = banner?.props.children[1];
      const card = grid.props.children[0];

      const preventDefault = mock();
      const event = {
        key: " ",
        preventDefault,
      } as unknown as KeyboardEvent<HTMLDivElement>;

      card.props.onKeyDown(event);

      expect(preventDefault).toHaveBeenCalledTimes(1);
      expect(onSelectAgent).toHaveBeenCalledTimes(1);
      expect(onSelectAgent).toHaveBeenCalledWith(fixture_agentStateWaitingQuestion);
    });

    it("does not trigger onSelectAgent on other keys", () => {
      const onSelectAgent = mock();
      const banner = AttentionBanner({
        agents: [fixture_agentStateWaitingQuestion],
        onSelectAgent,
      });

      expect(banner).not.toBeNull();
      const grid = banner?.props.children[1];
      const card = grid.props.children[0];

      const preventDefault = mock();
      const event = {
        key: "Escape",
        preventDefault,
      } as unknown as KeyboardEvent<HTMLDivElement>;

      card.props.onKeyDown(event);

      expect(preventDefault).toHaveBeenCalledTimes(0);
      expect(onSelectAgent).toHaveBeenCalledTimes(0);
    });
  });

  describe("LogDrawer", () => {
    it("renders close button with type='button' and aria-label='Close log drawer'", () => {
      const html = renderToStaticMarkup(<LogDrawer agent={fixture_agentStateRunning} onClose={() => {}} />);

      expect(html).toContain('type="button"');
      expect(html).toContain('aria-label="Close log drawer"');
      expect(html).toContain('data-testid="LogDrawer--close-button"');
    });
  });
});
