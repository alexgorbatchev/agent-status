import { describe, it, expect } from "bun:test";
import React from "react";
import { renderToString } from "react-dom/server";
import Markdown from "react-markdown";
import { rehypeAnsi } from "../rehypeAnsi";
import { preprocessMarkdownAnsi } from "../ansiUtils";

describe("rehypeAnsi Plugin", () => {
  it("renders thinking frame with ANSI teal 'Thinking:' and grey thought content with markdown intact", () => {
    const rawThinking = `\u001b[38;2;138;190;183mThinking:\u001b[39m  \u001b[38;2;128;128;128mOkay, here's my interpretation of that simple instruction, framed as my own thought process:

**Running the Checks**

Alright, that's straightforward enough. "Now run just check!" It's a command, likely within a build system. \u001b[39m`;

    const processed = preprocessMarkdownAnsi(rawThinking);
    const html = renderToString(<Markdown rehypePlugins={[rehypeAnsi]}>{processed}</Markdown>);

    // Thinking prefix rendered with color 138, 190, 183
    expect(html).toContain('style="color:rgb(138, 190, 183)"');
    expect(html).toContain("Thinking:");

    // Thought body rendered with color 128, 128, 128
    expect(html).toContain('style="color:rgb(128, 128, 128)"');
    expect(html).toContain("Okay, here&#x27;s my interpretation");

    // Markdown strong tag rendered
    expect(html).toContain("<strong>");
    expect(html).toContain("Running the Checks");

    // Paragraph tags rendered
    expect(html).toContain("<p>");

    // Raw ANSI codes must not appear in output
    expect(html).not.toContain("38;2;138;190;183m");
    expect(html).not.toContain("38;2;128;128;128m");
    expect(html).not.toContain("[39m");
  });

  it("handles bare ANSI codes without \\u001b escape byte", () => {
    const rawThinking = ` [38;2;138;190;183mThinking: [39m  [38;2;128;128;128mExamining dependencies:\n\n- Package A\n- Package B [39m`;

    const processed = preprocessMarkdownAnsi(rawThinking);
    const html = renderToString(<Markdown rehypePlugins={[rehypeAnsi]}>{processed}</Markdown>);

    expect(html).toContain('style="color:rgb(138, 190, 183)"');
    expect(html).toContain("Thinking:");
    expect(html).toContain('style="color:rgb(128, 128, 128)"');
    expect(html).toContain("Examining dependencies:");
    expect(html).toContain("<ul>");
    expect(html).toContain("<li>");
    expect(html).toContain("Package A");

    expect(html).not.toContain("38;2;138;190;183m");
    expect(html).not.toContain("[39m");
  });

  it("handles standard 16 ANSI colors inside markdown", () => {
    const input = "Here is \u001b[31mred text\u001b[39m and \u001b[32mgreen text\u001b[39m.";
    const html = renderToString(<Markdown rehypePlugins={[rehypeAnsi]}>{input}</Markdown>);

    expect(html).toContain('style="color:rgb(239, 68, 68)"');
    expect(html).toContain("red text");
    expect(html).toContain('style="color:rgb(34, 197, 94)"');
    expect(html).toContain("green text");
  });

  it("renders plain markdown unchanged when no ANSI codes exist", () => {
    const input = "# Heading\n\nSimple text";
    const html = renderToString(<Markdown rehypePlugins={[rehypeAnsi]}>{input}</Markdown>);

    expect(html).toContain("<h1>Heading</h1>");
    expect(html).toContain("<p>Simple text</p>");
  });

  it("handles bare reset immediately attached to punctuation without spaces", () => {
    const input = "[38;2;138;190;183mThinking:[39m normal text";
    const html = renderToString(<Markdown rehypePlugins={[rehypeAnsi]}>{input}</Markdown>);

    expect(html).toContain('style="color:rgb(138, 190, 183)"');
    expect(html).toContain("Thinking:");
    expect(html).toContain("normal text");
    expect(html).not.toContain("[39m");
  });

  it("preserves styled whitespace when background color or text decoration is active", () => {
    const input = "before\u001b[41m   \u001b[49mafter";
    const html = renderToString(<Markdown rehypePlugins={[rehypeAnsi]}>{input}</Markdown>);

    expect(html).toContain('style="background-color:rgb(127, 29, 29)"');
    expect(html).toContain("   ");
  });

  it("renders fenced code block with language identifier properly", () => {
    const input = "\u001b[32m```typescript\nconst a = 1;\n```\u001b[39m";
    const processed = preprocessMarkdownAnsi(input);
    const html = renderToString(<Markdown rehypePlugins={[rehypeAnsi]}>{processed}</Markdown>);

    expect(html).toContain('<code class="language-typescript">');
    expect(html).toContain("const a = 1;");
  });
});
