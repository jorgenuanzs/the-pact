import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";

import type { AgentEnrollmentResult, ProjectSummary } from "@/api/types";
import { Page } from "@/components/layout/Page";
import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { DataTable, DataTableBody, DataTableCell, DataTableHead, DataTableHeaderCell, DataTableRow } from "@/components/ui/DataTable";
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/Dialog";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { useToast } from "@/components/ui/Toast";
import { useWorkspaceAccess } from "@/features/overview/queries";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { relativeDate, roleLabel, text } from "@/lib/format";
import { desktopBridge } from "@/platform/desktop";

import { useEnrollAgent } from "./queries";

type AgentType = "codex" | "claude";

function actorList(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value as Array<Record<string, unknown>> : [];
}

function isPendingAgent(entry: Record<string, unknown>): boolean {
  return !entry.connected && Number(entry.pending_enrollments || 0) > 0;
}

function agentMeta(entry: Record<string, unknown>): string {
  const sessions = Number(entry.session_count || 0);
  const enrollments = Number(entry.enrollment_count || 0);
  return [
    text(entry.agent_type, "Agente"),
    enrollments ? `${enrollments} ${enrollments === 1 ? "destino autorizado" : "destinos autorizados"}` : "",
    sessions ? `${sessions} ${sessions === 1 ? "sesión" : "sesiones"}` : "Aún no iniciado",
  ].filter(Boolean).join(" · ");
}

function statusLabel(entry: Record<string, unknown>, connected: boolean, agent: boolean): string {
  if (agent && connected) return "Conectado";
  if (agent && entry.status === "retired") return "Retirado";
  if (agent && entry.access_active === false) return "Sin acceso";
  if (agent && isPendingAgent(entry)) return "Pendiente de iniciar";
  if (agent) return "Desconectado";
  if (entry.status === "active") return "Activo";
  if (entry.status === "disabled") return "Desactivado";
  if (entry.status === "retired") return "Retirado";
  return "Inactivo";
}

function AccessTable({
  entries,
  agents = false,
  onHowToStart,
}: {
  entries: Array<Record<string, unknown>>;
  agents?: boolean;
  onHowToStart?: (entry: Record<string, unknown>) => void;
}) {
  if (!entries.length) return (
    <EmptyState
      title={agents ? "Aún no hay agentes registrados" : "No hay usuarios autorizados"}
      description={agents ? "Añade Codex o Claude para autorizarlo en este workspace. Después, abre un chat nuevo en una carpeta vinculada para iniciar su primera sesión." : undefined}
    />
  );
  return (
    <DataTable>
      <DataTableHead><tr>
        <DataTableHeaderCell>Identidad</DataTableHeaderCell>
        <DataTableHeaderCell>{agents ? "Responsable" : "Acceso"}</DataTableHeaderCell>
        <DataTableHeaderCell>Estado</DataTableHeaderCell>
        <DataTableHeaderCell>Última señal</DataTableHeaderCell>
        {agents ? <DataTableHeaderCell><span className="pact-visually-hidden">Acciones</span></DataTableHeaderCell> : null}
      </tr></DataTableHead>
      <DataTableBody>{entries.map((entry, index) => {
        const name = text(entry.display_name || entry.agent_id || entry.principal_id, "Identidad");
        const connected = Boolean(entry.connected);
        const pending = agents && isPendingAgent(entry);
        const active = agents ? connected : entry.status === "active";
        return (
          <DataTableRow key={text(entry.logical_agent_key || entry.principal_id || entry.agent_id, String(index))}>
            <DataTableCell><span className="identity-cell"><Avatar name={name} kind={agents ? "agent" : "person"} size="sm" /><span><strong>{name}</strong><small>{agents ? agentMeta(entry) : text(entry.principal_type, "Usuario")}</small></span></span></DataTableCell>
            <DataTableCell><strong>{agents ? text(entry.sponsor_display_name) : roleLabel(text(entry.effective_role, "viewer"))}</strong><small className="table-detail">{text(entry.access_source || entry.sponsor_effective_role)}</small></DataTableCell>
            <DataTableCell><StatusChip tone={active ? "active" : entry.access_active === false ? "danger" : pending ? "warning" : "neutral"}>{statusLabel(entry, connected, agents)}</StatusChip></DataTableCell>
            <DataTableCell>{pending ? "Nunca" : relativeDate(entry.last_seen_at)}</DataTableCell>
            {agents ? <DataTableCell>{pending ? <Button size="sm" variant="ghost" onClick={() => onHowToStart?.(entry)}>Cómo iniciar</Button> : null}</DataTableCell> : null}
          </DataTableRow>
        );
      })}</DataTableBody>
    </DataTable>
  );
}

