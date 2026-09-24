import { describe, it, expect } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { StatusBadge } from "../components/StatusBadge";

describe("StatusBadge Component", () => {
  it("renders RUNNING + BUSY status badge with pulsing blue indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="BUSY" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="RUNNING" data-semantic-status="info" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-900/60 text-blue-200 border border-blue-700/50"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-blue-400 animate-pulse"></span><span data-testid="StatusBadge--label">Working</span></span>',
    );
  });

  it("renders RUNNING + IDLE status badge with slate indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="IDLE" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="RUNNING" data-semantic-status="neutral" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-800 text-slate-300 border border-slate-700"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-slate-500"></span><span data-testid="StatusBadge--label">Idle</span></span>',
    );
  });

  it("renders RUNNING + WAITING_QUESTION status badge with amber indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="WAITING_QUESTION" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="RUNNING" data-semantic-status="warning" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-900/60 text-amber-200 border border-amber-600 animate-pulse"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-amber-400"></span><span data-testid="StatusBadge--label">Waiting on Question</span></span>',
    );
  });

  it("renders RUNNING + ERROR status badge with red indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="ERROR" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="RUNNING" data-semantic-status="error" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-900/60 text-red-200 border border-red-600 animate-pulse"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-red-400"></span><span data-testid="StatusBadge--label">Error</span></span>',
    );
  });

  it("renders RUNNING + RATE_LIMITED status badge with pulsing amber indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="RATE_LIMITED" />);
    expect(html).toContain('data-semantic-status="warning"');
    expect(html).toContain("Rate Limited");
    expect(html).toContain("bg-amber-400");
  });

  it("renders EXITED + IDLE status badge with emerald indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="EXITED" activity="IDLE" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="EXITED" data-semantic-status="success" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/50 text-emerald-300 border border-emerald-800/60"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-emerald-500"></span><span data-testid="StatusBadge--label">Finished</span></span>',
    );
  });

  it("renders EXITED + BUSY status badge as Finished default", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="EXITED" activity="BUSY" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="EXITED" data-semantic-status="success" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-emerald-950/50 text-emerald-300 border border-emerald-800/60"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-emerald-500"></span><span data-testid="StatusBadge--label">Finished</span></span>',
    );
  });

  it("renders EXITED + WAITING_QUESTION status badge with amber indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="EXITED" activity="WAITING_QUESTION" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="EXITED" data-semantic-status="warning" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-950/40 text-amber-400 border border-amber-800/80"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-amber-500"></span><span data-testid="StatusBadge--label">Abandoned prompt</span></span>',
    );
  });

  it("renders EXITED + ERROR status badge with red indicator", () => {
    const html = renderToStaticMarkup(<StatusBadge lifecycle="EXITED" activity="ERROR" />);
    expect(html).toBe(
      '<span data-testid="StatusBadge" role="status" data-lifecycle="EXITED" data-semantic-status="error" class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-950/40 text-red-400 border border-red-800/80"><span data-testid="StatusBadge--indicator" class="w-2 h-2 rounded-full bg-red-500"></span><span data-testid="StatusBadge--label">Died on error</span></span>',
    );
  });

  it("renders RUNNING + BUSY with silence stall as Stalled (Xm) with static amber indicator", () => {
    const html = renderToStaticMarkup(
      <StatusBadge lifecycle="RUNNING" activity="BUSY" isSessionStalled={true} silenceDuration="2m" />,
    );
    expect(html).toContain('data-testid="StatusBadge"');
    expect(html).toContain("Stalled (2m)");
    expect(html).toContain("bg-amber-400");
    expect(html).not.toContain("animate-pulse");
  });

  describe("Semantic Precedence over Harness Branding (#106)", () => {
    it("strictly preserves semantic red for ERROR even when harness is supplied", () => {
      const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="ERROR" harness="claude-code" />);
      expect(html).toContain('data-semantic-status="error"');
      expect(html).toContain("bg-red-900/60");
      expect(html).toContain("text-red-200");
      expect(html).toContain("border-red-600");
      expect(html).not.toContain("brand");
      expect(html).not.toContain("claude");
    });

    it("strictly preserves semantic amber for WAITING_QUESTION even when harness is supplied", () => {
      const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="WAITING_QUESTION" harness="pi" />);
      expect(html).toContain('data-semantic-status="warning"');
      expect(html).toContain("bg-amber-900/60");
      expect(html).toContain("text-amber-200");
      expect(html).toContain("border-amber-600");
      expect(html).not.toContain("brand");
      expect(html).not.toContain("pi");
    });

    it("strictly preserves semantic blue for BUSY (working) without brand intrusion", () => {
      const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="BUSY" harness="codex" />);
      expect(html).toContain('data-semantic-status="info"');
      expect(html).toContain("bg-blue-900/60");
      expect(html).toContain("text-blue-200");
      expect(html).not.toContain("codex");
    });

    it("strictly preserves semantic emerald for EXITED (finished) without brand intrusion", () => {
      const html = renderToStaticMarkup(<StatusBadge lifecycle="EXITED" activity="IDLE" harness="opencode" />);
      expect(html).toContain('data-semantic-status="success"');
      expect(html).toContain("bg-emerald-950/50");
      expect(html).toContain("text-emerald-300");
      expect(html).not.toContain("opencode");
    });
  });

  describe("Unacknowledged completion (#108)", () => {
    it("renders RUNNING + IDLE with hasUnseenCompletion as emerald Completed badge", () => {
      const html = renderToStaticMarkup(<StatusBadge lifecycle="RUNNING" activity="IDLE" hasUnseenCompletion={true} />);
      expect(html).toContain('data-testid="StatusBadge"');
      expect(html).toContain('data-semantic-status="success"');
      expect(html).toContain("bg-emerald-950/60 text-emerald-300 border border-emerald-600/80");
      expect(html).toContain("bg-emerald-400");
      expect(html).toContain('<span data-testid="StatusBadge--label">Completed</span>');
    });

    it("renders RUNNING + IDLE with hasUnseenCompletion=false as standard Idle badge", () => {
      const html = renderToStaticMarkup(
        <StatusBadge lifecycle="RUNNING" activity="IDLE" hasUnseenCompletion={false} />,
      );
      expect(html).toContain('data-semantic-status="neutral"');
      expect(html).toContain('<span data-testid="StatusBadge--label">Idle</span>');
      expect(html).not.toContain("Completed");
    });
  });
});
