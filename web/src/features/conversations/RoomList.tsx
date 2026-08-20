import type { Room } from "@/api/types";
import { EmptyState } from "@/components/ui/States";

export function RoomList({ workspaceName, rooms, selectedRoomID, canCreate = true, onSelect, onCreate }: { workspaceName: string; rooms: Room[]; selectedRoomID?: string; canCreate?: boolean; onSelect: (room: Room) => void; onCreate: () => void }) {
  return (
    <aside className="room-list">
      <header><p>SALAS DE {workspaceName.toLocaleUpperCase("es")}</p>{canCreate ? <button type="button" aria-label="Crear conversación" title="Crear conversación" onClick={onCreate}>+</button> : null}</header>
      {rooms.length ? <nav aria-label="Conversaciones del workspace">{rooms.map((room) => <button key={room.id} type="button" className={room.id === selectedRoomID ? "room-list-item is-active" : "room-list-item"} aria-current={room.id === selectedRoomID ? "page" : undefined} onClick={() => onSelect(room)}><span>#</span><strong>{room.name}</strong>{room.message_count ? <em>{room.message_count}</em> : null}</button>)}</nav> : <EmptyState title="No hay conversaciones" description={canCreate ? "Crea la primera sala compartida del workspace." : "Aún no se ha creado ninguna sala compartida."} actionLabel={canCreate ? "Crear conversación" : undefined} onAction={canCreate ? onCreate : undefined} />}
      <footer>Las salas son conversaciones del equipo en {workspaceName}. Mencionar a un agente no lo ejecuta.</footer>
    </aside>
  );
}
