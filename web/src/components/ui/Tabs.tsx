import {
  createContext,
  useContext,
  useId,
  type ButtonHTMLAttributes,
  type HTMLAttributes,
  type KeyboardEvent,
  type ReactNode,
} from "react";

import { cx } from "./utils";

interface TabsContextValue {
  value: string;
  onValueChange: (value: string) => void;
  baseId: string;
}
const TabsContext = createContext<TabsContextValue | null>(null);

function useTabsContext(component: string) {
  const context = useContext(TabsContext);
  if (!context) throw new Error(`${component} debe estar dentro de Tabs.`);
  return context;
}

function safeId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]/g, "-");
}

export interface TabsProps {
  value: string;
  onValueChange: (value: string) => void;
  children: ReactNode;
}

export function Tabs({ value, onValueChange, children }: TabsProps) {
  const baseId = useId();
  return <TabsContext.Provider value={{ value, onValueChange, baseId }}>{children}</TabsContext.Provider>;
}

export function TabsList({ className, onKeyDown, ...props }: HTMLAttributes<HTMLDivElement>) {
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    onKeyDown?.(event);
    if (event.defaultPrevented) return;
    const tabs = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]:not(:disabled)')];
    const index = tabs.indexOf(document.activeElement as HTMLButtonElement);
    if (index < 0 || tabs.length === 0) return;
    let nextIndex: number | null = null;
    if (event.key === "ArrowRight") nextIndex = (index + 1) % tabs.length;
    if (event.key === "ArrowLeft") nextIndex = (index - 1 + tabs.length) % tabs.length;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = tabs.length - 1;
    if (nextIndex !== null) {
      event.preventDefault();
      tabs[nextIndex].focus();
      tabs[nextIndex].click();
    }
  };

  return <div role="tablist" className={cx("pact-tabs-list", className)} onKeyDown={handleKeyDown} {...props} />;
}

export interface TabsTriggerProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  value: string;
}

export function TabsTrigger({ value, className, onClick, type = "button", ...props }: TabsTriggerProps) {
  const context = useTabsContext("TabsTrigger");
  const selected = context.value === value;
  const suffix = safeId(value);
  return (
    <button
      type={type}
      role="tab"
      id={`${context.baseId}-tab-${suffix}`}
      aria-controls={`${context.baseId}-panel-${suffix}`}
      aria-selected={selected}
      tabIndex={selected ? 0 : -1}
      className={cx("pact-tab", className)}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented) context.onValueChange(value);
      }}
      {...props}
    />
  );
}

export interface TabsContentProps extends HTMLAttributes<HTMLDivElement> {
  value: string;
  forceMount?: boolean;
}

export function TabsContent({ value, forceMount = false, className, ...props }: TabsContentProps) {
  const context = useTabsContext("TabsContent");
  const selected = context.value === value;
  if (!selected && !forceMount) return null;
  const suffix = safeId(value);
  return (
    <div
      role="tabpanel"
      id={`${context.baseId}-panel-${suffix}`}
      aria-labelledby={`${context.baseId}-tab-${suffix}`}
      hidden={!selected}
      tabIndex={0}
      className={cx("pact-tab-panel", className)}
      {...props}
    />
  );
}
