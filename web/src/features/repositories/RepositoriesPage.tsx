import { useMemo, useState, type FormEvent, type KeyboardEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import type { GitHubStatus, Repository } from "@/api/types";
import { Page } from "@/components/layout/Page";
import { Button } from "@/components/ui/Button";
import { DataTable, DataTableBody, DataTableCell, DataTableHead, DataTableHeaderCell, DataTableRow } from "@/components/ui/DataTable";
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/Dialog";
import { EmptyState, ErrorState, LoadingState } from "@/components/ui/States";
import { StatusChip } from "@/components/ui/StatusChip";
import { useToast } from "@/components/ui/Toast";
import { useProjectAccess, useWorkspaceOverview } from "@/features/overview/queries";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { currentProjectRole, roleAtLeast } from "@/lib/access";
import { canManage, relativeDate, shortID, text } from "@/lib/format";

import { useAttachRepository, useAvailableRepositories, useConnectGitHub, useSyncRepository, useWorkspaceRepositories } from "./queries";

type RecordValue = Record<string, unknown>;

function records(value: unknown): RecordValue[] {
  return Array.isArray(value) ? value as RecordValue[] : [];
}

function githubConnection(status?: GitHubStatus): { connected: boolean; label: string; accounts: string[]; repositoryCount: number } {
  const installations = records(status?.installations);
  const active = installations.filter((item) => item.status === "active");
  const accounts = active.map((item) => text(item.account_login)).filter(Boolean);
  const repositoryCount = active.reduce((total, item) => total + Number(item.repository_count || 0), Number(status?.repositories || 0));
  if (!status?.configured) return { connected: false, label: "GitHub App no configurada", accounts: [], repositoryCount: 0 };
  if (active.length) return { connected: true, label: "Instalación activa", accounts, repositoryCount };
  return { connected: false, label: installations.length ? "Instalación suspendida" : "GitHub sin conectar", accounts: [], repositoryCount: 0 };
}

function repositoryName(repository: Repository): string {
  return text(repository.github_full_name || repository.full_name || repository.name, "Repositorio");
}

function repositoryRole(repository: Repository): string {
  if (repository.primary) return "Principal";
  if (repository.required) return "Requerido";
  return "Opcional";
}

function intentFrom(item: RecordValue): RecordValue {
  return item.intent && typeof item.intent === "object" ? item.intent as RecordValue : item;
}

function workspaceFrom(item: RecordValue): RecordValue {
  return item.workspace && typeof item.workspace === "object" ? item.workspace as RecordValue : {};
}

function scopeFrom(value: RecordValue): RecordValue {
  return value.resource && typeof value.resource === "object" ? value.resource as RecordValue : value;
}

function workForRepository(items: RecordValue[], repository: Repository): RecordValue[] {
  return items.filter((item) => {
    const intent = intentFrom(item);
    const workspace = workspaceFrom(item);
    const scopes = records(item.scopes);
    if (text(workspace.repository_id) === repository.id || text(intent.repository_id) === repository.id) return true;
    if (scopes.some((scope) => text(scopeFrom(scope).repository_id) === repository.id)) return true;
    return Boolean(repository.primary && text(intent.project_id) === text(repository.project_id));
  });
}

function actorNames(items: RecordValue[]): string[] {
  return [...new Set(items.map((item) => text(item.responsible_name || intentFrom(item).responsible_name)).filter(Boolean))];
}

function syncTone(status?: string): "active" | "danger" | "warning" {
  return status === "synced" ? "active" : status === "failed" ? "danger" : "warning";
}

function syncLabel(repository: Repository): string {
  if (repository.sync_status === "synced") return `Sincronizado · ${relativeDate(repository.synced_at || repository.last_success_at)}`;
  if (repository.sync_status === "failed") return "Error de sincronización";
  return `Sin verificar${repository.last_success_at ? ` · ${relativeDate(repository.last_success_at)}` : ""}`;
}

export function RepositoriesPage() {
  const { workspace, project, workspaceProjects, github, principal } = useWorkspace();
  const navigate = useNavigate();
  const projectIDs = workspaceProjects.map((item) => item.id);
  const repositories = useWorkspaceRepositories(projectIDs);
  const overview = useWorkspaceOverview(projectIDs);
  const access = useProjectAccess(project?.id);
  const connection = githubConnection(github);
  const available = useAvailableRepositories(project?.id, connection.connected);
  const connect = useConnectGitHub();
  const [attachOpen, setAttachOpen] = useState(false);
  const canMaintain = canManage(principal?.organization_role) || roleAtLeast(currentProjectRole(access.data, principal), "maintainer");

  if (!workspace) return <ErrorState title="Workspace no encontrado" />;
  if (!project) return <Page title="Repositorios" description="Vincula código al workspace."><EmptyState title="PACT aún no preparó el workspace operativo" description="Inicializa PACT desde un repositorio para vincular código a este workspace." /></Page>;
  if (repositories.isPending) return <LoadingState label="Cargando repositorios" />;

  const items = records(overview.data.work_items);
  const accountCopy = connection.accounts.length ? `organización ${connection.accounts.join(", ")}` : "ninguna cuenta autorizada";

  return (
    <Page title="Repositorios" description={`Los repositorios técnicos de ${workspace.name}, vinculados desde la instalación de GitHub.`}>
      <section className="github-connection-strip" aria-label="Conexión con GitHub">
        <span className="pact-section-label">GITHUB APP</span>
        <p>{connection.connected ? <>Conectada a <strong>{accountCopy}</strong>{connection.repositoryCount ? ` · ${connection.repositoryCount} repositorios autorizados` : ""}</> : connection.label}</p>
        <span className="github-installation-state" data-active={connection.connected || undefined}><i />{connection.label}</span>
        {connection.connected && canMaintain ? <Button variant="secondary" size="sm" onClick={() => setAttachOpen(true)}>Vincular repositorio</Button> : <Button variant="secondary" size="sm" disabled={!github?.configured || !canManage(principal?.organization_role)} loading={connect.isPending} onClick={() => connect.mutate()}>Conectar GitHub</Button>}
      </section>

      {repositories.data?.length ? (
        <>
          <RepositoryList
            repositories={repositories.data}
            workItems={items}
            onOpen={(repository) => navigate(encodeURIComponent(repository.id))}
          />
          <p className="repository-list-note">Cada repositorio es una parte técnica de {workspace.name}. Selecciona uno para revisar su trabajo activo, scopes y estado Git.</p>
        </>
      ) : (
        <div className="repository-empty"><EmptyState title="No hay repositorios vinculados" description="Conecta GitHub y vincula el código necesario para este workspace." actionLabel={connection.connected && canMaintain ? "Vincular repositorio" : undefined} onAction={connection.connected && canMaintain ? () => setAttachOpen(true) : undefined} /></div>
      )}

      <AttachRepositoryDialog open={attachOpen} onOpenChange={setAttachOpen} projectID={project.id} repositories={available.data || []} />
    </Page>
  );
}

function RepositoryList({ repositories, workItems, onOpen }: { repositories: Repository[]; workItems: RecordValue[]; onOpen: (repository: Repository) => void }) {
  const openFromKeyboard = (event: KeyboardEvent<HTMLTableRowElement>, repository: Repository) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    onOpen(repository);
  };

  return (
    <DataTable containerClassName="repository-table-wrap" className="repository-list-table" aria-label="Repositorios vinculados">
      <DataTableHead><tr><DataTableHeaderCell>Repositorio</DataTableHeaderCell><DataTableHeaderCell>Rol · propósito</DataTableHeaderCell><DataTableHeaderCell>Rama · commit</DataTableHeaderCell><DataTableHeaderCell>Trabajo activo</DataTableHeaderCell><DataTableHeaderCell>Sincronización</DataTableHeaderCell></tr></DataTableHead>
      <DataTableBody>{repositories.map((repository) => {
        const work = workForRepository(workItems, repository);
        const actors = actorNames(work);
        return (
          <DataTableRow key={repository.id} className="repository-list-row" tabIndex={0} role="link" aria-label={`Abrir ${repositoryName(repository)}`} onClick={() => onOpen(repository)} onKeyDown={(event) => openFromKeyboard(event, repository)}>
            <DataTableCell><strong className="repository-name">{repositoryName(repository)}</strong><small className="table-detail">{text((repository as Repository).description, repository.purpose ? `Código de ${repository.purpose}` : "Repositorio del workspace")}</small></DataTableCell>
            <DataTableCell><StatusChip tone={repository.primary ? "active" : "neutral"}>{repositoryRole(repository)}{repository.purpose ? ` · ${repository.purpose}` : ""}</StatusChip></DataTableCell>
            <DataTableCell><code>{text(repository.default_branch, "—")}</code><small className="table-detail pact-mono">{shortID(repository.canonical_revision, 12)}</small></DataTableCell>
            <DataTableCell>{work.length ? <><span>{work.length} {work.length === 1 ? "intent" : "intents"}{actors.length ? ` · ${actors.join(", ")}` : ""}</span>{work.some((item) => intentFrom(item).status === "blocked") ? <small className="table-detail repository-work-warning">Hay trabajo bloqueado</small> : null}</> : <span className="repository-muted">Sin trabajo declarado</span>}</DataTableCell>
            <DataTableCell><span className="repository-sync" data-tone={syncTone(repository.sync_status)}><i />{syncLabel(repository)}</span></DataTableCell>
          </DataTableRow>
        );
      })}</DataTableBody>
    </DataTable>
  );
}

