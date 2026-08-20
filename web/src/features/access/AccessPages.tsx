import { Page } from "@/components/layout/Page";
import { Avatar } from "@/components/ui/Avatar";
import { DataTable, DataTableBody, DataTableCell, DataTableHead, DataTableHeaderCell, DataTableRow } from "@/components/ui/DataTable";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { useWorkspaceAccess } from "@/features/overview/queries";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { relativeDate, roleLabel, text } from "@/lib/format";

function actorList(value: unknown): Array<Record<string, unknown>> { return Array.isArray(value) ? value as Array<Record<string, unknown>> : []; }

function agentMeta(entry: Record<string, unknown>): string {
  const sessions = Number(entry.session_count || 0);
  const identities = Number(entry.identity_count || 1);
  return [
    text(entry.agent_type, "Agente"),
    sessions ? `${sessions} ${sessions === 1 ? "sesión" : "sesiones"}` : "Sin sesiones",
    identities > 1 ? `${identities} registros consolidados` : "",
  ].filter(Boolean).join(" · ");
}

function statusLabel(entry: Record<string, unknown>, connected: boolean, agent: boolean): string {
  if (agent && connected) return "Conectado";
  if (agent && entry.status === "retired") return "Retirado";
  if (agent && entry.access_active === false) return "Sin acceso";
  if (agent) return "Desconectado";
  if (entry.status === "active") return "Activo";
  if (entry.status === "disabled") return "Desactivado";
  if (entry.status === "retired") return "Retirado";
  return "Inactivo";
}

function AccessTable({ entries, agents = false }: { entries: Array<Record<string, unknown>>; agents?: boolean }) {
  if (!entries.length) return <EmptyState title={agents ? "No hay agentes autorizados" : "No hay usuarios autorizados"} />;
  return <DataTable><DataTableHead><tr><DataTableHeaderCell>Identidad</DataTableHeaderCell><DataTableHeaderCell>{agents ? "Responsable" : "Acceso"}</DataTableHeaderCell><DataTableHeaderCell>Estado</DataTableHeaderCell><DataTableHeaderCell>Última señal</DataTableHeaderCell></tr></DataTableHead><DataTableBody>{entries.map((entry, index) => { const name = text(entry.display_name || entry.agent_id || entry.principal_id, "Identidad"); const connected = Boolean(entry.connected); const active = agents ? connected : entry.status === "active"; return <DataTableRow key={text(entry.logical_agent_key || entry.principal_id || entry.agent_id, String(index))}><DataTableCell><span className="identity-cell"><Avatar name={name} kind={agents ? "agent" : "person"} size="sm" /><span><strong>{name}</strong><small>{agents ? agentMeta(entry) : text(entry.principal_type, "Usuario")}</small></span></span></DataTableCell><DataTableCell><strong>{agents ? text(entry.sponsor_display_name) : roleLabel(text(entry.effective_role, "viewer"))}</strong><small className="table-detail">{text(entry.access_source || entry.sponsor_effective_role)}</small></DataTableCell><DataTableCell><StatusChip tone={active ? "active" : entry.access_active === false ? "danger" : "neutral"}>{statusLabel(entry, connected, agents)}</StatusChip></DataTableCell><DataTableCell>{relativeDate(entry.last_seen_at)}</DataTableCell></DataTableRow>; })}</DataTableBody></DataTable>;
}

export function PeoplePage() {
  const { workspace, workspaceProjects } = useWorkspace(); const access = useWorkspaceAccess(workspaceProjects.map((item) => item.id));
  if (!workspace || !workspaceProjects.length) return <ErrorState title="Workspace sin unidad operativa" />;
  if (access.isPending) return <LoadingState label="Cargando usuarios y agentes" />;
  const members = actorList(access.data?.members); const agents = actorList(access.data?.agents);
  return <Page kicker="GESTIÓN" title="Usuarios y agentes" description={`Quién tiene acceso a ${workspace.name} y qué agentes dependen de cada usuario.`}><section className="stacked-sections"><section><header className="section-heading"><h2>Usuarios</h2><span>{members.length}</span></header><AccessTable entries={members} /></section><section><header className="section-heading"><h2>Agentes</h2><span>{agents.length}</span></header><AccessTable entries={agents} agents /></section></section></Page>;
}
