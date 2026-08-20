import { Avatar } from "@/components/ui/Avatar";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { text } from "@/lib/format";

function records(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value as Array<Record<string, unknown>> : [];
}

function statusCopy(status: "idle" | "connecting" | "connected" | "reconnecting" | "offline"): string {
  if (status === "connected") return "SSE en vivo";
  if (status === "reconnecting") return "Reconectando SSE";
  if (status === "offline") return "SSE sin conexión";
  if (status === "connecting") return "Conectando SSE";
  return "SSE inactivo";
}

export function WorkspaceLiveStatus() {
  const { workspace, access, stream } = useWorkspace();
  if (!workspace) return null;
  const actors = new Map<string, { name: string; kind: "agent" | "person" }>();

  for (const item of [...records(access?.agents), ...records(access?.sessions)]) {
    const connected = Boolean(item.connected) || Number(item.active_sessions || 0) > 0 || item.session_status === "active";
    if (!connected) continue;
    const id = text(item.actor_id || item.agent_id || item.session_id || item.id);
    if (!id || actors.has(id)) continue;
    const kind = text(item.actor_kind || item.kind || item.agent_type).toLowerCase().includes("agent") || Boolean(item.agent_id) ? "agent" : "person";
    actors.set(id, { name: text(item.display_name || item.actor_name || item.name, "Actor"), kind });
  }
  const visibleActors = [...actors.values()].slice(0, 5);
  const tone = stream.status === "connected" ? "active" : stream.status === "offline" ? "offline" : "pending";

  return (
    <div className="workspace-live-status" data-tone={tone}>
      <span className="workspace-live-copy"><i aria-hidden="true" />{statusCopy(stream.status)}{actors.size ? ` · ${actors.size} ${actors.size === 1 ? "actor conectado" : "actores conectados"}` : ""}</span>
      {visibleActors.length ? (
        <span className="workspace-live-actors" aria-label={`${actors.size} actores conectados`}>
          {visibleActors.map((actor, index) => <Avatar key={`${actor.name}-${index}`} name={actor.name} kind={actor.kind} size="sm" decorative={false} />)}
        </span>
      ) : null}
    </div>
  );
}