export function RepositoryDetailPage() {
  const { repositoryId } = useParams();
  const { workspace, project, workspaceProjects, principal, github } = useWorkspace();
  const projectIDs = workspaceProjects.map((item) => item.id);
  const repositories = useWorkspaceRepositories(projectIDs);
  const overview = useWorkspaceOverview(projectIDs);
  const access = useProjectAccess(project?.id);
  const canMaintain = canManage(principal?.organization_role) || roleAtLeast(currentProjectRole(access.data, principal), "maintainer");
  const repository = repositories.data?.find((item) => item.id === repositoryId);

  if (!workspace) return <ErrorState title="Workspace no encontrado" />;
  if (repositories.isPending) return <LoadingState label="Cargando repositorio" />;
  if (!repository) return <Page title="Repositorio no encontrado" description="Puede que ya no esté vinculado a este workspace."><p className="repository-back-link"><Link to=".." relative="path">← Repositorios de {workspace.name}</Link></p></Page>;

  const items = workForRepository(records(overview.data.work_items), repository);
  const scopes = items.flatMap((item) => records(item.scopes).map((scope) => ({ scope, item })));
  const projectID = text(repository.project_id, project?.id);
  const connection = githubConnection(github);

  return (
    <Page title={repositoryName(repository)} description={`${text((repository as Repository).description, repository.purpose ? `Código de ${repository.purpose}` : "Repositorio técnico")} · parte de ${workspace.name}`}>
      <div className="repository-detail-toolbar"><Link to=".." relative="path">← Repositorios de {workspace.name}</Link><span>{repositoryRole(repository)}{repository.purpose ? ` · ${repository.purpose}` : ""}</span></div>

      <dl className="repository-facts-strip">
        <div><dt>Rama canónica</dt><dd><code>{text(repository.default_branch, "—")} · {shortID(repository.canonical_revision, 12)}</code></dd></div>
        <div><dt>Sincronización</dt><dd><span className="repository-sync" data-tone={syncTone(repository.sync_status)}><i />{syncLabel(repository)}</span></dd></div>
        <div><dt>Instalación</dt><dd>GitHub · {connection.accounts.join(", ") || "sin cuenta"}</dd></div>
        <div><dt>Trabajo activo</dt><dd>{items.length} {items.length === 1 ? "intent" : "intents"}{actorNames(items).length ? ` · ${actorNames(items).join(", ")}` : ""}</dd></div>
      </dl>

      <div className="repository-detail-grid">
        <main>
          <RepositoryIntentTable items={items} />
          <RepositoryScopeTable scopes={scopes} />
        </main>
        <RepositoryGitState repository={repository} projectID={projectID} editable={canMaintain} workItems={items} />
      </div>
    </Page>
  );
}

