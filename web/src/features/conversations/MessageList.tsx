import type { RoomMessage } from "@/api/types";
import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { EmptyState, LoadingState } from "@/components/ui/States";
import { text } from "@/lib/format";

function messageTime(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("es", { hour: "2-digit", minute: "2-digit" }).format(date);
}

export function MessageList({ messages, loading, onReply }: { messages: RoomMessage[]; loading: boolean; onReply?: (message: RoomMessage) => void }) {
  if (loading) return <LoadingState label="Cargando mensajes" />;
  if (!messages.length) return <EmptyState title="La conversación está vacía" description="Escribe el primer mensaje para compartir contexto con el equipo." />;
  const byID = new Map(messages.map((message) => [message.id, message]));
  return (
    <ol className="message-list" aria-live="polite">
      {messages.map((message) => {
        const name = text(message.author_display_name || message.actor?.display_name || message.display_name || message.author_actor_id || message.actor_id, "Colaborador");
        const kind = text(message.author_kind || message.actor?.kind, "person");
        const agent = kind.toLowerCase().includes("agent");
        const replied = message.reply_to_message_id ? byID.get(message.reply_to_message_id) : undefined;
        const repliedName = replied ? text(replied.author_display_name || replied.actor?.display_name || replied.display_name || replied.author_actor_id || replied.actor_id, "Colaborador") : "Mensaje anterior";
        return <li key={message.id} id={`message-${message.id}`} tabIndex={-1}><Avatar name={name} kind={agent ? "agent" : "person"} size="sm" /><article><header><strong>{name}</strong><span className="message-kind">{agent ? "AGENTE" : "PERSONA"}</span><time>{messageTime(message.created_at)}</time>{onReply ? <Button className="message-reply-action" size="sm" variant="ghost" onClick={() => onReply(message)}>Responder</Button> : null}</header>{message.reply_to_message_id ? <div className="message-reply"><strong>{repliedName}</strong><span>{replied ? text(replied.body || replied.content, "Mensaje vacío") : `Respuesta a ${message.reply_to_message_id.slice(0, 8)}`}</span></div> : null}<p>{text(message.body || message.content, "Mensaje vacío")}</p></article></li>;
      })}
    </ol>
  );
}
