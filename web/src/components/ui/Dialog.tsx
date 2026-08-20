import {
  cloneElement,
  createContext,
  forwardRef,
  isValidElement,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type ComponentPropsWithoutRef,
  type HTMLAttributes,
  type MouseEvent,
  type ReactElement,
  type ReactNode,
  type Ref,
} from "react";

import { IconButton } from "./Button";
import { Icon } from "./Icon";
import { cx } from "./utils";

interface DialogContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
  dialogRef: React.RefObject<HTMLDialogElement | null>;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  titleId: string;
  descriptionId: string;
}

const DialogContext = createContext<DialogContextValue | null>(null);

function useDialogContext(component: string) {
  const context = useContext(DialogContext);
  if (!context) throw new Error(`${component} debe estar dentro de Dialog.`);
  return context;
}

function setRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === "function") ref(value);
  else if (ref) ref.current = value;
}

export interface DialogProps {
  children: ReactNode;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function Dialog({ children, open: controlledOpen, defaultOpen = false, onOpenChange }: DialogProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const generatedId = useId();
  const open = controlledOpen ?? uncontrolledOpen;
  const setOpen = useCallback((nextOpen: boolean) => {
    if (controlledOpen === undefined) setUncontrolledOpen(nextOpen);
    onOpenChange?.(nextOpen);
  }, [controlledOpen, onOpenChange]);

  return (
    <DialogContext.Provider
      value={{
        open,
        setOpen,
        dialogRef,
        triggerRef,
        titleId: `${generatedId}-title`,
        descriptionId: `${generatedId}-description`,
      }}
    >
      {children}
    </DialogContext.Provider>
  );
}

interface SlotButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean;
}

type SlottableButton = ReactElement<ButtonHTMLAttributes<HTMLButtonElement>>;

export const DialogTrigger = forwardRef<HTMLButtonElement, SlotButtonProps>(
  function DialogTrigger({ asChild = false, children, onClick, type = "button", ...props }, forwardedRef) {
    const { open, setOpen, triggerRef } = useDialogContext("DialogTrigger");
    const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
      onClick?.(event);
      if (!event.defaultPrevented) setOpen(true);
    };
    const sharedProps = {
      "aria-haspopup": "dialog" as const,
      "aria-expanded": open,
      "data-state": open ? "open" : "closed",
      onClick: handleClick,
    };

    if (asChild && isValidElement(children)) {
      const child = children as SlottableButton;
      return cloneElement(child, {
        ...props,
        ...sharedProps,
        onClick: (event: MouseEvent<HTMLButtonElement>) => {
          child.props.onClick?.(event);
          if (!event.defaultPrevented) handleClick(event);
        },
        ref: (node: HTMLButtonElement | null) => {
          triggerRef.current = node;
          setRef(forwardedRef, node);
        },
      } as ButtonHTMLAttributes<HTMLButtonElement>);
    }

    return (
      <button
        ref={(node) => {
          triggerRef.current = node;
          setRef(forwardedRef, node);
        }}
        type={type}
        {...sharedProps}
        {...props}
      >
        {children}
      </button>
    );
  },
);

export interface DialogContentProps extends Omit<ComponentPropsWithoutRef<"dialog">, "open"> {
  size?: "sm" | "md" | "lg";
  showClose?: boolean;
  closeLabel?: string;
  closeOnBackdrop?: boolean;
}

export const DialogContent = forwardRef<HTMLDialogElement, DialogContentProps>(
  function DialogContent(
    {
      children,
      className,
      size = "md",
      showClose = true,
      closeLabel = "Cerrar diálogo",
      closeOnBackdrop = true,
      onCancel,
      onClose,
      onClick,
      ...props
    },
    forwardedRef,
  ) {
    const context = useDialogContext("DialogContent");

    useEffect(() => {
      const dialog = context.dialogRef.current;
      if (!dialog) return;
      if (context.open && !dialog.open) {
        if (typeof dialog.showModal === "function") dialog.showModal();
        else dialog.setAttribute("open", "");
      }
      if (!context.open && dialog.open) {
        if (typeof dialog.close === "function") dialog.close();
        else dialog.removeAttribute("open");
      }
    }, [context.open, context.dialogRef]);

    return (
      <dialog
        ref={(node) => {
          context.dialogRef.current = node;
          setRef(forwardedRef, node);
        }}
        className={cx("pact-dialog-content", className)}
        data-size={size}
        aria-labelledby={context.titleId}
        aria-describedby={context.descriptionId}
        onCancel={(event) => {
          onCancel?.(event);
          if (!event.defaultPrevented) {
            event.preventDefault();
            context.setOpen(false);
            context.triggerRef.current?.focus();
          }
        }}
        onClose={(event) => {
          onClose?.(event);
          context.setOpen(false);
          context.triggerRef.current?.focus();
        }}
        onClick={(event) => {
          onClick?.(event);
          if (!event.defaultPrevented && closeOnBackdrop && event.target === event.currentTarget) {
            context.setOpen(false);
          }
        }}
        {...props}
      >
        {children}
        {showClose && (
          <DialogClose asChild>
            <IconButton className="pact-dialog-close" aria-label={closeLabel}>
              <Icon name="close" />
            </IconButton>
          </DialogClose>
        )}
      </dialog>
    );
  },
);

export const DialogClose = forwardRef<HTMLButtonElement, SlotButtonProps>(
  function DialogClose({ asChild = false, children, onClick, type = "button", ...props }, ref) {
    const { setOpen } = useDialogContext("DialogClose");
    const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
      onClick?.(event);
      if (!event.defaultPrevented) setOpen(false);
    };

    if (asChild && isValidElement(children)) {
      const child = children as SlottableButton;
      return cloneElement(child, {
        ...props,
        onClick: (event: MouseEvent<HTMLButtonElement>) => {
          child.props.onClick?.(event);
          if (!event.defaultPrevented) handleClick(event);
        },
        ref,
      } as ButtonHTMLAttributes<HTMLButtonElement>);
    }

    return <button ref={ref} type={type} onClick={handleClick} {...props}>{children}</button>;
  },
);

export function DialogHeader({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <header className={cx("pact-dialog-header", className)} {...props} />;
}

export const DialogTitle = forwardRef<HTMLHeadingElement, HTMLAttributes<HTMLHeadingElement>>(
  function DialogTitle({ className, ...props }, ref) {
    const { titleId } = useDialogContext("DialogTitle");
    return <h2 ref={ref} id={titleId} className={cx("pact-dialog-title", className)} {...props} />;
  },
);

export const DialogDescription = forwardRef<HTMLParagraphElement, HTMLAttributes<HTMLParagraphElement>>(
  function DialogDescription({ className, ...props }, ref) {
    const { descriptionId } = useDialogContext("DialogDescription");
    return <p ref={ref} id={descriptionId} className={cx("pact-dialog-description", className)} {...props} />;
  },
);

export function DialogBody({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cx("pact-dialog-body", className)} {...props} />;
}

export function DialogFooter({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <footer className={cx("pact-dialog-footer", className)} {...props} />;
}
