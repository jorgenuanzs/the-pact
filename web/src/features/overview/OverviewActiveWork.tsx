import { Link } from "react-router-dom";

import { Avatar } from "@/components/ui/Avatar";
import { StatusChip, type StatusTone } from "@/components/ui/StatusChip";
import { relativeDate, text } from "@/lib/format";

type WorkItem = Record<string, unknown>;

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" ? value as Record<string, unknown> : {};
}

function statusTone(status: string): StatusTone {
  if (status === "active" || status === "completed") return "active";
  if (status === "blocked") return "warning";
  if (status === "cancelled" || status === "abandoned") return "danger";
  return "neutral";
}

function statusLabel(status: string): string {
  return ({
    active: "Activo",
    blocked: "Bloqueado",
    completed: "Completado",
    cancelled: "Cancelado",
    abandoned: "Abandonado",
  } as Record<string, string>)[status] || text(status, "Sin estado");
}

function scopeLabel(item: WorkItem): string {
  const scopes = Array.isArray(item.scopes) ? item.scopes as Array<Record<string, unknown>> : [];
  const firstResource = record(scopes[0]?.resource);
  const workspace = record(item.workspace);
  return text(firstResource.locator || workspace.git_branch || item.branch, "Sin scope declarado");
}

export function OverviewActiveWork({ items }: { items: WorkItem[] }) {
  return (
    <section className="overview-active-work" id="overview-active-work">
      <header className="overview-section-heading">
        <span><p className="pact-kicker">AHORA</p><h2>Trabajo activo</h2></span>
        <Link to="live">Ver trabajo en vivo →</Link>
      </header>
      {items.length ? (
        <ol className="overview-work-list">
          {items.slice(0, 5).map((item, index) => {
            const intent = record(item.intent);
            const actorName = text(item.responsible_name || item.actor_name || record(item.actor).display_name || item.actor_id, "Actor desconocido");
            const status = text(intent.status || item.status, "unknown");
            return (
              <li key={text(intent.id || item.id, String(index))} data-tone={status === "blocked" ? "warning" : undefined}>
                <span className="overview-work-actor">
                  <Avatar name={actorName} kind={text(item.actor_kind).toLowerCase().includes("agent") ? "agent" : "person"} size="sm" />
                  <strong>{actorName}</strong>
                </span>
                <span className="overview-work-objective">
                  <strong>{text(intent.title || item.objective, "Sin objetivo")}</strong>
                  <code>{scopeLabel(item)}</code>
                </span>
                <StatusChip tone={statusTone(status)}>{statusLabel(status)}</StatusChip>
                <time>{relativeDate(item.session_last_seen_at || item.heartbeat_at || item.last_seen_at)}</time>
              </li>
            );
          })}
        </ol>
      ) : (
        <div className="overview-work-empty">
          <strong>No hay trabajo declarado</strong>
          <span>Los nuevos intents aparecerán aquí.</span>
          <Link to="live">Abrir trabajo en vivo →</Link>
        </div>
      )}
    </section>
  );
}