function RepositoryIntentTable({ items }: { items: RecordValue[] }) {
  return (
    <section className="repository-detail-section">
      <h2>Intents sobre este repositorio</h2>
      {items.length ? <DataTable containerClassName="repository-detail-table-wrap" aria-label="Intents del repositorio"><DataTableHead><tr><DataTableHeaderCell>Actor</DataTableHeaderCell><DataTableHeaderCell>Objetivo</DataTableHeaderCell><DataTableHeaderCell>Scope</DataTableHeaderCell><DataTableHeaderCell>Estado</DataTableHeaderCell><DataTableHeaderCell>Latido</DataTableHeaderCell></tr></DataTableHead><DataTableBody>{items.map((item, index) => {
        const intent = intentFrom(item);
        const firstScope = scopeFrom(records(item.scopes)[0] || {});
        const status = text(intent.status, "unknown");
        return <DataTableRow key={text(intent.id, String(index))}><DataTableCell><strong>{text(item.responsible_name, "Sin responsable")}</strong></DataTableCell><DataTableCell>{text(intent.title || intent.goal, "Sin objetivo")}</DataTableCell><DataTableCell><code>{text(firstScope.locator, "—")}</code></DataTableCell><DataTableCell><StatusChip tone={status === "active" ? "active" : status === "blocked" ? "warning" : "neutral"}>{status === "active" ? "Activo" : status === "blocked" ? "Bloqueado" : status}</StatusChip></DataTableCell><DataTableCell><span className="pact-mono">{relativeDate(item.session_last_seen_at || intent.updated_at)}</span></DataTableCell></DataTableRow>;
      })}</DataTableBody></DataTable> : <p className="repository-section-empty">No hay intents declarados sobre este repositorio.</p>}
    </section>
  );
}

