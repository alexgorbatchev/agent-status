import React from "react";
import clsx from "clsx";
import type { IViewSwitcherProps } from "./types";
import { IconWrapper } from "./IconWrapper";

export function ViewSwitcher<T extends string = string>({
  options,
  value,
  onChange,
}: IViewSwitcherProps<T>): React.JSX.Element {
  return (
    <div data-testid="ViewSwitcher" className="flex items-center gap-2">
      {options.map((option) => {
        const isSelected = option.id === value;
        return (
          <button
            key={option.id}
            type="button"
            data-testid={option.testId || `ViewSwitcher--${option.id}`}
            onClick={() => onChange(option.id)}
            className={clsx(
              "flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors",
              isSelected
                ? "bg-blue-600 text-white shadow-md shadow-blue-600/30"
                : "bg-slate-900 text-slate-400 hover:text-slate-200 border border-slate-800",
            )}
          >
            {option.icon ? <IconWrapper size="sm">{option.icon}</IconWrapper> : null}
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
