import React from "react";
import clsx from "clsx";
import type { IIconWrapperProps, IconSize, IconTone, IconAlign } from "./types";

const SIZE_CLASSES: Record<IconSize, string> = {
  xs: "w-3 h-3",
  sm: "w-3.5 h-3.5",
  md: "w-4 h-4",
  lg: "w-5 h-5",
  xl: "w-6 h-6",
};

const TONE_CLASSES: Record<IconTone, string> = {
  default: "",
  muted: "text-slate-400",
  primary: "text-blue-400",
  warning: "text-amber-400",
  error: "text-red-400",
  success: "text-emerald-400",
  purple: "text-purple-400",
  subtle: "text-slate-500",
};

const ALIGN_CLASSES: Record<IconAlign, string> = {
  center: "",
  top: "mt-0.5",
};

export function IconWrapper({
  children,
  size = "md",
  tone = "default",
  align = "center",
  spin = false,
}: IIconWrapperProps): React.JSX.Element {
  return (
    <span
      data-testid="IconWrapper"
      className={clsx(
        "flex items-center justify-center shrink-0",
        SIZE_CLASSES[size],
        TONE_CLASSES[tone],
        ALIGN_CLASSES[align],
        spin && "animate-spin",
      )}
    >
      {children}
    </span>
  );
}
