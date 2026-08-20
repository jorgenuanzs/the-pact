import type { HTMLAttributes } from "react";

import { cx, initials } from "./utils";

export type AvatarKind = "person" | "agent";
export type AvatarSize = "sm" | "md" | "lg";
export type AvatarColor =
  | "neutral"
  | "lime"
  | "cyan"
  | "cobalt"
  | "orange"
  | "terracotta"
  | "mauve";

export interface AvatarProps extends HTMLAttributes<HTMLSpanElement> {
  name: string;
  kind?: AvatarKind;
  size?: AvatarSize;
  color?: AvatarColor;
  decorative?: boolean;
}
export function Avatar({
  name,
  kind = "person",
  size = "md",
  color,
  decorative = true,
  className,
  ...props
}: AvatarProps) {
  return (
    <span
      className={cx("pact-avatar", className)}
      data-kind={kind}
      data-size={size}
      data-color={color}
      aria-hidden={decorative || undefined}
      aria-label={decorative ? undefined : `${kind === "agent" ? "Agente" : "Persona"}: ${name}`}
      {...props}
    >
      {initials(name)}
    </span>
  );
}
