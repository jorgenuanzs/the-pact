import { Button } from "./Button";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "./Dialog";

export function ConfirmDialog({ open, title, description, confirmLabel = "Confirmar", busy = false, destructive = false, onConfirm, onOpenChange }: { open: boolean; title: string; description: string; confirmLabel?: string; busy?: boolean; destructive?: boolean; onConfirm: () => void; onOpenChange: (open: boolean) => void }) {
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent size="sm"><DialogHeader><DialogTitle>{title}</DialogTitle><DialogDescription>{description}</DialogDescription></DialogHeader><DialogBody /><DialogFooter><Button variant="secondary" onClick={() => onOpenChange(false)}>Cancelar</Button><Button variant={destructive ? "danger" : "primary"} loading={busy} onClick={onConfirm}>{confirmLabel}</Button></DialogFooter></DialogContent></Dialog>;
}
