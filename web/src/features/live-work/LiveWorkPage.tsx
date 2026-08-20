import { Page } from "@/components/layout/Page";
import { Avatar } from "@/components/ui/Avatar";
import { ErrorState, LoadingState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { useWorkspaceOverview } from "@/features/overview/queries";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { relativeDate, text } from "@/lib/format";

import { LiveWorkTabs } from "./LiveWorkTabs";

export function LiveWorkPage() {
  const { workspace, project, workspaceProjects } = useWorkspace();
  const overview = useWorkspaceOverview(workspaceProjects.map((item) => item.id));
  if (!workspace) return <ErrorState title="Workspace no encontrado" />;
  if (!project) return <Page title="Trabajo en vivo" description="Este workspace aún no tiene una unidad operativa preparada."><ErrorState title="No hay trabajo operativo" description="Inicializa PACT desde un repositorio para empezar a coordinar trabajo." /></Page>;
  if (overview.isPending) return <LoadingState label="Cargando trabajo en vivo" />;
  const data = overview.data || {};
  const actors = Array.isArray(data.active_work) ? data.active_work as Array<Record<string, unknown>> : [];
  const intents = Array.isArray(data.work_items) ? data.work_items : [];
  const handoffs = Array.isArray(data.handoffs) ? data.handoffs as Array<Record<string, unknown>> : [];
  return (
    <Page kicker="OPERACIÓN" title="Trabajo en vivo" description={`Intents, scopes, bloqueos y latidos de ${workspace.name}.`}>
      <section className="presence-strip" aria-label="Actores conectados">
        <header><strong>Ahora</strong><span>{actors.length} actores con presencia registrada</span></header>
        <div>{actors.map((actor, index) => { const name = text(actor.actor_name || actor.responsible_name || actor.actor_id, "Actor"); return <article key={text(actor.actor_id || actor.session_id, String(index))}><Avatar name={name} kind={text(actor.actor_kind).includes("agent") ? "agent" : "person"} size="sm" /><span><strong>{name}</strong><small>{text(actor.intent_title || actor.session_status, "Sin intent")}</small></span><StatusChip tone={actor.session_status === "active" ? "active" : "neutral"}>{relativeDate(actor.last_seen_at)}</StatusChip></article>; })}</div>
      </section>
      <LiveWorkTabs intents={intents} codeActivity={data.code_activity} handoffs={handoffs} />
    </Page>
  );
}
