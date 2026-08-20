import { useState } from "react";
import { useNavigate } from "react-router-dom";

import type { RoomMention, RoomMessage } from "@/api/types";
import { Button } from "@/components/ui/Button";
import { Dialog, DialogBody, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/Dialog";
import { Icon } from "@/components/ui/Icon";
import { EmptyState, LoadingState } from "@/components/ui/States";
import { relativeDate, text } from "@/lib/format";

import { useMentions, useUpdateMention } from "./queries";

type MentionWithMessage = RoomMention & { message?: RoomMessage };

export function MentionInbox({ workspaceID, variant = "button" }: { workspaceID?: string; variant?: "rail" | "button" }) {
  const [open, setOpen] = useState(false);
  const mentions = useMentions(workspaceID);
  const update = useUpdateMention(workspaceID);
  const navigate = useNavigate();
  const count = mentions.data?.length || 0;

  const openMention = (mention: MentionWithMessage) => {
    setOpen(false);
    navigate(`/w/${encodeURIComponent(mention.workspace_id)}/conversations?room=${encodeURIComponent(mention.room_id)}${mention.message?.id ? `&message=${encodeURIComponent(mention.message.id)}` : ""}`);
    if (mention.status === "pending") {
      update.mutate({ mentionID: mention.id, status: "read" });
    }
  };

  return (
    <>
      {variant === "rail" ? (
        <button className="global-mentions-button" type="button" aria-label={`Menciones pendientes: ${count}`} title="Menciones" onClick={() => setOpen(true)}>
          <Icon name="mentions" />{count ? <strong>{count > 99 ? "99+" : count}</strong> : null}
        </button>
      ) : (
        <Button variant="secondary" size="sm" onClick={() => setOpen(true)}>Menciones{count ? ` · ${count}` : ""}</Button>
      )}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="mention-inbox-dialog">
          <DialogHeader><p className="pact-kicker">PARA TI</p><DialogTitle>{workspaceID ? "Menciones de este workspace" : "Menciones pendientes"}</DialogTitle></DialogHeader>
          <DialogBody>
            {mentions.isPending ? <LoadingState label="Cargando menciones" /> : count ? (
              <ol className="mention-inbox-list">
                {(mentions.data as MentionWithMessage[]).map((mention) => (
                  <li key={mention.id}>
                    <button type="button" onClick={() => openMention(mention)}>
                      <span><strong>#{text(mention.room_name, "conversación")}</strong><time>{relativeDate(mention.created_at)}</time></span>
                      <p>{text(mention.message?.body || mention.message_excerpt, "Te mencionaron en una conversación.")}</p>
                      <small>Por {text(mention.message?.author_display_name, "un colaborador")}</small>
                    </button>
                    <Button variant="ghost" size="sm" loading={update.isPending && update.variables?.mentionID === mention.id} onClick={() => update.mutate({ mentionID: mention.id, status: "dismissed" })}>Descartar</Button>
                  </li>
                ))}
              </ol>
            ) : <EmptyState title="No tienes menciones pendientes" description="Las solicitudes de personas y agentes aparecerán aquí." />}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}
