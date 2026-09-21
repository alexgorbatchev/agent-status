import React, { useMemo } from "react";
import type { IAnsiTextProps } from "./types";
import { parseAnsi, stateToCssProperties } from "../utils/ansiUtils";

export function AnsiText({ text, className }: IAnsiTextProps): React.JSX.Element {
  const chunks = useMemo(() => parseAnsi(text), [text]);

  return (
    <span data-testid="AnsiText" className={className}>
      {chunks.map((chunk, idx) => {
        if (chunk.style) {
          return (
            <span key={idx} style={stateToCssProperties(chunk.style)}>
              {chunk.text}
            </span>
          );
        }
        return <React.Fragment key={idx}>{chunk.text}</React.Fragment>;
      })}
    </span>
  );
}
