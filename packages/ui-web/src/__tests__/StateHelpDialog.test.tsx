import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import { StateHelpDialog } from "../components/StateHelpDialog";

describe("StateHelpDialog Component", () => {
  it("renders dialog with header, title, legend items, and close buttons", () => {
    const html = renderToString(<StateHelpDialog onClose={() => {}} />);

    expect(html).toContain('data-testid="StateHelpDialog"');
    expect(html).toContain('role="dialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain('data-testid="StateHelpDialog--title"');
    expect(html).toContain("Agent States &amp; Lifecycle Guide");
    expect(html).toContain('data-testid="StateHelpDialog--close-button"');
    expect(html).toContain('data-testid="StateHelpDialog--content"');
    expect(html).toContain("Active vs. Idle vs. Finished");
    expect(html).toContain("Running Agent States");
    expect(html).toContain("Exited Agent States");
    expect(html).toContain('data-testid="StateHelpDialog--got-it-button"');
  });

  it("renders all status badge variations in the guide", () => {
    const html = renderToString(<StateHelpDialog onClose={() => {}} />);

    expect(html).toContain("Actively working");
    expect(html).toContain("Idle &amp; waiting for input");
    expect(html).toContain("Waiting on question response");
    expect(html).toContain("Execution error");
    expect(html).toContain("Rate limited");
    expect(html).toContain("Completed turn");
    expect(html).toContain("Finished");
    expect(html).toContain("Died on error");
    expect(html).toContain("Abandoned prompt");
  });

  it("explains running error state distinguishing unhandled runtime errors from normal command failures", () => {
    const html = renderToString(<StateHelpDialog onClose={() => {}} />);

    expect(html).toContain("unhandled runtime error");
    expect(html).toContain("preserve Working status");
  });

  it("explains idle freshness tiering and dormant/stale dimming behavior", () => {
    const html = renderToString(<StateHelpDialog onClose={() => {}} />);

    expect(html).toContain("Idle freshness tiering");
    expect(html).toContain("Fresh");
    expect(html).toContain("&lt; 15m");
    expect(html).toContain("Standard");
    expect(html).toContain("15m–2h");
    expect(html).toContain("Dormant / Stale");
    expect(html).toContain("&gt; 2h");
    expect(html).toContain("dimmed");
  });

  it("has accessible button labels and semantics", () => {
    const html = renderToString(<StateHelpDialog onClose={() => {}} />);

    expect(html).toContain('aria-label="Close state legend help dialog"');
    expect(html).toContain('type="button"');
  });
});
