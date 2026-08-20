import type { HTMLAttributes } from "react";

import { cx } from "./utils";

export interface PanelProps extends HTMLAttributes<HTMLElement> {
  as?: "section" | "article" | "div";
  padding?: "none" | "sm" | "md" | "lg";
}

export function Panel({ as: Element = "section", padding = "none", className, ...props }: PanelProps) {
  return <Element className={cx("pact-panel", className)} data-padding={padding === "none" ? undefined : padding} {...props} />;
}

export function PanelHeader({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <header className={cx("pact-panel-header", className)} {...props} />;
}

export function PanelBody({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("pact-panel-body", className)} {...props} />;
}

export function PanelFooter({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <footer className={cx("pact-panel-footer", className)} {...props} />;
}

export function PanelTitle({ className, ...props }: HTMLAttributes<HTMLHeadingElement>) {
  return <h2 className={cx("pact-panel-title", className)} {...props} />;
}
