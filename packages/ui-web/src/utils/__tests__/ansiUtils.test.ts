import { describe, it, expect } from "bun:test";
import {
  parseAnsi,
  stripAnsi,
  preprocessMarkdownAnsi,
  parseAnsiCodes,
  stateToStyleString,
  stateToCssProperties,
  type IAnsiStyle,
} from "../ansiUtils";

describe("ansiUtils", () => {
  describe("parseAnsiCodes", () => {
    it("handles reset codes (0, empty)", () => {
      const state: IAnsiStyle = {
        color: "rgb(255, 0, 0)",
        backgroundColor: "rgb(0, 255, 0)",
        fontWeight: "bold",
        fontStyle: "italic",
        textDecoration: "underline",
        opacity: 0.7,
      };

      parseAnsiCodes("0", state);
      expect(state.color).toBeUndefined();
      expect(state.backgroundColor).toBeUndefined();
      expect(state.fontWeight).toBeUndefined();
      expect(state.fontStyle).toBeUndefined();
      expect(state.textDecoration).toBeUndefined();
      expect(state.opacity).toBeUndefined();
    });

    it("handles standard 16 foreground and background colors", () => {
      const state: IAnsiStyle = {};

      // Standard red foreground
      parseAnsiCodes("31", state);
      expect(state.color).toBe("rgb(239, 68, 68)");

      // Standard green background
      parseAnsiCodes("42", state);
      expect(state.backgroundColor).toBe("rgb(20, 83, 45)");

      // Reset foreground
      parseAnsiCodes("39", state);
      expect(state.color).toBeUndefined();
      expect(state.backgroundColor).toBe("rgb(20, 83, 45)");

      // Reset background
      parseAnsiCodes("49", state);
      expect(state.backgroundColor).toBeUndefined();
    });

    it("handles bright foreground and background colors", () => {
      const state: IAnsiStyle = {};

      // Bright yellow foreground
      parseAnsiCodes("93", state);
      expect(state.color).toBe("rgb(253, 224, 71)");

      // Bright blue background
      parseAnsiCodes("104", state);
      expect(state.backgroundColor).toBe("rgb(30, 64, 175)");
    });

    it("handles 24-bit truecolor RGB foreground (38;2;r;g;b)", () => {
      const state: IAnsiStyle = {};

      // Thinking prefix teal: 138, 190, 183
      parseAnsiCodes("38;2;138;190;183", state);
      expect(state.color).toBe("rgb(138, 190, 183)");

      // Thought grey: 128, 128, 128
      parseAnsiCodes("38;2;128;128;128", state);
      expect(state.color).toBe("rgb(128, 128, 128)");
    });

    it("handles 24-bit truecolor RGB background (48;2;r;g;b)", () => {
      const state: IAnsiStyle = {};

      parseAnsiCodes("48;2;40;50;60", state);
      expect(state.backgroundColor).toBe("rgb(40, 50, 60)");
    });

    it("handles 256-color palette (38;5;n and 48;5;n)", () => {
      const state: IAnsiStyle = {};

      // Standard color 1 (red)
      parseAnsiCodes("38;5;1", state);
      expect(state.color).toBe("rgb(239, 68, 68)");

      // Bright color 9 (bright red)
      parseAnsiCodes("38;5;9", state);
      expect(state.color).toBe("rgb(248, 113, 113)");

      // 6x6x6 color cube
      parseAnsiCodes("38;5;196", state);
      expect(state.color).toBe("rgb(255, 0, 0)");

      // Grayscale ramp
      parseAnsiCodes("48;5;240", state);
      expect(state.backgroundColor).toBe("rgb(88, 88, 88)");
    });

    it("handles text styles (bold, dim, italic, underline and resets)", () => {
      const state: IAnsiStyle = {};

      parseAnsiCodes("1;3;4", state);
      expect(state.fontWeight).toBe("bold");
      expect(state.fontStyle).toBe("italic");
      expect(state.textDecoration).toBe("underline");

      // Dim
      parseAnsiCodes("2", state);
      expect(state.opacity).toBe(0.7);

      // Reset bold/dim
      parseAnsiCodes("22", state);
      expect(state.fontWeight).toBeUndefined();
      expect(state.opacity).toBeUndefined();

      // Reset italic
      parseAnsiCodes("23", state);
      expect(state.fontStyle).toBeUndefined();

      // Reset underline
      parseAnsiCodes("24", state);
      expect(state.textDecoration).toBeUndefined();
    });
  });

  describe("stateToStyleString and stateToCssProperties", () => {
    it("converts state to CSS style string", () => {
      const state: IAnsiStyle = {
        color: "rgb(138, 190, 183)",
        fontWeight: "bold",
      };
      const styleString = stateToStyleString(state);
      expect(styleString).toContain("color: rgb(138, 190, 183)");
      expect(styleString).toContain("font-weight: bold");
    });

    it("converts state to React CSSProperties", () => {
      const state: IAnsiStyle = {
        color: "rgb(128, 128, 128)",
        backgroundColor: "rgb(0, 0, 0)",
        opacity: 0.7,
      };
      const cssProps = stateToCssProperties(state);
      expect(cssProps.color).toBe("rgb(128, 128, 128)");
      expect(cssProps.backgroundColor).toBe("rgb(0, 0, 0)");
      expect(cssProps.opacity).toBe(0.7);
    });
  });

  describe("parseAnsi", () => {
    it("returns single unstyled chunk for plain text", () => {
      const chunks = parseAnsi("Hello world");
      expect(chunks).toHaveLength(1);
      expect(chunks[0].text).toBe("Hello world");
      expect(chunks[0].style).toBeUndefined();
    });

    it("parses standard escape sequences with \\u001b[", () => {
      const input = "\u001b[38;2;138;190;183mThinking:\u001b[39m \u001b[38;2;128;128;128mPondering\u001b[39m";
      const chunks = parseAnsi(input);

      expect(chunks.length).toBeGreaterThanOrEqual(3);
      expect(chunks[0].text).toBe("Thinking:");
      expect(chunks[0].style?.color).toBe("rgb(138, 190, 183)");

      const ponderingChunk = chunks.find((c) => c.text === "Pondering");
      expect(ponderingChunk).toBeDefined();
      expect(ponderingChunk?.style?.color).toBe("rgb(128, 128, 128)");
    });

    it("parses bare escape sequences without escape character (e.g. [38;2;138;190;183m)", () => {
      const input = " [38;2;138;190;183mThinking: [39m  [38;2;128;128;128mEvaluating [39m";
      const chunks = parseAnsi(input);

      const thinkingChunk = chunks.find((c) => c.text.includes("Thinking:"));
      expect(thinkingChunk).toBeDefined();
      expect(thinkingChunk?.style?.color).toBe("rgb(138, 190, 183)");

      const evalChunk = chunks.find((c) => c.text.includes("Evaluating"));
      expect(evalChunk).toBeDefined();
      expect(evalChunk?.style?.color).toBe("rgb(128, 128, 128)");
    });

    it("parses bare reset codes immediately following characters without whitespace", () => {
      const input = "[31mFAIL[0m: test error[39m";
      const chunks = parseAnsi(input);

      expect(chunks[0].text).toBe("FAIL");
      expect(chunks[0].style?.color).toBe("rgb(239, 68, 68)");

      expect(chunks[1].text).toBe(": test error");
      expect(chunks[1].style).toBeUndefined();
    });
  });

  describe("stripAnsi", () => {
    it("strips escape sequences", () => {
      const input = "\u001b[38;2;138;190;183mThinking:\u001b[39m \u001b[38;2;128;128;128mBody\u001b[39m";
      expect(stripAnsi(input)).toBe("Thinking: Body");
    });

    it("strips bare escape sequences without \\u001b", () => {
      const input = " [38;2;138;190;183mThinking: [39m  [38;2;128;128;128mBody [39m";
      expect(stripAnsi(input).replace(/\s+/g, " ").trim()).toBe("Thinking: Body");
    });
  });

  describe("preprocessMarkdownAnsi", () => {
    it("moves markdown heading syntax before leading ANSI sequence", () => {
      const input = "\u001b[38;2;128;128;128m# Heading 1\n\nParagraph\u001b[39m";
      const processed = preprocessMarkdownAnsi(input);
      expect(processed.startsWith("# ")).toBe(true);
      expect(processed).toContain("\u001b[38;2;128;128;128mHeading 1");
    });

    it("moves markdown list syntax before leading ANSI sequence", () => {
      const input = "\u001b[38;2;128;128;128m- Item 1\n- Item 2\u001b[39m";
      const processed = preprocessMarkdownAnsi(input);
      expect(processed.startsWith("- ")).toBe(true);
    });

    it("moves markdown blockquote syntax before leading ANSI sequence", () => {
      const input = "\u001b[38;2;128;128;128m> Quote\u001b[39m";
      const processed = preprocessMarkdownAnsi(input);
      expect(processed.startsWith("> ")).toBe(true);
    });

    it("preserves multiple preceding ANSI codes before heading", () => {
      const input = "\u001b[1m\u001b[31m# Bold Red Heading\u001b[0m";
      const processed = preprocessMarkdownAnsi(input);
      expect(processed.startsWith("# ")).toBe(true);
      expect(processed).toContain("\u001b[1m\u001b[31mBold Red Heading");
    });

    it("handles code fences without corrupting language identifier", () => {
      const input = "\u001b[32m```typescript\nconst x = 1;\n```\u001b[39m";
      const processed = preprocessMarkdownAnsi(input);
      expect(processed).toContain("```typescript");
      expect(processed).not.toContain("```\u001b[32mtypescript");
    });

    it("handles indented markdown and bare ANSI code with leading space", () => {
      const input = "  \u001b[32m- Indented item\n [38;2;128;128;128m## Subheading";
      const processed = preprocessMarkdownAnsi(input);
      expect(processed).toContain("  - \u001b[32mIndented item");
      expect(processed).toContain(" ## [38;2;128;128;128mSubheading");
    });
  });
});
