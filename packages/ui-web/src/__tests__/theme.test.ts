import { describe, it, expect } from "bun:test";

describe("Theme Scrollbar CSS", () => {
  it("includes custom scrollbar rules matching dark slate page colors", async () => {
    const themeCss = await Bun.file(`${import.meta.dir}/../theme.css`).text();

    // Standard CSS scrollbar properties
    expect(themeCss).toContain("scrollbar-width: thin;");
    expect(themeCss).toContain("scrollbar-color: #334155 transparent;");

    // WebKit / Chromium scrollbar properties
    expect(themeCss).toContain("::-webkit-scrollbar");
    expect(themeCss).toContain("::-webkit-scrollbar-track");
    expect(themeCss).toContain("::-webkit-scrollbar-thumb");
    expect(themeCss).toContain("::-webkit-scrollbar-corner");
    expect(themeCss).toContain("#334155"); // slate-700
    expect(themeCss).toContain("#475569"); // slate-600 hover
    expect(themeCss).toContain("#64748b"); // slate-500 active
  });

  it("defines dedicated brand tokens and enforces semantic specificity (#106)", async () => {
    const themeCss = await Bun.file(`${import.meta.dir}/../theme.css`).text();

    // Semantic tokens
    expect(themeCss).toContain("--status-success:");
    expect(themeCss).toContain("--status-warning:");
    expect(themeCss).toContain("--status-error:");
    expect(themeCss).toContain("--status-info:");

    // Brand tokens
    expect(themeCss).toContain("--brand-claude:");
    expect(themeCss).toContain("--brand-pi:");
    expect(themeCss).toContain("--brand-codex:");
    expect(themeCss).toContain("--brand-opencode:");
    expect(themeCss).toContain("--brand-default:");
    expect(themeCss).toContain("--brand-muted:");

    // Tailwind theme bindings
    expect(themeCss).toContain("--color-brand-claude: var(--brand-claude);");
    expect(themeCss).toContain("--color-brand-pi: var(--brand-pi);");
    expect(themeCss).toContain("--color-brand-codex: var(--brand-codex);");
    expect(themeCss).toContain("--color-brand-opencode: var(--brand-opencode);");

    // Semantic specificity overrides
    expect(themeCss).toContain('[data-semantic-status="error"]');
    expect(themeCss).toContain('[data-semantic-status="warning"]');
    expect(themeCss).toContain('[data-semantic-status="success"]');
    expect(themeCss).toContain("!important");

    // Light mode WCAG AA overrides
    expect(themeCss).toContain("@media (prefers-color-scheme: light)");
  });
});