export function PeoplePage() {
  const { workspace, workspaceProjects } = useWorkspace();
  const navigate = useNavigate();
  const access = useWorkspaceAccess(workspace?.id);
  const [addOpen, setAddOpen] = useState(false);
  const [instructionsAgent, setInstructionsAgent] = useState<Record<string, unknown> | null>(null);
  if (!workspace) return <ErrorState title="Workspace no encontrado" />;
  if (access.isPending) return <LoadingState label="Cargando usuarios y agentes" />;
  if (access.error) return <ErrorState title="No se pudo cargar el acceso" description={(access.error as Error).message} />;
  const members = actorList(access.data?.members);
  const agents = actorList(access.data?.agents);
  const configureLocally = (agentType: AgentType, projectID: string) => {
    setAddOpen(false);
    navigate(`/local/folders?connect=agent&client=${encodeURIComponent(agentType)}&workspace=${encodeURIComponent(workspace.id)}&project=${encodeURIComponent(projectID)}`);
  };
  return (
    <Page
      kicker="GESTIÓN"
      title="Usuarios y agentes"
      actions={<Button size="sm" onClick={() => setAddOpen(true)}>Agregar</Button>}
    >
      <section className="stacked-sections">
        <section><header className="section-heading"><h2>Usuarios</h2><span>{members.length}</span></header><AccessTable entries={members} /></section>
        <section><header className="section-heading"><h2>Agentes</h2><span>{agents.length}</span></header><AccessTable entries={agents} agents onHowToStart={setInstructionsAgent} /></section>
      </section>
      <AddWorkspaceActorDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        workspaceID={workspace.id}
        workspaceName={workspace.name}
        projects={workspaceProjects}
        onAddPerson={() => { setAddOpen(false); navigate("/organization/access"); }}
        onConfigureLocally={configureLocally}
      />
      <AgentInstructionsDialog
        open={Boolean(instructionsAgent)}
        agent={instructionsAgent}
        onOpenChange={(open) => { if (!open) setInstructionsAgent(null); }}
        onConfigureLocally={() => {
          const agentType = text(instructionsAgent?.agent_type, "codex") as AgentType;
          configureLocally(agentType, text(instructionsAgent?.enrollment_project_id, workspaceProjects[0]?.id || ""));
          setInstructionsAgent(null);
        }}
      />
    </Page>
  );
}

