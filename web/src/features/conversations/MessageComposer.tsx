import { useMemo, useState, type FormEvent } from "react";

import type { Actor, RoomMessage } from "@/api/types";
import { Button } from "@/components/ui/Button";
import { text } from "@/lib/format";

export function MessageComposer({ participants, reply, sending, onCancelReply, onSend }: { participants: Actor[]; reply: RoomMessage | null; sending: boolean; onCancelReply: () => void; onSend: (body: string, mentions: string[]) => Promise<void> }) {
  const [body, setBody] = useState("");
  const suggestions = useMemo(() => {
    const match = body.match(/(?:^|\s)@([\w.-]*)$/);
    if (!match) return [];
    const query = match[1].toLowerCase();
    return participants.filter((participant) => text(participant.handle || participant.display_name || participant.actor_id, "").toLowerCase().includes(query)).slice(0, 6);
  }, [body, participants]);

  const selectMention = (participant: Actor) => {
    const handle = text(participant.handle || participant.display_name || participant.actor_id, "actor").replace(/^@/, "").replace(/\s+/g, "-");
    setBody((current) => current.replace(/@([\w.-]*)$/, `@${handle} `));
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!body.trim()) return;
    const mentions = participants.filter((participant) => {
      const handle = text(participant.handle || participant.display_name || participant.actor_id, "").replace(/^@/, "").replace(/\s+/g, "-");
      return handle && body.includes(`@${handle}`);
    }).map((participant) => text(participant.actor_id || participant.id, "")).filter(Boolean);
    await onSend(body.trim(), mentions);
    setBody("");
  };

  return (
    <div className="message-composer-shell">
      <form className="message-composer" onSubmit={submit}>
        {reply ? <div className="message-composer-reply"><span>Respondiendo a <strong>{text(reply.author_display_name || reply.actor?.display_name || reply.display_name || reply.author_actor_id || reply.actor_id)}</strong></span><Button size="sm" variant="ghost" onClick={onCancelReply}>Cancelar</Button></div> : null}
        <textarea aria-label="Mensaje" placeholder="Escribe un mensaje o usa @ para mencionar a una persona o agente…" rows={2} value={body} onChange={(event) => setBody(event.target.value)} />
        {suggestions.length ? <div className="mention-suggestions" role="listbox" aria-label="Mencionar participante">{suggestions.map((participant) => <button type="button" role="option" key={text(participant.actor_id || participant.id)} onClick={() => selectMention(participant)}>@{text(participant.handle || participant.display_name || participant.actor_id)}</button>)}</div> : null}
        <footer><span className="composer-mention-hint">@ Mencionar</span><small>Solo se notifica a quien menciones explícitamente.</small><Button size="sm" type="submit" loading={sending}>Enviar</Button></footer>
      </form>
    </div>
  );
}
