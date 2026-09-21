import type React from "react";

export interface IAnsiStyle {
  color?: string;
  backgroundColor?: string;
  fontWeight?: "bold" | "normal";
  fontStyle?: "italic" | "normal";
  textDecoration?: "underline" | "none";
  opacity?: number;
}

export interface IAnsiChunk {
  text: string;
  style?: IAnsiStyle;
}

const ESC = String.fromCharCode(27);

export function createAnsiCsiRegex(): RegExp {
  return new RegExp(`(?:${ESC}\\[|\\[)([0-9;]+)m`, "g");
}

export function createNonSgrAnsiRegex(): RegExp {
  return new RegExp(`${ESC}\\[[0-9;]*[A-LN-Za-ln-z]`, "g");
}

const STANDARD_FG: Record<number, string> = {
  30: "rgb(75, 85, 99)",
  31: "rgb(239, 68, 68)",
  32: "rgb(34, 197, 94)",
  33: "rgb(234, 179, 8)",
  34: "rgb(59, 130, 246)",
  35: "rgb(168, 85, 247)",
  36: "rgb(6, 182, 212)",
  37: "rgb(226, 232, 240)",
};

const BRIGHT_FG: Record<number, string> = {
  90: "rgb(148, 163, 184)",
  91: "rgb(248, 113, 113)",
  92: "rgb(74, 222, 128)",
  93: "rgb(253, 224, 71)",
  94: "rgb(96, 165, 250)",
  95: "rgb(192, 132, 252)",
  96: "rgb(34, 211, 238)",
  97: "rgb(255, 255, 255)",
};

const STANDARD_BG: Record<number, string> = {
  40: "rgb(15, 23, 42)",
  41: "rgb(127, 29, 29)",
  42: "rgb(20, 83, 45)",
  43: "rgb(113, 63, 18)",
  44: "rgb(30, 58, 138)",
  45: "rgb(88, 28, 135)",
  46: "rgb(22, 78, 99)",
  47: "rgb(100, 116, 139)",
};

const BRIGHT_BG: Record<number, string> = {
  100: "rgb(51, 65, 85)",
  101: "rgb(153, 27, 27)",
  102: "rgb(22, 101, 52)",
  103: "rgb(133, 77, 14)",
  104: "rgb(30, 64, 175)",
  105: "rgb(107, 33, 168)",
  106: "rgb(14, 116, 144)",
  107: "rgb(148, 163, 184)",
};

export function get256Color(n: number): string {
  if (n >= 0 && n <= 7) {
    return STANDARD_FG[n + 30] ?? "rgb(226, 232, 240)";
  }
  if (n >= 8 && n <= 15) {
    return BRIGHT_FG[n - 8 + 90] ?? "rgb(255, 255, 255)";
  }
  if (n >= 16 && n <= 231) {
    const index = n - 16;
    const r = Math.floor(index / 36) * 51;
    const g = Math.floor((index % 36) / 6) * 51;
    const b = (index % 6) * 51;
    return `rgb(${r}, ${g}, ${b})`;
  }
  if (n >= 232 && n <= 255) {
    const gray = (n - 232) * 10 + 8;
    return `rgb(${gray}, ${gray}, ${gray})`;
  }
  return "rgb(226, 232, 240)";
}

export function parseAnsiCodes(codesStr: string, state: IAnsiStyle): void {
  if (!codesStr || codesStr === "0") {
    state.color = undefined;
    state.backgroundColor = undefined;
    state.fontWeight = undefined;
    state.fontStyle = undefined;
    state.textDecoration = undefined;
    state.opacity = undefined;
    return;
  }

  const parts = codesStr.split(";").map((p) => Number.parseInt(p, 10));
  let i = 0;
  while (i < parts.length) {
    const code = parts[i];
    if (code === 0) {
      state.color = undefined;
      state.backgroundColor = undefined;
      state.fontWeight = undefined;
      state.fontStyle = undefined;
      state.textDecoration = undefined;
      state.opacity = undefined;
    } else if (code === 1) {
      state.fontWeight = "bold";
    } else if (code === 2) {
      state.opacity = 0.7;
    } else if (code === 3) {
      state.fontStyle = "italic";
    } else if (code === 4) {
      state.textDecoration = "underline";
    } else if (code === 22) {
      state.fontWeight = undefined;
      state.opacity = undefined;
    } else if (code === 23) {
      state.fontStyle = undefined;
    } else if (code === 24) {
      state.textDecoration = undefined;
    } else if (code >= 30 && code <= 37) {
      state.color = STANDARD_FG[code];
    } else if (code === 39) {
      state.color = undefined;
    } else if (code >= 40 && code <= 47) {
      state.backgroundColor = STANDARD_BG[code];
    } else if (code === 49) {
      state.backgroundColor = undefined;
    } else if (code >= 90 && code <= 97) {
      state.color = BRIGHT_FG[code];
    } else if (code >= 100 && code <= 107) {
      state.backgroundColor = BRIGHT_BG[code];
    } else if (code === 38 && parts[i + 1] === 2 && i + 4 < parts.length) {
      state.color = `rgb(${parts[i + 2]}, ${parts[i + 3]}, ${parts[i + 4]})`;
      i += 4;
    } else if (code === 48 && parts[i + 1] === 2 && i + 4 < parts.length) {
      state.backgroundColor = `rgb(${parts[i + 2]}, ${parts[i + 3]}, ${parts[i + 4]})`;
      i += 4;
    } else if (code === 38 && parts[i + 1] === 5 && i + 2 < parts.length) {
      state.color = get256Color(parts[i + 2]);
      i += 2;
    } else if (code === 48 && parts[i + 1] === 5 && i + 2 < parts.length) {
      state.backgroundColor = get256Color(parts[i + 2]);
      i += 2;
    }
    i++;
  }
}

