import type { WorkspaceContext } from "@/api/types";
import { EmptyState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { text } from "@/lib/format";

type Item = Record<string, unknown>;

const lanes: Array<{ key: keyof WorkspaceContext; title: string; label: string; tone: "active" | "warning" | "info" | "neutral" }> = [
  { key: "decisions", title: "Decisiones", label: "Decisión", tone: "active" },
  { key: "requirements", title: "Requisitos", label: "Requisito", tone: "info" },
  { key: "constraints", title: "Restricciones", label: "Restricción", tone: "neutral" },
  { key: "open_questions", title: "Preguntas abiertas", label: "Pregunta", tone: "info" },
  { key: "risks", title: "Riesgos", label: "Riesgo", tone: "warning" },
  { key: "resources", title: "Fuentes", label: "Fuente", tone: "neutral" },
];

export function KnowledgeBoard({ context }: { context?: WorkspaceContext }) {
  const count = lanes.reduce((total, lane) => total + (Array.isArray(context?.[lane.key]) ? (context?.[lane.key] as Item[]).length : 0), 0);
  if (!context || !count) return <EmptyState title="No hay contexto durable" description="Las decisiones, requisitos, riesgos y fuentes que registren los agentes aparecerán aquí." />;
  return (
    <div className="knowledge-board">
      {lanes.map((lane) => {
        const items = Array.isArray(context[lane.key]) ? context[lane.key] as Item[] : [];
        if (!items.length) return null;
        return (
          <section className="knowledge-lane" key={lane.key}>
            <header><h2>{lane.title}</h2><span>{items.length}</span></header>
            <div>{items.map((item, index) => <article key={text(item.id, String(index))}><StatusChip tone={lane.tone}>{text(item.status, lane.label)}</StatusChip><h3>{text(item.title || item.name, `${lane.label} sin título`)}</h3><p>{text(item.summary || item.description || item.content, "Sin descripción")}</p></article>)}</div>
          </section>
        );
      })}
    </div>
  );
}
