import { useEffect, useMemo, useState } from "react";

import type { AdminUser } from "@/api/types";
import { Page } from "@/components/layout/Page";
import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/Tabs";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { canManage, formatDate, roleLabel, text } from "@/lib/format";

import { InvitationsPanel } from "./InvitationsPanel";
import { useUserDirectory } from "./queries";
import { UserProfile } from "./UserProfile";

export function OrganizationAccessPage() {
  const { principal, projects } = useWorkspace(); const query = useUserDirectory(canManage(principal?.organization_role)); const [tab, setTab] = useState("users"); const [selectedID, setSelectedID] = useState(""); const [search, setSearch] = useState("");
  const users = query.data?.users || [];
  useEffect(() => { if (!users.some((user) => user.principal_id === selectedID)) setSelectedID(users.find((user) => user.principal_id === principal?.id)?.principal_id || users[0]?.principal_id || ""); }, [principal?.id, selectedID, users]);
  const filtered = useMemo(() => users.filter((user) => [user.display_name, user.email, user.username].some((value) => String(value || "").toLowerCase().includes(search.toLowerCase()))), [search, users]);
  if (!canManage(principal?.organization_role)) return <ErrorState title="No tienes acceso" description="Solo propietarios y administradores pueden gestionar acceso y seguridad." />;
  if (query.isPending) return <LoadingState label="Cargando acceso y seguridad" />;
  if (query.error) return <ErrorState title="No se pudo cargar la administración" description={(query.error as Error).message} actionLabel="Reintentar" onAction={() => void query.refetch()} />;
  const selected = users.find((user) => user.principal_id === selectedID);
  return <Page kicker="ORGANIZACIÓN" title="Acceso y seguridad" description="Identidades, permisos, invitaciones, sesiones y auditoría de toda la organización." actions={<Button variant="secondary" onClick={() => void query.refetch()}>Actualizar</Button>}><Tabs value={tab} onValueChange={setTab}><TabsList className="admin-tabs"><TabsTrigger value="users">Personas <span>{users.length}</span></TabsTrigger><TabsTrigger value="invitations">Invitaciones <span>{query.data?.invitations.length || 0}</span></TabsTrigger><TabsTrigger value="activity">Auditoría <span>{query.data?.events.length || 0}</span></TabsTrigger></TabsList><TabsContent value="users"><div className="admin-users-layout"><aside className="user-directory"><label className="workspace-sidebar-search"><span>⌕</span><input type="search" placeholder="Nombre, correo o usuario" value={search} onChange={(event) => setSearch(event.target.value)} /></label><nav>{filtered.map((user) => <UserButton key={user.principal_id} user={user} selected={user.principal_id === selectedID} onClick={() => setSelectedID(user.principal_id)} />)}</nav></aside>{selected ? <UserProfile user={selected} principal={principal} projects={projects} /> : <EmptyState title="Selecciona una persona" />}</div></TabsContent><TabsContent value="invitations"><InvitationsPanel invitations={query.data?.invitations || []} projects={projects} canInvite /></TabsContent><TabsContent value="activity"><Audit events={query.data?.events || []} /></TabsContent></Tabs></Page>;
}

function UserButton({ user, selected, onClick }: { user: AdminUser; selected: boolean; onClick: () => void }) { return <button type="button" className={selected ? "user-directory-item is-active" : "user-directory-item"} aria-current={selected ? "true" : undefined} onClick={onClick}><Avatar name={user.display_name || user.username || "Usuario"} size="sm" /><span><strong>{text(user.display_name)}</strong><small>{text(user.email || user.username)}</small></span><StatusChip tone={user.status === "active" ? "active" : "danger"}>{roleLabel(user.organization_role)}</StatusChip></button>; }

function Audit({ events }: { events: Array<Record<string, unknown>> }) { if (!events.length) return <EmptyState title="Aún no hay acciones administrativas" />; return <ol className="admin-audit-list">{events.map((event, index) => <li key={text(event.id, String(index))}><span className="activity-marker" /><article><strong>{text(event.actor_display_name, "PACT")} · {text(event.action, "acción administrativa")}</strong><p>{text(event.target_display_name || (event.details as Record<string, unknown> | undefined)?.email, "Acceso de la organización")}</p><time>{formatDate(event.created_at)}</time></article></li>)}</ol>; }
