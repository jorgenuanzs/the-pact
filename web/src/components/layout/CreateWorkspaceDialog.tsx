import { useEffect, useState, type FormEvent } from "react";

import { Button } from "@/components/ui/Button";
import { ColorPicker, type WorkspaceColor } from "@/components/ui/ColorPicker";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/Dialog";
import { useToast } from "@/components/ui/Toast";
import { APIError } from "@/api/client";
import { slugify } from "@/lib/format";
import { useCreateWorkspace } from "@/features/workspaces/queries";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (workspaceID: string) => void;
}

export function CreateWorkspaceDialog({ open, onOpenChange, onCreated }: Props) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [color, setColor] = useState<WorkspaceColor>("#c9ee4d");
  const [slugEdited, setSlugEdited] = useState(false);
  const mutation = useCreateWorkspace();
  const { toast } = useToast();

  useEffect(() => {
    if (!open) return;
    setName("");
    setSlug("");
    setDescription("");
    setColor("#c9ee4d");
    setSlugEdited(false);
  }, [open]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!name.trim() || !slugify(slug)) return;
    try {
      const workspace = await mutation.mutateAsync({
        name: name.trim(),
        slug: slugify(slug),
        description: description.trim(),
        color,
      });
      toast({ title: `Workspace ${workspace.name} creado`, tone: "success" });
      onOpenChange(false);
      onCreated(workspace.id);
    } catch (error) {
      toast({ title: "No se pudo crear el workspace", description: error instanceof APIError ? error.message : undefined, tone: "danger" });
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <form onSubmit={submit}>
          <DialogHeader>
            <p className="pact-kicker">NUEVO WORKSPACE</p>
            <DialogTitle>Crear workspace</DialogTitle>
            <DialogDescription>Reúne repositorios, contexto, conversaciones, personas y agentes.</DialogDescription>
          </DialogHeader>
          <DialogBody className="pact-form-stack">
            <label className="pact-field">
              <span>Nombre</span>
              <input
                autoFocus
                required
                value={name}
                onChange={(event) => {
                  setName(event.target.value);
                  if (!slugEdited) setSlug(slugify(event.target.value));
                }}
              />
            </label>
            <label className="pact-field">
              <span>Slug</span>
              <input required value={slug} onChange={(event) => { setSlugEdited(true); setSlug(slugify(event.target.value)); }} />
            </label>
            <label className="pact-field">
              <span>Descripción</span>
              <textarea rows={3} value={description} onChange={(event) => setDescription(event.target.value)} />
            </label>
            <ColorPicker value={color} onValueChange={setColor} name="create-workspace-color" />
          </DialogBody>
          <DialogFooter>
            <Button variant="secondary" onClick={() => onOpenChange(false)}>Cancelar</Button>
            <Button type="submit" loading={mutation.isPending}>Crear workspace</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
