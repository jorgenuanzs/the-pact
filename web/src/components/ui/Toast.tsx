import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

import { IconButton } from "./Button";
import { Icon } from "./Icon";

export type ToastTone = "default" | "success" | "warning" | "danger";

export interface ToastInput {
  title: string;
  description?: string;
  tone?: ToastTone;
  duration?: number;
}

interface ToastRecord extends ToastInput {
  id: number;
}

interface ToastContextValue {
  toast: (input: ToastInput) => number;
  dismiss: (id: number) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastRecord[]>([]);
  const nextId = useRef(1);
  const timers = useRef(new Map<number, number>());

  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id);
    if (timer) window.clearTimeout(timer);
    timers.current.delete(id);
    setToasts((current) => current.filter((item) => item.id !== id));
  }, []);

  const toast = useCallback((input: ToastInput) => {
    const id = nextId.current++;
    setToasts((current) => [...current, { tone: "default", duration: 5_000, ...input, id }]);
    if (input.duration !== 0) {
      const timer = window.setTimeout(() => dismiss(id), input.duration ?? 5_000);
      timers.current.set(id, timer);
    }
    return id;
  }, [dismiss]);

  useEffect(() => () => {
    timers.current.forEach((timer) => window.clearTimeout(timer));
    timers.current.clear();
  }, []);

  return (
    <ToastContext.Provider value={{ toast, dismiss }}>
      {children}
      {typeof document !== "undefined" && createPortal(
        <div className="pact-toast-viewport" role="region" aria-label="Notificaciones">
          {toasts.map((item) => (
            <div
              key={item.id}
              className="pact-toast"
              data-tone={item.tone}
              role={item.tone === "danger" ? "alert" : "status"}
            >
              <span>
                <span className="pact-toast-title">{item.title}</span>
                {item.description && <span className="pact-toast-description">{item.description}</span>}
              </span>
              <IconButton className="pact-toast-close" size="sm" aria-label="Cerrar notificación" onClick={() => dismiss(item.id)}>
                <Icon name="close" size="sm" />
              </IconButton>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast debe usarse dentro de ToastProvider.");
  return context;
}