function AddWorkspaceActorDialog({
  open,
  onOpenChange,
  workspaceID,
  workspaceName,
  projects,
  onAddPerson,
  onConfigureLocally,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceID: string;
  workspaceName: string;
  projects: ProjectSummary[];
  onAddPerson: () => void;
  onConfigureLocally: (agentType: AgentType, projectID: string) => void;
}) {
  const [mode, setMode] = useState<"choice" | "agent" | "success">("choice");
  const [agentType, setAgentType] = useState<AgentType>("codex");
  const [projectID, setProjectID] = useState("");
  const [result, setResult] = useState<AgentEnrollmentResult | null>(null);
  const enroll = useEnrollAgent(workspaceID);
  const { toast } = useToast();
  const selectedProjectID = projectID || projects[0]?.id || "";

  useEffect(() => {
    if (!open) return;
    setMode("choice");
    setAgentType("codex");
    setProjectID(projects[0]?.id || "");
    setResult(null);
    enroll.reset();
  }, [open, projects]); // eslint-disable-line react-hooks/exhaustive-deps

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!selectedProjectID) return;
    try {
      const next = await enroll.mutateAsync({ project_id: selectedProjectID, agent_type: agentType, client_type: `${agentType}-mcp` });
      setResult(next);
      setMode("success");
      toast({
        title: `${agentType === "codex" ? "Codex" : "Claude"} añadido al workspace`,
        description: next.enrollment.status === "active" ? "La autorización ya existía y el agente tiene historial de sesión." : "Quedará pendiente hasta que abras su primera sesión MCP.",
        tone: "success",
      });
    } catch (error) {
      toast({ title: "No se pudo añadir el agente", description: (error as Error).message, tone: "danger" });
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        {mode === "choice" ? <>
          <DialogHeader><p className="pact-kicker">NUEVO ACCESO</p><DialogTitle>Agregar a {workspaceName}</DialogTitle><DialogDescription>Las personas reciben permisos; los agentes de IA quedan vinculados a la cuenta de su responsable.</DialogDescription></DialogHeader>
          <DialogBody><div className="access-add-choices">
            <button type="button" onClick={onAddPerson}><span>PE</span><strong>Invitar persona</strong><small>Abre Acceso y seguridad para crear una invitación y asignar permisos.</small></button>
            <button type="button" onClick={() => setMode("agent")}><span>IA</span><strong>Agregar agente de IA</strong><small>Autoriza Codex o Claude y muéstralo antes de su primera sesión.</small></button>
          </div></DialogBody>
          <DialogFooter><Button variant="secondary" onClick={() => onOpenChange(false)}>Cancelar</Button></DialogFooter>
        </> : null}

        {mode === "agent" ? <form onSubmit={submit}>
          <DialogHeader><p className="pact-kicker">AGENTE DE IA</p><DialogTitle>Autorizar un cliente</DialogTitle><DialogDescription>El agente pertenecerá a tu identidad y aparecerá como pendiente hasta que el cliente abra PACT MCP.</DialogDescription></DialogHeader>
          <DialogBody className="pact-form-stack">
            <div className="access-agent-choices" role="radiogroup" aria-label="Cliente de IA">
              {(["codex", "claude"] as const).map((candidate) => <button key={candidate} type="button" role="radio" aria-checked={agentType === candidate} data-selected={agentType === candidate || undefined} onClick={() => setAgentType(candidate)}><span>{candidate === "codex" ? "CX" : "CL"}</span><strong>{candidate === "codex" ? "Codex" : "Claude Code"}</strong><small>Integración MCP por carpeta</small></button>)}
            </div>
            <label className="pact-field"><span>Repositorio técnico</span><select required value={selectedProjectID} onChange={(event) => setProjectID(event.target.value)}><option value="">Selecciona un repositorio</option>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select><small>Define en qué carpeta debe iniciarse este agente. Seguirá visible a nivel del workspace.</small></label>
            {!projects.length ? <div className="local-inline-alert">Conecta primero un repositorio a este workspace para poder configurar un agente.</div> : null}
          </DialogBody>
          <DialogFooter><Button variant="secondary" onClick={() => setMode("choice")}>Atrás</Button><Button type="submit" loading={enroll.isPending} disabled={!selectedProjectID}>Agregar agente</Button></DialogFooter>
        </form> : null}

        {mode === "success" && result ? <>
          <DialogHeader><p className="pact-kicker">AGENTE AUTORIZADO</p><DialogTitle>{result.enrollment.agent_name}{result.enrollment.status === "active" ? " ya estaba registrado" : " está pendiente"}</DialogTitle><DialogDescription>{result.enrollment.status === "active" ? `PACT conservó la autorización existente en ${workspaceName}.` : `PACT ya lo muestra en ${workspaceName}; aún no cuenta como conectado.`}</DialogDescription></DialogHeader>
          <DialogBody><ol className="agent-start-steps"><li><strong>Configura una carpeta local</strong><span>Vincula el checkout del repositorio con este workspace desde PACT Desktop.</span></li><li><strong>Abre un chat nuevo</strong><span>Inicia {agentType === "codex" ? "Codex" : "Claude Code"} dentro de esa carpeta para cargar la configuración MCP.</span></li><li><strong>PACT lo activará automáticamente</strong><span>La primera sesión cambia su estado de pendiente a conectado y comienza a emitir latidos.</span></li></ol></DialogBody>
          <DialogFooter><Button variant="secondary" onClick={() => onOpenChange(false)}>Cerrar</Button>{desktopBridge() ? <Button onClick={() => onConfigureLocally(agentType, result.enrollment.project_id)}>Configurar este computador</Button> : null}</DialogFooter>
        </> : null}
      </DialogContent>
    </Dialog>
  );
}

function AgentInstructionsDialog({ open, agent, onOpenChange, onConfigureLocally }: { open: boolean; agent: Record<string, unknown> | null; onOpenChange: (open: boolean) => void; onConfigureLocally: () => void }) {
  const name = text(agent?.display_name, "El agente");
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent size="sm"><DialogHeader><p className="pact-kicker">PENDIENTE DE INICIAR</p><DialogTitle>Iniciar {name}</DialogTitle><DialogDescription>La autorización ya existe; falta que el cliente abra su primera sesión MCP.</DialogDescription></DialogHeader><DialogBody><ol className="agent-start-steps"><li><strong>Abre PACT Desktop</strong><span>Configura el cliente en la carpeta local que corresponde a este workspace.</span></li><li><strong>Abre un chat nuevo en esa carpeta</strong><span>Los chats que ya estaban abiertos pueden no recargar la configuración MCP.</span></li><li><strong>Comprueba el estado</strong><span>PACT lo marcará como conectado al recibir el primer latido.</span></li></ol></DialogBody><DialogFooter><Button variant="secondary" onClick={() => onOpenChange(false)}>Cerrar</Button>{desktopBridge() ? <Button onClick={onConfigureLocally}>Configurar este computador</Button> : null}</DialogFooter></DialogContent></Dialog>;
}
