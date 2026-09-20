import { describe, it, expect, mock } from "bun:test";
import assert from "node:assert";
import type { KeyboardEvent } from "react";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentCard } from "../components/AgentCard";
import { AttentionBanner } from "../components/AttentionBanner";
import { LogDrawer } from "../components/LogDrawer";
import { StateHelpDialog } from "../components/StateHelpDialog";
import { setupMockDomEnvironment, mountComponent } from "./helpers";
import { fixture_agentStateRunning, fixture_agentStateWaitingQuestion, fixture_agentStateError } from "./fixtures";

describe("Accessibility & ARIA semantics", () => {
  describe("AgentCard", () => {
    it("renders with role='button', tabIndex=0, and aria-label", () => {
      const html = renderToStaticMarkup(<AgentCard agent={fixture_agentStateRunning} />);

      expect(html).toContain('role="button"');
      expect(html).toContain('tabindex="0"');
      expect(html).toContain('aria-label="View details for Build agent-status dashboard"');
    });

    it("triggers onOpenDrawer callback on click", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      const onOpenDrawer = mock();
      const { container, unmount } = await mountComponent(
        <AgentCard agent={fixture_agentStateRunning} onOpenDrawer={onOpenDrawer} />,
        mockDoc,
      );

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card);
      card.click();

      expect(onOpenDrawer).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledWith(fixture_agentStateRunning);
      await unmount();
      cleanup();
    });

    it("triggers onOpenDrawer callback on Enter key down", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      const onOpenDrawer = mock();
      const { container, unmount } = await mountComponent(
        <AgentCard agent={fixture_agentStateRunning} onOpenDrawer={onOpenDrawer} />,
        mockDoc,
      );

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card);

      const preventDefault = mock();
      card.dispatchEvent({
        type: "keydown",
        key: "Enter",
        preventDefault,
      });

      expect(preventDefault).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledWith(fixture_agentStateRunning);
      await unmount();
      cleanup();
    });

    it("triggers onOpenDrawer callback on Space key down", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      const onOpenDrawer = mock();
      const { container, unmount } = await mountComponent(
        <AgentCard agent={fixture_agentStateRunning} onOpenDrawer={onOpenDrawer} />,
        mockDoc,
      );

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card);

      const preventDefault = mock();
      card.dispatchEvent({
        type: "keydown",
        key: " ",
        preventDefault,
      });

      expect(preventDefault).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledTimes(1);
      expect(onOpenDrawer).toHaveBeenCalledWith(fixture_agentStateRunning);
      await unmount();
      cleanup();
    });

    it("does not trigger onOpenDrawer on other keys", async () => {
      const { mockDoc, cleanup } = setupMockDomEnvironment();
      const onOpenDrawer = mock();
      const { container, unmount } = await mountComponent(
        <AgentCard agent={fixture_agentStateRunning} onOpenDrawer={onOpenDrawer} />,
        mockDoc,
      );

      const card = container.querySelector('[data-testid="AgentCard"]');
      assert(card);

      const preventDefault = mock();
      card.dispatchEvent({
        type: "keydown",
        key: "Tab",
        preventDefault,
      });

      expect(preventDefault).toHaveBeenCalledTimes(0);
      expect(onOpenDrawer).toHaveBeenCalledTimes(0);
      await unmount();
      cleanup();
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

  describe("StateHelpDialog", () => {
    it("renders dialog with role='dialog', aria-modal='true', and close button with type='button'", () => {
      const html = renderToStaticMarkup(<StateHelpDialog onClose={() => {}} />);

      expect(html).toContain('role="dialog"');
      expect(html).toContain('aria-modal="true"');
      expect(html).toContain('aria-labelledby="StateHelpDialog--title"');
      expect(html).toContain('type="button"');
      expect(html).toContain('aria-label="Close state legend help dialog"');
    });
  });
});
