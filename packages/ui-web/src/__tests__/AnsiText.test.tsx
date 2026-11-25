import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import { AnsiText } from "../components/AnsiText";

describe("AnsiText Component", () => {
  it("renders plain text without wrapping when no ANSI codes present", () => {
    const html = renderToString(<AnsiText text="Plain output" />);
    expect(html).toBe('<span data-testid="AnsiText">Plain output</span>');
  });

  it("renders 24-bit RGB truecolor escape sequences as styled spans", () => {
    const input = "\u001b[38;2;138;190;183mThinking:\u001b[39m \u001b[38;2;128;128;128mDone\u001b[39m";
    const html = renderToString(<AnsiText text={input} />);

    expect(html).toContain('style="color:rgb(138, 190, 183)"');
    expect(html).toContain("Thinking:");
    expect(html).toContain('style="color:rgb(128, 128, 128)"');
    expect(html).toContain("Done");
    expect(html).not.toContain("38;2;138;190;183m");
  });

  it("renders 16 standard ANSI colors and text styles", () => {
    const input = "\u001b[1;31mBold Red Error\u001b[0m: \u001b[32mSuccess\u001b[0m";
    const html = renderToString(<AnsiText text={input} />);

    expect(html).toContain("color:rgb(239, 68, 68)");
    expect(html).toContain("font-weight:bold");
    expect(html).toContain("Bold Red Error");
    expect(html).toContain("color:rgb(34, 197, 94)");
    expect(html).toContain("Success");
  });

  it("renders bare ANSI codes without \\u001b", () => {
    const input = " [38;2;138;190;183mPrefix: [39m  [38;2;128;128;128mBody [39m";
    const html = renderToString(<AnsiText text={input} />);

    expect(html).toContain("color:rgb(138, 190, 183)");
    expect(html).toContain("Prefix:");
    expect(html).toContain("color:rgb(128, 128, 128)");
    expect(html).toContain("Body");
  });
});
