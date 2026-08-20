import { useEffect, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";

import type { Room, RoomMessage } from "@/api/types";
import { Page } from "@/components/layout/Page";
import { Button } from "@/components/ui/Button";
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/Dialog";
import { ErrorState, LoadingState } from "@/components/ui/States";
import { useToast } from "@/components/ui/Toast";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { useWorkspaceAccess } from "@/features/overview/queries";
import { currentProjectRole, roleAtLeast } from "@/lib/access";
import { canManage } from "@/lib/format";

import { MessageComposer } from "./MessageComposer";
import { MentionInbox } from "./MentionInbox";
import { MessageList } from "./MessageList";
import { RoomList } from "./RoomList";
import { useCreateRoom, useParticipants, usePostMessage, useRoomMessages, useRooms } from "./queries";

export function ConversationsPage() {
  const { workspace, workspaceProjects, principal } = useWorkspace();
  const [search, setSearch] = useSearchParams();
  const rooms = useRooms(workspace?.id);
  const participants = useParticipants(workspace?.id);
  const access = useWorkspaceAccess(workspaceProjects.map((item) => item.id));
  const [createOpen, setCreateOpen] = useState(false);
  const [reply, setReply] = useState<RoomMessage | null>(null);
  const selectedRoom = rooms.data?.find((room) => room.id === search.get("room")) || rooms.data?.find((room) => room.managed_default) || rooms.data?.[0];
  const messages = useRoomMessages(workspace?.id, selectedRoom?.id);
  const post = usePostMessage(workspace?.id || "", selectedRoom?.id || "");
  const { toast } = useToast();
  const currentRole = currentProjectRole(access.data, principal);
  const canCreateRooms = canManage(principal?.organization_role) || roleAtLeast(currentRole, "maintainer");
  const canSendMessages = canManage(principal?.organization_role) || roleAtLeast(currentRole, "contributor");

  useEffect(() => {
    if (selectedRoom && search.get("room") !== selectedRoom.id) {
      const next = new URLSearchParams(search);
      next.set("room", selectedRoom.id);
      setSearch(next, { replace: true });
    }
  }, [search, selectedRoom, setSearch]);

  useEffect(() => {
    const messageID = search.get("message");
    if (!messageID || !messages.data?.length) return;
    requestAnimationFrame(() => {
      const target = document.getElementById(`message-${messageID}`);
      target?.scrollIntoView({ block: "center" });
      target?.focus({ preventScroll: true });
    });
  }, [messages.data, search]);

  if (!workspace) return <ErrorState title="Workspace no encontrado" />;
  if (rooms.isPending || (workspaceProjects.length > 0 && access.isPending)) return <LoadingState label="Cargando conversaciones" />;

  const send = async (body: string, mentionActorIDs: string[]) => {
    try {
      await post.mutateAsync({ body, reply_to_message_id: reply?.id, mention_actor_ids: mentionActorIDs });
      setReply(null);
    } catch (error) {
      toast({ title: "No se pudo enviar el mensaje", description: (error as Error).message, tone: "danger" });
    }
  };

  return (
    <Page className="conversations-page" fullBleed title="Conversaciones" description={`Salas del equipo de ${workspace.name} · las menciones no ejecutan agentes`}>
      <div className="conversation-layout">
        <RoomList workspaceName={workspace.name} rooms={rooms.data || []} selectedRoomID={selectedRoom?.id} canCreate={canCreateRooms} onSelect={(room) => setSearch({ room: room.id })} onCreate={() => setCreateOpen(true)} />
        <section className="conversation-main">
          {selectedRoom ? <><header><span><h2>#{selectedRoom.name}</h2><p>{selectedRoom.description || `Conversación compartida de ${workspace.name}`} · {participants.data?.length || 0} participantes</p></span><div className="conversation-header-actions"><MentionInbox workspaceID={workspace.id} /><span className="conversation-participants">Participantes · {participants.data?.length || 0}</span></div></header><MessageList messages={messages.data || []} loading={messages.isPending} onReply={canSendMessages ? setReply : undefined} />{canSendMessages ? <MessageComposer participants={participants.data || []} reply={reply} sending={post.isPending} onCancelReply={() => setReply(null)} onSend={send} /> : <div className="conversation-read-only"><strong>Acceso de solo lectura</strong><span>Necesitas rol de colaborador para escribir en esta conversación.</span></div>}</> : <ErrorState title="Selecciona una conversación" description={canCreateRooms ? "Crea una sala para comenzar." : "Aún no hay conversaciones disponibles."} actionLabel={canCreateRooms ? "Crear conversación" : undefined} onAction={canCreateRooms ? () => setCreateOpen(true) : undefined} />}
        </section>
      </div>
      <CreateRoomDialog open={createOpen} onOpenChange={setCreateOpen} workspaceID={workspace.id} onCreated={(room) => setSearch({ room: room.id })} />
    </Page>
  );
}

function CreateRoomDialog({ open, onOpenChange, workspaceID, onCreated }: { open: boolean; onOpenChange: (open: boolean) => void; workspaceID: string; onCreated: (room: Room) => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const create = useCreateRoom(workspaceID);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const room = await create.mutateAsync({ name: name.trim(), description: description.trim() });
    onCreated(room); onOpenChange(false); setName(""); setDescription("");
  };
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><form onSubmit={submit}><DialogHeader><p className="pact-kicker">NUEVA SALA</p><DialogTitle>Crear conversación</DialogTitle></DialogHeader><DialogBody className="pact-form-stack"><label className="pact-field"><span>Nombre</span><input autoFocus required value={name} onChange={(event) => setName(event.target.value)} /></label><label className="pact-field"><span>Descripción</span><textarea rows={3} value={description} onChange={(event) => setDescription(event.target.value)} /></label></DialogBody><DialogFooter><Button variant="secondary" onClick={() => onOpenChange(false)}>Cancelar</Button><Button type="submit" loading={create.isPending}>Crear conversación</Button></DialogFooter></form></DialogContent></Dialog>;
}
