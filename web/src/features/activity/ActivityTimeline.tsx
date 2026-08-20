import { useEffect, useRef, useState } from "react";

import type { PactEvent } from "@/api/types";
import { EmptyState } from "@/components/ui/States";
import { formatDate, shortID, text } from "@/lib/format";

const labels: Record<string, string> = {
  "pact.intent.started.v1": "Intent iniciado",
  "pact.intent.active.v1": "Intent activo",
  "pact.intent.blocked.v1": "Intent bloqueado",
  "pact.intent.completed.v1": "Intent completado",
  "pact.session.started.v1": "Sesión conectada",
  "pact.session.closed.v1": "Sesión cerrada",
  "pact.repository.canonical_synced.v1": "Repositorio verificado",
  "pact.repository.sync_failed.v1": "Error al verificar repositorio",
  "pact.project.repository_attached.v1": "Repositorio vinculado",
  "pact.handoff.offered.v1": "Handoff ofrecido",
  "pact.handoff.accepted.v1": "Handoff aceptado",
  "pact.context.compiled.v1": "Contexto compilado",
  "pact.knowledge.record.proposed.v1": "Conocimiento propuesto",
};

export type ActivityCategory = "work" | "sessions" | "code" | "context" | "access" | "system";

export function activityLabel(kind: string): string {
  return labels[kind] || kind || "Evento de PACT";
}

export function activityCategory(kind: string): ActivityCategory {
  if (kind.includes("intent") || kind.includes("handoff") || kind.includes("scope") || kind.includes("worktree")) return "work";
  if (kind.includes("session") || kind.includes("agent") || kind.includes("node")) return "sessions";
  if (kind.includes("repository") || kind.includes("git") || kind.includes("changeset")) return "code";
  if (kind.includes("context") || kind.includes("knowledge") || kind.includes("room") || kind.includes("message")) return "context";
  if (kind.includes("access") || kind.includes("member") || kind.includes("invitation") || kind.includes("principal")) return "access";
  return "system";
}

function eventReference(event: PactEvent): string {
  return text(event.intent_id || event.session_id || event.aggregate_id || event.id, "—");
}

function eventKey(event: PactEvent, index: number): string {
  return text(event.id || `${event.project_id || "project"}:${event.sequence}`, String(index));
}

export function ActivityTimeline({ events, variant = "timeline" }: { events: PactEvent[]; variant?: "timeline" | "feed" }) {
  if (!events.length) return <EmptyState title="No hay actividad que coincida" description="Prueba otra búsqueda o filtro. Los eventos durables aparecerán aquí." />;
  if (variant === "feed") return <ActivityFeed events={events} />;
  return (
    <ol className="activity-timeline">
      {events.map((event, index) => {
        const kind = text(event.type || event.event_type, "Evento de PACT");
        return (
          <li key={text(event.id || event.sequence, String(index))}>
            <span className="activity-sequence">{shortID(event.sequence, 7)}</span>
            <span className="activity-marker" aria-hidden="true" />
            <article>
              <header><strong>{activityLabel(kind)}</strong><time>{formatDate(event.occurred_at || event.created_at)}</time></header>
              <p>{event.actor?.display_name || text(event.actor_id, "PACT")} registró este cambio.</p>
            </article>
          </li>
        );
      })}
    </ol>
  );
}

function ActivityFeed({ events }: { events: PactEvent[] }) {
  const [expandedEvent, setExpandedEvent] = useState<string | null>(null);
  const feedRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (expandedEvent && !events.some((event, index) => eventKey(event, index) === expandedEvent)) setExpandedEvent(null);
  }, [events, expandedEvent]);

  useEffect(() => {
    if (!expandedEvent) return;
    const dismissExpandedRow = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Element
        && feedRef.current?.contains(target)
        && target.closest(".activity-feed-toggle, .activity-feed-details")) return;
      setExpandedEvent(null);
    };
    const dismissWithKeyboard = (event: KeyboardEvent) => {
      if (event.key === "Escape") setExpandedEvent(null);
    };
    document.addEventListener("pointerdown", dismissExpandedRow);
    document.addEventListener("keydown", dismissWithKeyboard);
    return () => {
      document.removeEventListener("pointerdown", dismissExpandedRow);
      document.removeEventListener("keydown", dismissWithKeyboard);
    };
  }, [expandedEvent]);

  return (
    <section ref={feedRef} className="activity-feed" aria-label="Historial de actividad">
      <div className="activity-feed-header"><span>MOMENTO</span><span>EVENTO</span><span>ACTOR</span><span>REFERENCIA</span><span>DATOS</span></div>
      <ol>
        {events.map((event, index) => {
          const key = eventKey(event, index);
          const kind = text(event.type || event.event_type, "Evento de PACT");
          const actor = event.actor?.display_name || text(event.actor_id, "PACT");
          const data = event.data || event.payload;
          const hasData = Boolean(data && Object.keys(data).length);
          const expanded = expandedEvent === key;
          const detailsID = `activity-data-${key.replace(/[^a-zA-Z0-9_-]/g, "-")}`;
          return (
            <li key={key} data-expanded={expanded || undefined}>
              <time>{formatDate(event.occurred_at || event.created_at)}</time>
              <span className="activity-feed-event"><strong>{activityLabel(kind)}</strong><small>{kind}</small></span>
              <span className="activity-feed-actor">{actor}</span>
              <code title={eventReference(event)}>{shortID(eventReference(event), 14)}</code>
              <span className="activity-feed-data-cell">{hasData ? <button className="activity-feed-toggle" type="button" aria-expanded={expanded} aria-controls={detailsID} onClick={() => setExpandedEvent(expanded ? null : key)}>{expanded ? "Ocultar datos" : "Ver datos"}</button> : <span className="repository-muted">—</span>}</span>
              {expanded && data ? <div className="activity-feed-details" id={detailsID}><span>DATOS DEL EVENTO</span><pre>{JSON.stringify(data, null, 2)}</pre></div> : null}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
