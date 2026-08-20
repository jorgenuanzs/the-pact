import { useEffect, useState, type FormEvent } from "react";

import type { AdminUser, Principal, ProjectSummary } from "@/api/types";
import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { Panel, PanelBody, PanelFooter, PanelHeader, PanelTitle } from "@/components/ui/Panel";
import { StatusChip } from "@/components/ui/StatusChip";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/Tabs";
import { useToast } from "@/components/ui/Toast";
import { requestData } from "@/api/client";
import { roleLabel, text } from "@/lib/format";

import { useRevokeSessions, useSetProjectRole, useToggleUser, useUpdateUser } from "./queries";

function canManageUser(principal: Principal | undefined, user: AdminUser): boolean {
  if (principal?.organization_role === "owner") return true;
  return principal?.organization_role === "admin" && user.organization_role === "member";
}

export function UserProfile({ user, principal, projects }: { user: AdminUser; principal?: Principal; projects: ProjectSummary[] }) {
  const [tab, setTab] = useState("profile");
  const current = user.principal_id === principal?.id;
  const manageable = canManageUser(principal, user);
  return <Panel className="user-detail"><PanelHeader className="user-detail-header"><span className="identity-cell"><Avatar name={user.display_name || user.username || "Usuario"} size="lg" /><span><PanelTitle>{text(user.display_name)}</PanelTitle><small>{text(user.email || user.username)}</small></span></span><StatusChip tone={user.status === "active" ? "active" : "danger"}>{user.status === "active" ? "Activo" : "Desactivado"}</StatusChip></PanelHeader><Tabs value={tab} onValueChange={setTab}><TabsList><TabsTrigger value="profile">Perfil</TabsTrigger><TabsTrigger value="permissions">Permisos</TabsTrigger><TabsTrigger value="security">Seguridad</TabsTrigger></TabsList><TabsContent value="profile"><ProfileForm user={user} manageable={manageable} current={current} principal={principal} /></TabsContent><TabsContent value="permissions"><Permissions user={user} manageable={manageable} projects={projects} /></TabsContent><TabsContent value="security"><Security user={user} manageable={manageable} current={current} /></TabsContent></Tabs></Panel>;
}

function ProfileForm({ user, manageable, current, principal }: { user: AdminUser; manageable: boolean; current: boolean; principal?: Principal }) {
  const [displayName, setDisplayName] = useState(user.display_name || ""); const [email, setEmail] = useState(user.email || ""); const [username, setUsername] = useState(user.username || ""); const [role, setRole] = useState(user.organization_role || "member");
  const mutation = useUpdateUser(); const { toast } = useToast();
  useEffect(() => { setDisplayName(user.display_name || ""); setEmail(user.email || ""); setUsername(user.username || ""); setRole(user.organization_role || "member"); }, [user]);
  const editable = manageable && !(current && principal?.organization_role === "admin");
  const submit = async (event: FormEvent) => { event.preventDefault(); try { await mutation.mutateAsync({ principalID: user.principal_id, display_name: displayName.trim(), email: email.trim(), username: username.trim(), ...(principal?.organization_role === "owner" && !current ? { organization_role: role } : {}) }); toast({ title: "Usuario actualizado", tone: "success" }); } catch (error) { toast({ title: "No se pudo actualizar", description: (error as Error).message, tone: "danger" }); } };
  return <form onSubmit={submit}><PanelBody className="pact-form-grid"><label className="pact-field"><span>Nombre visible</span><input disabled={!editable} value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label><label className="pact-field"><span>Correo</span><input type="email" disabled={!editable} value={email} onChange={(event) => setEmail(event.target.value)} /></label><label className="pact-field"><span>Usuario</span><input disabled={!editable} value={username} onChange={(event) => setUsername(event.target.value)} /></label><label className="pact-field"><span>Rol de organización</span><select disabled={!manageable || current || principal?.organization_role !== "owner"} value={role} onChange={(event) => setRole(event.target.value)}><option value="member">Miembro</option><option value="admin">Administrador</option><option value="owner">Propietario</option></select></label></PanelBody><PanelFooter><small>{editable ? "Los cambios quedan registrados en auditoría." : "Tu rol no permite modificar esta cuenta."}</small><Button type="submit" loading={mutation.isPending} disabled={!editable}>Guardar perfil</Button></PanelFooter></form>;
}

function projectRoles(user: AdminUser): Map<string, string> {
  const roles = new Map<string, string>();
  if (Array.isArray(user.project_roles)) for (const item of user.project_roles) { const value = item as Record<string, unknown>; if (value.project_id) roles.set(String(value.project_id), String(value.role || "")); }
  else if (user.project_roles) for (const [id, role] of Object.entries(user.project_roles)) roles.set(id, String(role));
  return roles;
}