function RepositoryScopeTable({ scopes }: { scopes: Array<{ scope: RecordValue; item: RecordValue }> }) {
  return (
    <section className="repository-detail-section">
      <h2>Scopes reservados</h2>
      {scopes.length ? <ul className="repository-scope-list">{scopes.map(({ scope, item }, index) => {
        const resource = scopeFrom(scope);
        return <li key={text(scope.id, String(index))}><code>{text(resource.locator, "Scope sin ruta")}</code><span>{text(item.responsible_name, "Sin responsable")}</span><StatusChip tone={scope.status === "active" ? "active" : "neutral"}>{text(scope.mode, text(scope.status, "Reservado"))}</StatusChip><time>{scope.expires_at ? relativeDate(scope.expires_at) : "—"}</time></li>;
      })}</ul> : <p className="repository-section-empty">No hay scopes reservados.</p>}
    </section>
  );
}

function RepositoryGitState({ repository, projectID, editable, workItems }: { repository: Repository; projectID: string; editable: boolean; workItems: RecordValue[] }) {
  const sync = useSyncRepository();
  const { toast } = useToast();
  const worktrees = workItems.filter((item) => Object.keys(workspaceFrom(item)).length > 0);
  const verify = async () => {
    try { await sync.mutateAsync({ projectID, repositoryID: repository.id }); toast({ title: "Repositorio verificado", tone: "success" }); }
    catch (error) { toast({ title: "No se pudo verificar el repositorio", description: (error as Error).message, tone: "danger" }); }
  };
  return (
    <aside className="repository-git-state">
      <h2>Estado Git observado</h2>
      <dl><div><dt>Canónico</dt><dd><code>{shortID(repository.canonical_revision, 12)}</code></dd></div><div><dt>Worktrees</dt><dd>{worktrees.length} activos</dd></div><div><dt>Scopes</dt><dd>{workItems.reduce((count, item) => count + records(item.scopes).length, 0)} reservados</dd></div><div><dt>Verificado</dt><dd>{relativeDate(repository.synced_at || repository.last_success_at)}</dd></div></dl>
      {editable ? <Button variant="secondary" size="sm" loading={sync.isPending} onClick={() => void verify()}>Verificar ahora</Button> : null}
    </aside>
  );
}

function AttachRepositoryDialog({ open, onOpenChange, projectID, repositories }: { open: boolean; onOpenChange: (open: boolean) => void; projectID: string; repositories: RecordValue[] }) {
  const unattached = useMemo(() => repositories.filter((item) => !item.attached_repository_id), [repositories]);
  const [selected, setSelected] = useState("");
  const [purpose, setPurpose] = useState("backend");
  const [required, setRequired] = useState(true);
  const [primary, setPrimary] = useState(false);
  const attach = useAttachRepository(projectID);
  const { toast } = useToast();
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const githubRepositoryID = Number(selected);
    if (!Number.isSafeInteger(githubRepositoryID)) return;
    try {
      await attach.mutateAsync({ github_repository_id: githubRepositoryID, purpose: purpose.trim(), required, primary });
      setSelected("");
      onOpenChange(false);
      toast({ title: "Repositorio vinculado", tone: "success" });
    } catch (error) {
      toast({ title: "No se pudo vincular", description: (error as Error).message, tone: "danger" });
    }
  };
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><form onSubmit={submit}><DialogHeader><p className="pact-kicker">NUEVO REPOSITORIO</p><DialogTitle>Vincular repositorio autorizado</DialogTitle></DialogHeader><DialogBody className="pact-form-stack"><label className="pact-field"><span>Repositorio</span><select autoFocus required value={selected} onChange={(event) => setSelected(event.target.value)}><option value="">Selecciona un repositorio</option>{unattached.map((repository) => <option key={text(repository.github_repository_id || repository.id)} value={text(repository.github_repository_id || repository.id)}>{text(repository.full_name)}{repository.private ? " · privado" : ""}</option>)}</select></label><label className="pact-field"><span>Propósito</span><input value={purpose} onChange={(event) => setPurpose(event.target.value)} /></label><div className="attach-repository-options"><label className="pact-check"><input type="checkbox" checked={required} onChange={(event) => setRequired(event.target.checked)} /><span>Necesario para el workspace</span></label><label className="pact-check"><input type="checkbox" checked={primary} onChange={(event) => setPrimary(event.target.checked)} /><span>Repositorio principal</span></label></div></DialogBody><DialogFooter><Button variant="secondary" onClick={() => onOpenChange(false)}>Cancelar</Button><Button type="submit" loading={attach.isPending} disabled={!unattached.length}>Vincular repositorio</Button></DialogFooter></form></DialogContent></Dialog>;
}
