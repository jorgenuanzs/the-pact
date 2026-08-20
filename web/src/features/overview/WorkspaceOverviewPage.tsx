import { Page } from "@/components/layout/Page";
import { Button } from "@/components/ui/Button";
import { ErrorState, LoadingState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { useRooms } from "@/features/conversations/queries";
import { useWorkspaceRepositories } from "@/features/repositories/queries";
import { useWorkspaceAccess, useWorkspaceContext, useWorkspaceOverview } from "./queries";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { number, relativeDate } from "@/lib/format";

import { OverviewActiveWork } from "./OverviewActiveWork";
import { WorkspaceSectionIndex, type WorkspaceSectionEntry } from "./WorkspaceSectionIndex";

function workItems(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value as Array<Record<string, unknown>> : [];
}

function list(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function itemStatus(item: Record<string, unknown>): string {
  const intent = item.intent && typeof item.intent === "object" ? item.intent as Record<string, unknown> : {};
  return String(intent.status || item.status || "");
}

export function WorkspaceOverviewPage() {
  const { workspace, workspaceProjects, stream } = useWorkspace();
  const projectIDs = workspaceProjects.map((item) => item.id);
  const overview = useWorkspaceOverview(projectIDs);
  const context = useWorkspaceContext(workspace?.id);
  const repositories = useWorkspaceRepositories(projectIDs);
  const rooms = useRooms(workspace?.id);
  const access = useWorkspaceAccess(projectIDs);

  if (!workspace) return <ErrorState title="Workspace no encontrado" description="Puede que ya no tengas acceso a este workspace." />;
  if (overview.isPending && workspaceProjects.length) return <LoadingState label={`Cargando ${workspace.name}`} />;
  const data = overview.data || {};
  const items = workItems(data.work_items || data.intents);
  const events = stream.events.length ? stream.events : data.recent_events || [];
  const counts = data.counts || {};
  const blocked = items.filter((item) => itemStatus(item) === "blocked").length;
  const contextCount = Object.values(context.data || {}).filter(Array.isArray).reduce((total, values) => total + values.length, 0);
  const attentionContext = list(context.data?.open_questions).length + list(context.data?.risks).length;
  const repositoryCount = repositories.data?.length || 0;
  const unsyncedRepositories = repositories.data?.filter((repository) => repository.sync_status !== "synced").length || 0;
  const members = list(access.data?.members).length;
  const agents = list(access.data?.agents).length;
  const roomCount = rooms.data?.length || 0;
  const messageCount = rooms.data?.reduce((total, room) => total + number(room.message_count), 0) || 0;
  const activeCount = number(counts.active_intents ?? items.length);
  const sections: WorkspaceSectionEntry[] = [
    { to: "live", icon: "play", label: "Trabajo en vivo", description: blocked ? `${blocked} ${blocked === 1 ? "intent bloqueado" : "intents bloqueados"}` : "Sin bloqueos activos", value: `${activeCount} activos`, tone: blocked ? "warning" : "active" },
    { to: "conversations", icon: "hash", label: "Conversaciones", description: messageCount ? `${messageCount} mensajes compartidos` : "Sin mensajes todavía", value: `${roomCount} ${roomCount === 1 ? "sala" : "salas"}` },
    { to: "context", icon: "context", label: "Contexto", description: attentionContext ? `${attentionContext} elementos requieren revisión` : "Sin elementos pendientes", value: `${contextCount} registros`, tone: attentionContext ? "warning" : "neutral" },
    { to: "repositories", icon: "repository", label: "Repositorios", description: unsyncedRepositories ? `${unsyncedRepositories} sin sincronizar` : repositoryCount ? "Todos sincronizados" : "Ninguno vinculado", value: `${repositoryCount} repos`, tone: unsyncedRepositories ? "warning" : repositoryCount ? "active" : "neutral" },
    { to: "people", icon: "people", label: "Usuarios y agentes", description: `${members} ${members === 1 ? "usuario" : "usuarios"} · ${agents} ${agents === 1 ? "agente" : "agentes"}`, value: `${number(counts.active_sessions ?? data.active_work?.length)} conectados` },
    { to: "activity", icon: "activity", label: "Actividad", description: events[0] ? `Último evento ${relativeDate(events[0].occurred_at || events[0].created_at)}` : "Sin eventos recientes", value: `${events.length} recientes` },
  ];

  return (
    <Page
      kicker="WORKSPACE"
      title={workspace.name}
      description={workspace.description || "Coordina repositorios, contexto, conversaciones, personas y agentes desde un único lugar."}
    >
      <section className="attention-stack" aria-label="Atención">
        {blocked ? <div className="attention-banner" data-tone="warning"><StatusChip tone="warning">BLOQUEO</StatusChip><span><strong>Hay trabajo bloqueado en este workspace.</strong><small>Revisa los intents y sus scopes antes de iniciar trabajo nuevo.</small></span><Button variant="secondary" size="sm" onClick={() => document.getElementById("overview-active-work")?.scrollIntoView()}>Ver trabajo</Button></div> : null}
        {Array.isArray(context.data?.warnings) ? context.data.warnings.map((warning) => <div className="attention-banner" data-tone="info" key={warning}><StatusChip tone="info">CONTEXTO</StatusChip><span><strong>{warning}</strong></span></div>) : null}
      </section>

      <div className="overview-command-grid">
        <OverviewActiveWork items={items} />
        <WorkspaceSectionIndex entries={sections} />
      </div>
      {data.generated_at ? <p className="generated-at">Estado generado {relativeDate(data.generated_at)}</p> : null}
    </Page>
  );
}
