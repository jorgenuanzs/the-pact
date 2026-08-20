import { useEffect, useState, type FormEvent } from "react";

import { Page } from "@/components/layout/Page";
import { Button } from "@/components/ui/Button";
import { ColorPicker, type WorkspaceColor } from "@/components/ui/ColorPicker";
import { Panel, PanelBody, PanelFooter, PanelHeader, PanelTitle } from "@/components/ui/Panel";
import { ErrorState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { useToast } from "@/components/ui/Toast";
import { canManage, roleLabel, text } from "@/lib/format";

import { useUpdateWorkspace } from "./queries";
import { useWorkspace } from "./WorkspaceContext";

export function WorkspaceSettingsPage() {
  const { workspace, principal, github } = useWorkspace();
  const editable = canManage(principal?.organization_role);
  const mutation = useUpdateWorkspace(workspace?.id || "");
  const { toast } = useToast();
  const [name, setName] = useState(workspace?.name || "");
  const [description, setDescription] = useState(workspace?.description || "");
  const [color, setColor] = useState<WorkspaceColor>((workspace?.color as WorkspaceColor) || "#c9ee4d");
  useEffect(() => { setName(workspace?.name || ""); setDescription(workspace?.description || ""); setColor((workspace?.color as WorkspaceColor) || "#c9ee4d"); }, [workspace]);
  if (!workspace) return <ErrorState title="Workspace no encontrado" />;
  const save = async (event: FormEvent) => {
    event.preventDefault();
    try { await mutation.mutateAsync({ name: name.trim(), description: description.trim(), color }); toast({ title: "Configuración guardada", tone: "success" }); }
    catch (error) { toast({ title: "No se pudo guardar", description: (error as Error).message, tone: "danger" }); }
  };
  return <Page kicker="GESTIÓN" title="Configuración" description={`Identidad, color e integraciones de ${workspace.name}.`}><div className="settings-grid"><Panel><form onSubmit={save}><PanelHeader><span><p className="pact-kicker">IDENTIDAD</p><PanelTitle>Workspace</PanelTitle></span></PanelHeader><PanelBody className="pact-form-stack"><label className="pact-field"><span>Nombre</span><input required disabled={!editable} value={name} onChange={(event) => setName(event.target.value)} /></label><label className="pact-field"><span>Descripción</span><textarea rows={4} disabled={!editable} value={description} onChange={(event) => setDescription(event.target.value)} /></label><ColorPicker value={color} disabled={!editable} onValueChange={setColor} /></PanelBody><PanelFooter><small>{editable ? "El slug y las conexiones existentes no cambiarán." : "Solo propietarios y administradores pueden modificar esta configuración."}</small><Button type="submit" loading={mutation.isPending} disabled={!editable}>Guardar cambios</Button></PanelFooter></form></Panel><aside className="settings-side"><Panel padding="md"><PanelTitle>Información</PanelTitle><dl className="fact-list"><div><dt>Slug</dt><dd><code>{workspace.slug}</code></dd></div><div><dt>ID</dt><dd><code>{workspace.id}</code></dd></div><div><dt>Tu rol</dt><dd>{roleLabel(principal?.organization_role)}</dd></div><div><dt>Versión</dt><dd>{text(workspace.version)}</dd></div></dl></Panel><Panel padding="md"><PanelTitle>GitHub</PanelTitle><StatusChip tone={github?.configured ? "active" : "warning"}>{github?.configured ? "Configurado" : "No configurado"}</StatusChip><p>La instalación de GitHub pertenece a la organización; cada workspace usa únicamente sus repositorios vinculados.</p></Panel></aside></div></Page>;
}