function Permissions({ user, manageable, projects }: { user: AdminUser; manageable: boolean; projects: ProjectSummary[] }) {
  const mutation = useSetProjectRole(); const roles = projectRoles(user); const global = user.organization_role === "owner" || user.organization_role === "admin";
  return <PanelBody><p className="section-copy">{global ? `El rol ${roleLabel(user.organization_role)} tiene acceso global.` : "Asigna únicamente los workspaces que esta persona necesita."}</p><div className="permission-list">{projects.map((project) => <label key={project.id}><span><strong>{project.name}</strong><small>{project.slug}</small></span><select aria-label={`Permiso en ${project.name}`} disabled={!manageable || global || user.status !== "active" || mutation.isPending} value={global ? "global" : roles.get(project.id) || ""} onChange={(event) => mutation.mutate({ principalID: user.principal_id, projectID: project.id, role: event.target.value || undefined })}>{global ? <option value="global">Acceso global</option> : <><option value="">Sin acceso</option><option value="viewer">Observador</option><option value="contributor">Colaborador</option><option value="maintainer">Responsable</option><option value="owner">Propietario</option></>}</select></label>)}</div></PanelBody>;
}

function Security({ user, manageable, current }: { user: AdminUser; manageable: boolean; current: boolean }) {
  const [confirm, setConfirm] = useState<"sessions" | "status" | null>(null); const revoke = useRevokeSessions(); const toggle = useToggleUser(); const { toast } = useToast();
  const act = async () => { try { if (confirm === "sessions") await revoke.mutateAsync(user.principal_id); if (confirm === "status") await toggle.mutateAsync({ principalID: user.principal_id, disabled: user.status === "active" }); toast({ title: confirm === "sessions" ? "Sesiones revocadas" : "Estado actualizado", tone: "success" }); setConfirm(null); } catch (error) { toast({ title: "No se pudo completar la acción", description: (error as Error).message, tone: "danger" }); } };
  return <PanelBody className="security-stack"><dl className="metric-strip compact"><div><dt>Sesiones web</dt><dd>{text(user.active_sessions, "0")}</dd></div><div><dt>Dispositivos</dt><dd>{text(user.active_devices, "0")}</dd></div><div><dt>Último acceso</dt><dd>{text(user.last_login_at, "Nunca")}</dd></div></dl><div className="danger-actions"><Button variant="secondary" disabled={!manageable || current} onClick={() => setConfirm("sessions")}>Revocar sesiones y dispositivos</Button><Button variant="danger" disabled={!manageable || current} onClick={() => setConfirm("status")}>{user.status === "active" ? "Desactivar usuario" : "Reactivar usuario"}</Button></div>{current ? <ChangePassword /> : null}<ConfirmDialog open={Boolean(confirm)} title={confirm === "sessions" ? "Revocar accesos" : user.status === "active" ? "Desactivar usuario" : "Reactivar usuario"} description={confirm === "sessions" ? `Se cerrarán todas las sesiones y dispositivos de ${user.display_name}.` : `Se cambiará el estado de ${user.display_name}.`} confirmLabel="Continuar" destructive onConfirm={() => void act()} busy={revoke.isPending || toggle.isPending} onOpenChange={(open) => { if (!open) setConfirm(null); }} /></PanelBody>;
}

function ChangePassword() {
  const [current, setCurrent] = useState(""); const [next, setNext] = useState(""); const [busy, setBusy] = useState(false); const { toast } = useToast();
  const submit = async (event: FormEvent) => { event.preventDefault(); if (next.length < 15) { toast({ title: "La nueva contraseña debe tener al menos 15 caracteres", tone: "warning" }); return; } setBusy(true); try { await requestData("/v1/auth/password", { method: "PUT", body: { current_password: current, new_password: next } }); setCurrent(""); setNext(""); toast({ title: "Contraseña actualizada", tone: "success" }); } catch (error) { toast({ title: "No se pudo cambiar la contraseña", description: (error as Error).message, tone: "danger" }); } finally { setBusy(false); } };
  return <form className="password-form" onSubmit={submit}><h3>Cambiar mi contraseña</h3><label className="pact-field"><span>Contraseña actual</span><input type="password" autoComplete="current-password" value={current} onChange={(event) => setCurrent(event.target.value)} /></label><label className="pact-field"><span>Contraseña nueva</span><input type="password" autoComplete="new-password" value={next} onChange={(event) => setNext(event.target.value)} /></label><Button type="submit" loading={busy}>Cambiar contraseña</Button></form>;
}
