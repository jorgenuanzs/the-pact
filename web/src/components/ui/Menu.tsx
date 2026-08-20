import {
  createContext,
  forwardRef,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type HTMLAttributes,
  type KeyboardEvent,
  type ReactNode,
  type Ref,
} from "react";

import { cx } from "./utils";

interface MenuContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  contentId: string;
}

const MenuContext = createContext<MenuContextValue | null>(null);

function useMenuContext(component: string) {
  const context = useContext(MenuContext);
  if (!context) throw new Error(`${component} debe estar dentro de Menu.`);
  return context;
}

function setRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === "function") ref(value);
  else if (ref) ref.current = value;
}

export interface MenuProps {
  children: ReactNode;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
}

export function Menu({ children, open: controlledOpen, defaultOpen = false, onOpenChange, className }: MenuProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const contentId = useId();
  const open = controlledOpen ?? uncontrolledOpen;

  const setOpen = useCallback((nextOpen: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(nextOpen);
    onOpenChange?.(nextOpen);
  }, [controlledOpen, onOpenChange]);

  useEffect(() => {
    if (!open) return;

    const handlePointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const handleKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setOpen(false);
      triggerRef.current?.focus();
    };

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open, setOpen]);

  return (
    <MenuContext.Provider value={{ open, setOpen, triggerRef, contentId }}>
      <div ref={rootRef} className={cx("pact-menu-root", className)}>{children}</div>
    </MenuContext.Provider>
  );
}

export const MenuTrigger = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement>>(
  function MenuTrigger({ children, onClick, onKeyDown, className, type = "button", ...props }, forwardedRef) {
    const { open, setOpen, triggerRef, contentId } = useMenuContext("MenuTrigger");
    return (
      <button
        ref={(node) => {
          triggerRef.current = node;
          setRef(forwardedRef, node);
        }}
        type={type}
        className={className}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={contentId}
        data-state={open ? "open" : "closed"}
        onClick={(event) => {
          onClick?.(event);
          if (!event.defaultPrevented) setOpen(!open);
        }}
        onKeyDown={(event) => {
          onKeyDown?.(event);
          if (event.defaultPrevented) return;
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setOpen(true);
          }
        }}
        {...props}
      >
        {children}
      </button>
    );
  },
);

export interface MenuContentProps extends HTMLAttributes<HTMLDivElement> {
  side?: "top" | "bottom";
  align?: "start" | "end";
}

export const MenuContent = forwardRef<HTMLDivElement, MenuContentProps>(
  function MenuContent({ children, className, side = "top", align = "start", onKeyDown, ...props }, ref) {
    const { open, setOpen, contentId } = useMenuContext("MenuContent");
    const localRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
      if (!open) return;
      queueMicrotask(() => {
        localRef.current
          ?.querySelector<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')
          ?.focus();
      });
    }, [open]);

    if (!open) return null;

    const handleNavigation = (event: KeyboardEvent<HTMLDivElement>) => {
      onKeyDown?.(event);
      if (event.defaultPrevented) return;
      const items = [...(localRef.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)') ?? [])];
      if (items.length === 0) return;
      const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement);
      let nextIndex: number | null = null;
      if (event.key === "ArrowDown") nextIndex = (currentIndex + 1) % items.length;
      if (event.key === "ArrowUp") nextIndex = (currentIndex - 1 + items.length) % items.length;
      if (event.key === "Home") nextIndex = 0;
      if (event.key === "End") nextIndex = items.length - 1;
      if (event.key === "Tab") setOpen(false);
      if (nextIndex !== null) {
        event.preventDefault();
        items[nextIndex].focus();
      }
    };

    return (
      <div
        ref={(node) => {
          localRef.current = node;
          setRef(ref, node);
        }}
        id={contentId}
        role="menu"
        className={cx("pact-menu-content", className)}
        data-side={side}
        data-align={align}
        onKeyDown={handleNavigation}
        {...props}
      >
        {children}
      </div>
    );
  },
);

export interface MenuItemProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onSelect"> {
  onSelect?: () => void;
  tone?: "default" | "danger";
}

export const MenuItem = forwardRef<HTMLButtonElement, MenuItemProps>(
  function MenuItem({ children, className, onClick, onSelect, tone = "default", type = "button", ...props }, ref) {
    const { setOpen } = useMenuContext("MenuItem");
    return (
      <button
        ref={ref}
        type={type}
        role="menuitem"
        tabIndex={-1}
        className={cx("pact-menu-item", className)}
        data-tone={tone}
        onClick={(event) => {
          onClick?.(event);
          if (!event.defaultPrevented) {
            onSelect?.();
            setOpen(false);
          }
        }}
        {...props}
      >
        {children}
      </button>
    );
  },
);

export function MenuLabel({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("pact-menu-label", className)} {...props} />;
}

export function MenuSeparator({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div role="separator" className={cx("pact-menu-separator", className)} {...props} />;
}
