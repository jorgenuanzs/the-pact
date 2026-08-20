import { useDeferredValue, useMemo, useState } from "react";

import type { PactEvent } from "@/api/types";
import { Page } from "@/components/layout/Page";
import { Button } from "@/components/ui/Button";
import { Icon } from "@/components/ui/Icon";
import { ErrorState, LoadingState } from "@/components/ui/States";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";

import { ActivityTimeline, activityCategory, activityLabel, type ActivityCategory } from "./ActivityTimeline";
import { useWorkspaceActivity } from "./queries";

const categories: Array<{ value: ActivityCategory | "all"; label: string }> = [
  { value: "all", label: "Toda la actividad" },
  { value: "work", label: "Trabajo e intents" },
  { value: "sessions", label: "Sesiones y agentes" },
  { value: "code", label: "Código y repositorios" },
  { value: "context", label: "Contexto y conocimiento" },
  { value: "access", label: "Acceso y gestión" },
  { value: "system", label: "Sistema" },
];

function eventKey(event: PactEvent): string {
  return String(event.id || `${event.project_id || "project"}:${event.sequence}`);
}

function eventTimestamp(event: PactEvent): number {
  const value = Date.parse(String(event.occurred_at || event.created_at || ""));
  return Number.isNaN(value) ? 0 : value;
}

function matchesSearch(event: PactEvent, query: string): boolean {
  if (!query) return true;
  const kind = String(event.type || event.event_type || "");
  const haystack = [
    activityLabel(kind), kind, event.id, event.sequence, event.actor_id,
    event.session_id, event.intent_id, JSON.stringify(event.data || event.payload || {}),
  ].join(" ").toLocaleLowerCase("es");
  return haystack.includes(query.toLocaleLowerCase("es"));
}

export function ActivityPage() {
  const { workspace, workspaceProjects, stream } = useWorkspace();
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState<ActivityCategory | "all">("all");
  const deferredSearch = useDeferredValue(search.trim());
  const history = useWorkspaceActivity(workspaceProjects.map((item) => item.id), deferredSearch);

  const events = useMemo(() => {
    const merged = new Map<string, PactEvent>();
    for (const event of [...stream.events.filter((item) => matchesSearch(item, deferredSearch)), ...history.events]) {
      merged.set(eventKey(event), event);
    }
    return [...merged.values()]
      .filter((event) => category === "all" || activityCategory(String(event.type || event.event_type || "")) === category)
      .sort((left, right) => eventTimestamp(right) - eventTimestamp(left));
  }, [category, deferredSearch, history.events, stream.events]);

  return (
    <Page title="Actividad" description={`Historial durable y flujo en vivo de ${workspace?.name || "este workspace"}.`} fullBleed className="activity-page">
      <div className="activity-browser">
        <div className="activity-toolbar">
          <label className="activity-search"><Icon name="search" size="sm" /><span className="pact-visually-hidden">Buscar actividad</span><input type="search" placeholder="Buscar por evento, actor, ID o contenido" value={search} onChange={(event) => setSearch(event.target.value)} /></label>
          <label className="activity-category"><span className="pact-visually-hidden">Filtrar actividad</span><select value={category} onChange={(event) => setCategory(event.target.value as ActivityCategory | "all")}>{categories.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
          <span className="activity-result-count">{events.length} {events.length === 1 ? "evento cargado" : "eventos cargados"}</span>
        </div>
        {history.loading ? <LoadingState label="Cargando actividad reciente" /> : history.error && !events.length ? <ErrorState title="No se pudo cargar la actividad" description={history.error.message} /> : <ActivityTimeline events={events} variant="feed" />}
        {history.error && events.length ? <p className="activity-load-error" role="alert">{history.error.message}</p> : null}
        {history.hasMore ? <div className="activity-load-more"><Button variant="secondary" loading={history.loadingMore} onClick={() => void history.loadMore()}>Cargar actividad anterior</Button></div> : events.length ? <p className="activity-end">Llegaste al inicio del historial disponible.</p> : null}
      </div>
    </Page>
  );
}