export function hasActiveStyle(state: IAnsiStyle): boolean {
  return Boolean(
    state.color ||
    state.backgroundColor ||
    state.fontWeight ||
    state.fontStyle ||
    state.textDecoration ||
    state.opacity !== undefined,
  );
}

export function stateToStyleString(state: IAnsiStyle): string {
  const parts: string[] = [];
  if (state.color) {
    parts.push(`color: ${state.color}`);
  }
  if (state.backgroundColor) {
    parts.push(`background-color: ${state.backgroundColor}`);
  }
  if (state.fontWeight) {
    parts.push(`font-weight: ${state.fontWeight}`);
  }
  if (state.fontStyle) {
    parts.push(`font-style: ${state.fontStyle}`);
  }
  if (state.textDecoration) {
    parts.push(`text-decoration: ${state.textDecoration}`);
  }
  if (state.opacity !== undefined) {
    parts.push(`opacity: ${state.opacity}`);
  }
  return parts.join("; ");
}

export function stateToCssProperties(state: IAnsiStyle): React.CSSProperties {
  const props: React.CSSProperties = {};
  if (state.color) {
    props.color = state.color;
  }
  if (state.backgroundColor) {
    props.backgroundColor = state.backgroundColor;
  }
  if (state.fontWeight) {
    props.fontWeight = state.fontWeight;
  }
  if (state.fontStyle) {
    props.fontStyle = state.fontStyle;
  }
  if (state.textDecoration) {
    props.textDecoration = state.textDecoration;
  }
  if (state.opacity !== undefined) {
    props.opacity = state.opacity;
  }
  return props;
}

export function parseAnsi(text: string): IAnsiChunk[] {
  const nonSgrRegex = createNonSgrAnsiRegex();
  const ansiCsiRegex = createAnsiCsiRegex();
  const cleanedText = text.replace(nonSgrRegex, "");
  const state: IAnsiStyle = {};
  const chunks: IAnsiChunk[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null = ansiCsiRegex.exec(cleanedText);

  while (match !== null) {
    const rawText = cleanedText.slice(lastIndex, match.index);
    if (rawText) {
      if (hasActiveStyle(state)) {
        chunks.push({ text: rawText, style: { ...state } });
      } else {
        chunks.push({ text: rawText });
      }
    }
    parseAnsiCodes(match[1], state);
    lastIndex = ansiCsiRegex.lastIndex;
    match = ansiCsiRegex.exec(cleanedText);
  }

  const remainingText = cleanedText.slice(lastIndex);
  if (remainingText) {
    if (hasActiveStyle(state)) {
      chunks.push({ text: remainingText, style: { ...state } });
    } else {
      chunks.push({ text: remainingText });
    }
  }

  return chunks;
}

export function stripAnsi(text: string): string {
  return text.replace(createNonSgrAnsiRegex(), "").replace(createAnsiCsiRegex(), "");
}

export function preprocessMarkdownAnsi(text: string): string {
  const blockRegex = new RegExp(
    `^(\\s*)((?:(?:${ESC}\\[|\\[)[0-9;]*m)+)(\\s*)(#{1,6}\\s|[-*+]\\s|\\d+\\.\\s|>\\s|\`\`\`)`,
  );
  return text
    .split("\n")
    .map((line) => {
      const match = blockRegex.exec(line);
      if (match) {
        const indent = match[1];
        const ansiCodes = match[2];
        const blockSyntax = match[4];
        const rest = line.slice(match[0].length);
        if (blockSyntax.startsWith("```")) {
          return `${indent}${ansiCodes}\n${indent}${blockSyntax}${rest}`;
        }
        return `${indent}${blockSyntax}${ansiCodes}${rest}`;
      }
      return line;
    })
    .join("\n");
}
