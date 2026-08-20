import { Avatar } from "@/components/ui/Avatar";
import { EmptyState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { relativeDate, text } from "@/lib/format";

type Handoff = Record<string, unknown>;

function stringList(value: unknown): string[] {
  return Array.isArray(value) ? value.map(String).filter(Boolean) : [];
}

function tone(status: string): "active" | "warning" | "neutral" | "danger" {
  if (status === "accepted") return "active";
  if (status === "offered") return "warning";
  if (status === "expired") return "danger";
  return "neutral";
}

export function HandoffList({ handoffs }: { handoffs: Handoff[] }) {
  if (!handoffs.length) {
    return <EmptyState title="No hay handoffs pendientes" description="Los relevos estructurados aparecerán aquí con lo completado, bloqueos y próximos pasos." />;
  }

  return (
    <ol className="handoff-list">
      {handoffs.map((handoff, index) => {
        const id = text(handoff.id, String(index));
        const from = text(handoff.from_actor_name, "Colaborador");
        const to = text(handoff.to_actor_name, "Sin destinatario");
        const status = text(handoff.status, "offered");
        const nextSteps = stringList(handoff.next_steps);
        const blockers = stringList(handoff.blockers);
        return (
          <li key={id}>
            <header>
              <span className="handoff-identity"><Avatar name={from} size="sm" /><span><strong>{from} → {to}</strong><small>{relativeDate(handoff.offered_at || handoff.created_at)}</small></span></span>
              <StatusChip tone={tone(status)}>{status === "offered" ? "Ofrecido" : status === "accepted" ? "Aceptado" : status === "expired" ? "Vencido" : status}</StatusChip>
            </header>
            <p>{text(handoff.summary, "Handoff sin resumen")}</p>
            {blockers.length ? <div><strong>Bloqueos</strong><ul>{blockers.map((item) => <li key={item}>{item}</li>)}</ul></div> : null}
            {nextSteps.length ? <div><strong>Próximos pasos</strong><ul>{nextSteps.map((item) => <li key={item}>{item}</li>)}</ul></div> : null}
          </li>
        );
      })}
    </ol>
  );
}
