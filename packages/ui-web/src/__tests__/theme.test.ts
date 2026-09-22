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
});
