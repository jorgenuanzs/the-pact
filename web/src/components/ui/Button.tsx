import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";

import { cx } from "./utils";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ControlSize = "sm" | "md" | "lg";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ControlSize;
  fullWidth?: boolean;
  loading?: boolean;
  loadingLabel?: string;
  leadingIcon?: ReactNode;
  trailingIcon?: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    children,
    className,
    variant = "primary",
    size = "md",
    fullWidth = false,
    loading = false,
    loadingLabel = "Procesando",
    leadingIcon,
    trailingIcon,
    disabled,
    type = "button",
    ...props
  },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={cx("pact-button", className)}
      data-variant={variant}
      data-size={size}
      data-full-width={fullWidth || undefined}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...props}
    >
      {loading ? <span className="pact-button-spinner" aria-hidden="true" /> : leadingIcon}
      <span>{loading ? loadingLabel : children}</span>
      {!loading && trailingIcon}
    </button>
  );
});

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  "aria-label": string;
  size?: ControlSize;
  children: ReactNode;
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(
  function IconButton({ className, size = "md", type = "button", ...props }, ref) {
    return (
      <button
        ref={ref}
        type={type}
        className={cx("pact-icon-button", className)}
        data-size={size}
        {...props}
      />
    );
  },
);
