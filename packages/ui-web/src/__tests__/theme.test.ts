import { describe, it, expect } from "bun:test";
import { contrastRatio } from "./helpers";

function extractCssTokens(css: string): Record<string, string> {
  const tokens: Record<string, string> = {};
  const tokenRegex = /--(status-[a-z]+|brand-[a-z]+):\s*(#[0-9a-fA-F]{6})/g;
  let match: RegExpExecArray | null = null;
  while ((match = tokenRegex.exec(css)) !== null) {
    tokens[match[1]] = match[2];
  }
  return tokens;
}

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

    // Comment accurately describes dark card surfaces and does not claim light backgrounds (#106)
    expect(themeCss).toContain("dark card surfaces (#020617, #0f172a, #1e293b)");
    expect(themeCss).not.toContain("light (#ffffff, #f8fafc)");

    // No conflicting data-semantic-status or .status- border rules that collide with Tailwind card borders (#106)
    expect(themeCss).not.toContain("[data-semantic-status=");
    expect(themeCss).not.toMatch(/\.status-(error|warning|success)\b/);

    // No light mode media query swap that degrades contrast on dark surfaces (#106)
    expect(themeCss).not.toContain("@media (prefers-color-scheme: light)");
  });

  it("satisfies WCAG AA contrast ratio (>= 4.5:1) for all brand and status tokens on dark surfaces (#106)", async () => {
    const themeCss = await Bun.file(`${import.meta.dir}/../theme.css`).text();
    const tokens = extractCssTokens(themeCss);

    // Verify all expected tokens are defined
    const requiredTokens = [
      "status-success",
      "status-warning",
      "status-error",
      "status-info",
      "brand-claude",
      "brand-pi",
      "brand-codex",
      "brand-opencode",
      "brand-default",
      "brand-muted",
    ];
    for (const name of requiredTokens) {
      expect(tokens[name]).toBeDefined();
    }
    expect(tokens["brand-claude"]).toBe("#db8659");

    // Actual dark card surfaces in the application
    const darkSurfaces = ["#020617", "#0f172a", "#1e293b"];

    for (const surface of darkSurfaces) {
      for (const name of requiredTokens) {
        const hex = tokens[name];
        const ratio = contrastRatio(hex, surface);
        expect(ratio).toBeGreaterThanOrEqual(4.5);
      }
    }
  });

  it("does not override Tailwind card borders with global semantic status selectors (#106)", async () => {
    const themeCss = await Bun.file(`${import.meta.dir}/../theme.css`).text();
    // theme.css should not use [data-semantic-status] or .status-* border overrides
    expect(themeCss).not.toContain("[data-semantic-status=");
    expect(themeCss).not.toMatch(/\[data-semantic-status/);
  });
});
