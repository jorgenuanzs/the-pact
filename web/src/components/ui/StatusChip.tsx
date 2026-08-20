import type { HTMLAttributes, ReactNode } from "react";

import { cx } from "./utils";

export type StatusTone = "neutral" | "active" | "warning" | "danger" | "info" | "stale";

const symbols: Record<StatusTone, string> = {
  neutral: "○",
  active: "●",
  warning: "△",
  danger: "×",
  info: "?",
  stale: "◷",
};

export interface StatusChipProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: StatusTone;
  icon?: ReactNode;
}
export function StatusChip({
  children,
  tone = "neutral",
  icon,
  className,
  ...props
}: StatusChipProps) {
  return (
    <span className={cx("pact-status-chip", className)} data-tone={tone} {...props}>
      <span className="pact-status-chip-symbol" aria-hidden="true">
        {icon ?? symbols[tone]}
      </span>
      <span>{children}</span>
    </span>
  );
}
