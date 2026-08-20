import { StatusChip } from "@/components/ui/StatusChip";
import { relativeDate, text } from "@/lib/format";

const stateCopy: Record<string, { label: string; tone: "active" | "warning" | "neutral" }> = {
  editing: { label: "Editando ahora", tone: "active" },
  recent: { label: "Cambios recientes", tone: "warning" },
  idle: { label: "Sin cambios recientes", tone: "neutral" },
  unobserved: { label: "Sin observador", tone: "neutral" },
};

const reasonCopy: Record<string, string> = {
  no_connected_observer: "No hay un observador conectado al código.",
  observer_connected_no_recent_change: "El observador está conectado y no ha detectado cambios recientes.",
  fresh_workspace_diff: "Hay cambios locales activos en el workspace.",
  fresh_external_git_change: "Se detectó un cambio reciente desde Git.",
  recent_workspace_diff: "Hubo cambios locales hace poco.",
  recent_external_git_change: "Hubo cambios externos en Git hace poco.",
  recent_changeset: "Se observó un changeset recientemente.",
};

export function CodeActivityStatus({ activity }: { activity?: Record<string, unknown> }) {
  const state = text(activity?.state, "unobserved");
  const copy = stateCopy[state] || stateCopy.unobserved;
  return (
    <div className="code-activity-status">
      <span><StatusChip tone={copy.tone}>{copy.label}</StatusChip><small>{text(activity?.observer_count, "0")} observadores conectados</small></span>
      <p>{reasonCopy[text(activity?.reason)] || "PACT todavía no tiene evidencia suficiente sobre la actividad del código."}</p>
      {activity?.observed_at ? <time>Última evidencia {relativeDate(activity.observed_at)}</time> : null}
    </div>
  );
}
