import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";

import type { ProjectSummary, Workspace } from "@/api/types";
import { Page } from "@/components/layout/Page";
import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Icon,
  StatusChip,
  useToast,
} from "@/components/ui";
import {
  desktopBridge,
  type ConnectLocalAgentResult,
  type DesktopUpdateStatus,
  type LocalClientStatus,
  type LocalComputerStatus,
  type LocalFolderInspection,
} from "@/platform/desktop";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";

type LocalView = "overview" | "agents" | "folders" | "service";
type ClientID = "codex" | "claude";

const viewCopy: Record<LocalView, { title: string; description: string }> = {
  overview: {
    title: "Este computador",
    description: "El entorno local que conecta tus editores, agentes y carpetas con PACT Server.",
  },
  agents: {
    title: "Clientes de IA",
    description: "Configura qué aplicaciones de IA pueden usar el contexto compartido de cada carpeta.",
  },
  folders: {
    title: "Carpetas locales",
    description: "Checkouts de este equipo que ya tienen una integración local con PACT.",
  },
  service: {
    title: "Runtime local",
    description: "El componente nativo que los agentes ejecutan bajo demanda, separado de la interfaz de PACT Desktop.",
  },
};

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  return "Ocurrió un error inesperado.";
}

function platformLabel(status: LocalComputerStatus): string {
  const systems: Record<string, string> = { darwin: "macOS", windows: "Windows", linux: "Linux" };
  const architectures: Record<string, string> = { arm64: "Apple Silicon / ARM64", amd64: "Intel / AMD64" };
  return `${systems[status.operating_system] || status.operating_system} · ${architectures[status.architecture] || status.architecture}`;
}

function normalizeLocalStatus(status: LocalComputerStatus): LocalComputerStatus {
  return {
    ...status,
    managed_server: status.managed_server || { installed: false, running: false, ready: false },
    clients: Array.isArray(status.clients) ? status.clients : [],
    folders: Array.isArray(status.folders)
      ? status.folders.map((folder) => ({
        ...folder,
        clients: Array.isArray(folder.clients) ? folder.clients : [],
      }))
      : [],
  };
}

export function LocalComputerPage({ view = "overview" }: { view?: LocalView }) {
  const bridge = desktopBridge();
  const directory = useWorkspace();
  const navigate = useNavigate();
  const { toast } = useToast();
  const [status, setStatus] = useState<LocalComputerStatus | null>(null);
  const [updateStatus, setUpdateStatus] = useState<DesktopUpdateStatus | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [preferredClient, setPreferredClient] = useState<ClientID>("codex");

  const refresh = useCallback(async () => {
    if (!bridge) {
      setError("Esta sección solo está disponible en PACT Desktop.");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextStatus, nextUpdateStatus] = await Promise.all([
        bridge.LocalComputerStatus(),
        bridge.UpdateStatus(),
        directory.refreshDirectory(),
      ]);
      setStatus(normalizeLocalStatus(nextStatus));
      setUpdateStatus(nextUpdateStatus);
    } catch (nextError) {
      setError(errorMessage(nextError));
    } finally {
      setLoading(false);
    }
  }, [bridge, directory.refreshDirectory]);

  useEffect(() => { void refresh(); }, [refresh]);

  const openConnector = (client: ClientID = "codex") => {
    setPreferredClient(client);
    setDialogOpen(true);
  };

  const handleConnected = async (result: ConnectLocalAgentResult) => {
    setDialogOpen(false);
    toast({
      title: result.changed ? "Cliente conectado" : "La integración ya estaba actualizada",
      description: result.restart_needed
        ? "Abre un chat nuevo en esa carpeta para que el cliente cargue PACT MCP."
        : undefined,
      tone: "success",
    });
    await refresh();
  };

  const copy = viewCopy[view];
  return (
    <Page
      kicker="PACT DESKTOP"
      title={copy.title}
      showWorkspaceStatus={false}
      actions={(
        <>
          <Button variant="secondary" size="sm" onClick={() => void refresh()} loading={loading}>Actualizar todo</Button>
          {view !== "service" ? <Button size="sm" onClick={() => openConnector()}>Conectar cliente</Button> : null}
        </>
      )}
    >
      <div className="local-computer-page">
        <p className="local-page-intro">{copy.description}</p>
        {error ? <div className="local-inline-alert" role="alert">{error}</div> : null}
        {loading && !status ? <div className="local-loading">Leyendo la configuración local…</div> : null}
        {status && view === "overview" ? (
          <LocalOverview
            status={status}
            workspaces={directory.workspaces}
            onNavigate={(target) => navigate(`/local/${target}`)}
            onOpenWorkspace={(workspaceID) => navigate(`/w/${encodeURIComponent(workspaceID)}`)}
            onConnect={openConnector}
          />
        ) : null}
        {status && view === "agents" ? <LocalAgents status={status} onConnect={openConnector} /> : null}
        {status && view === "folders" ? <LocalFolders status={status} onConnect={openConnector} /> : null}
        {status && view === "service" ? <LocalService status={status} updateStatus={updateStatus} onRefresh={refresh} /> : null}
      </div>
      <ConnectAgentDialog
        open={dialogOpen}
        preferredClient={preferredClient}
        currentServer={status?.server_url}
        workspaces={directory.workspaces}
        projects={directory.projects}
        onOpenChange={setDialogOpen}
        onConnected={(result) => void handleConnected(result)}
      />
    </Page>
  );
}

function LocalOverview({
  status,
  workspaces,
  onNavigate,
  onOpenWorkspace,
  onConnect,
}: {
  status: LocalComputerStatus;
  workspaces: Workspace[];
  onNavigate: (target: Exclude<LocalView, "overview">) => void;
  onOpenWorkspace: (workspaceID: string) => void;
  onConnect: (client: ClientID) => void;
}) {
  const connectedClients = status.clients.filter((client) => client.connected_folders > 0).length;
  return (
    <>
      <section className="local-machine-strip" aria-label="Estado de este computador">
        <div className="local-machine-identity">
          <span className="local-machine-icon"><Icon name="computer" size="lg" /></span>
          <span><strong>{status.hostname}</strong><small>{platformLabel(status)}</small></span>
        </div>
        <div><span>PACT Server</span><strong>{status.server_url || "Sin conexión"}</strong></div>
        <div><span>Runtime</span><StatusChip tone={status.runtime_ready ? "active" : "danger"}>{status.runtime_ready ? "Listo" : "No disponible"}</StatusChip></div>
      </section>

      <section className="local-section local-shared-workspaces">
        <header>
          <div><span>EN PACT SERVER</span><h2>Workspaces compartidos</h2></div>
          <small>Disponibles en todos los equipos con acceso a {status.server_url || "este servidor"}.</small>
        </header>
        {workspaces.length ? (
          <div className="local-workspace-list">
            {workspaces.map((workspace) => (
              <button key={workspace.id} type="button" onClick={() => onOpenWorkspace(workspace.id)}>
                <span className="local-workspace-color" style={{ backgroundColor: workspace.color || "#c9ee4d" }} aria-hidden="true" />
                <span><strong>{workspace.name}</strong><small>{workspace.description || "Contexto y coordinación compartidos"}</small></span>
                <span>{workspace.projects?.length || 0} {(workspace.projects?.length || 0) === 1 ? "repositorio" : "repositorios"}</span>
                <Icon name="arrowRight" size="sm" />
              </button>
            ))}
          </div>
        ) : (
          <p className="local-shared-empty">Este servidor aún no tiene workspaces visibles para tu cuenta.</p>
        )}
      </section>

      <section className="local-section local-device-section">
        <header>
          <div><span>SOLO EN ESTE EQUIPO</span><h2>Configuración local</h2></div>
          <small>Las rutas y los clientes no se copian a otros computadores.</small>
        </header>
        <div className="local-overview-grid">
        <button type="button" onClick={() => onNavigate("agents")}>
          <span><Icon name="people" /><strong>Clientes de IA</strong></span>
          <b>{connectedClients} / {status.clients.length}</b>
          <small>Codex y Claude configurados en carpetas locales</small>
        </button>
        <button type="button" onClick={() => onNavigate("folders")}>
          <span><Icon name="folder" /><strong>Carpetas</strong></span>
          <b>{status.folders.length}</b>
          <small>Checkouts recordados por este computador</small>
        </button>
        <button type="button" onClick={() => onNavigate("service")}>
          <span><Icon name="server" /><strong>Runtime local</strong></span>
          <b>{status.runtime_ready ? "OK" : "Error"}</b>
          <small>Se inicia bajo demanda y no depende de la ventana</small>
        </button>
        </div>
      </section>

      <section className="local-section">
        <header><div><span>INICIO RÁPIDO</span><h2>Conecta tu primer cliente</h2></div></header>
        <div className="local-client-list">
          {status.clients.map((client) => (
            <ClientRow key={client.id} client={client} onConnect={() => onConnect(client.id)} />
          ))}
        </div>
      </section>
    </>
  );
}

function LocalAgents({ status, onConnect }: { status: LocalComputerStatus; onConnect: (client: ClientID) => void }) {
  return (
    <section className="local-section local-primary-section">
      <header>
        <div><span>CLIENTES MCP</span><h2>Aplicaciones disponibles en este equipo</h2></div>
        <small>La configuración se aplica por carpeta, no a toda la cuenta.</small>
      </header>
      <div className="local-client-list">
        {status.clients.map((client) => (
          <ClientRow key={client.id} client={client} onConnect={() => onConnect(client.id)} />
        ))}
      </div>
      <p className="local-section-note">PACT instala una definición MCP local. No entrega tu contraseña al agente y no concede acceso automático a otros workspaces.</p>
    </section>
  );
}

function ClientRow({ client, onConnect }: { client: LocalClientStatus; onConnect: () => void }) {
  return (
    <article className="local-client-row">
      <span className="local-client-mark">{client.id === "codex" ? "CX" : "CL"}</span>
      <div>
        <strong>{client.name}</strong>
        <small>{client.detected ? `Detectado${client.detection ? ` · ${client.detection}` : ""}` : "No detectado; puedes preparar la integración igualmente"}</small>
      </div>
      <StatusChip tone={client.connected_folders > 0 ? "active" : client.detected ? "info" : "neutral"}>
        {client.connected_folders > 0 ? `${client.connected_folders} ${client.connected_folders === 1 ? "carpeta" : "carpetas"}` : client.detected ? "Disponible" : "Pendiente"}
      </StatusChip>
      <Button variant="secondary" size="sm" onClick={onConnect}>{client.connected_folders > 0 ? "Conectar otra" : "Conectar"}</Button>
    </article>
  );
}

function LocalFolders({ status, onConnect }: { status: LocalComputerStatus; onConnect: (client: ClientID) => void }) {
  return (
    <section className="local-section local-primary-section">
      <header><div><span>SOLO EN ESTE EQUIPO</span><h2>Carpetas recordadas</h2></div><small>Las rutas locales no se sincronizan con otros computadores.</small></header>
      {status.folders.length === 0 ? (
        <div className="local-empty">
          <Icon name="folder" size="lg" />
          <strong>Aún no hay carpetas conectadas desde PACT Desktop.</strong>
          <p>Elige Codex o Claude y luego selecciona una carpeta Git que ya pertenezca a este PACT Server.</p>
          <Button onClick={() => onConnect("codex")}>Conectar una carpeta</Button>
        </div>
      ) : (
        <div className="local-folder-table-wrap">
          <table className="local-folder-table">
            <thead><tr><th>CARPETA</th><th>CLIENTES</th><th>PACT SERVER</th><th>ESTADO</th></tr></thead>
            <tbody>
              {status.folders.map((folder) => (
                <tr key={folder.root}>
                  <td><strong>{folder.name}</strong><code>{folder.root}</code></td>
                  <td>{folder.clients.length ? folder.clients.map((client) => <span key={client}>{client}</span>) : "—"}</td>
                  <td><code>{folder.server_url}</code></td>
                  <td><StatusChip tone={folder.available ? "active" : "danger"}>{folder.available ? "Disponible" : "No disponible"}</StatusChip></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function LocalService({
  status,
  updateStatus,
  onRefresh,
}: {
  status: LocalComputerStatus;
  updateStatus: DesktopUpdateStatus | null;
  onRefresh: () => Promise<void>;
}) {
  const bridge = desktopBridge();
  const { toast } = useToast();
  const [busy, setBusy] = useState("");
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [setupCode, setSetupCode] = useState("");
  const server = status.managed_server;

  const checkForUpdates = async () => {
    if (!bridge) return;
    setCheckingUpdate(true);
    try {
      await bridge.CheckForUpdates();
    } catch (nextError) {
      toast({ title: "No se pudo comprobar la actualización", description: errorMessage(nextError), tone: "danger" });
    } finally {
      setCheckingUpdate(false);
      await onRefresh();
    }
  };

  const operate = async (operation: "install" | "start" | "stop" | "backup" | "upgrade") => {
    if (!bridge) return;
    setBusy(operation);
    try {
      if (operation === "install") {
        const result = await bridge.InstallLocalServer({ port: 8080 });
        setSetupCode(result.setup_code);
        toast({ title: "PACT Server local instalado", description: result.status.server_url, tone: "success" });
      } else if (operation === "start") {
        await bridge.StartLocalServer();
        toast({ title: "PACT Server local iniciado", tone: "success" });
      } else if (operation === "stop") {
        await bridge.StopLocalServer();
        toast({ title: "PACT Server local detenido", description: "Los datos permanecen guardados.", tone: "success" });
      } else if (operation === "backup") {
        const path = await bridge.BackupLocalServer();
        toast({ title: "Respaldo creado", description: path, tone: "success" });
      } else {
        const result = await bridge.UpgradeLocalServer("");
        toast({ title: "PACT Server local actualizado", description: `Respaldo previo: ${result.backup}`, tone: "success" });
      }
      await onRefresh();
    } catch (nextError) {
      toast({ title: "No se pudo administrar PACT Server", description: errorMessage(nextError), tone: "danger" });
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="local-service-layout">
      <div className="local-service-stack">
      <section className="local-section local-primary-section">
        <header><div><span>PACT LOCAL RUNTIME</span><h2>{status.runtime_ready ? "Instalado y listo" : "No se pudo instalar"}</h2></div><StatusChip tone={status.runtime_ready ? "active" : "danger"}>{status.runtime_ready ? "Bajo demanda" : "Error"}</StatusChip></header>
        <dl className="local-runtime-facts">
          <div><dt>Modo</dt><dd>Bajo demanda por MCP</dd></div>
          <div><dt>Versión local</dt><dd><code>{status.runtime_version || "—"}</code></dd></div>
          <div><dt>Plataforma</dt><dd>{platformLabel(status)}</dd></div>
          <div><dt>Servidor</dt><dd><code>{status.server_url || "—"}</code></dd></div>
        </dl>
        {status.runtime_path ? <div className="local-runtime-path"><span>EJECUTABLE</span><code>{status.runtime_path}</code></div> : null}
        {status.runtime_error ? <div className="local-inline-alert" role="alert">{status.runtime_error}</div> : null}
      </section>
      <section className="local-section local-primary-section local-update-section">
        <header>
          <div><span>PACT DESKTOP</span><h2>Actualizaciones de la aplicación</h2></div>
          <StatusChip tone={updateStatus?.configured ? "active" : "warning"}>{updateStatus?.configured ? "Firmadas" : "Desarrollo"}</StatusChip>
        </header>
        <dl className="local-runtime-facts">
          <div><dt>Versión</dt><dd><code>{updateStatus?.current_version || "—"}</code></dd></div>
          <div><dt>Compilación</dt><dd><code>{updateStatus?.commit || "—"}</code></dd></div>
          <div><dt>Estado</dt><dd>{updateStatus?.state || "—"}</dd></div>
          <div><dt>Canal</dt><dd>Estable</dd></div>
        </dl>
        {updateStatus?.error && updateStatus.current_version !== "dev" ? <div className="local-inline-alert" role="alert">{updateStatus.error}</div> : null}
        <div className="local-server-actions">
          <Button variant="secondary" loading={checkingUpdate} disabled={!updateStatus?.configured} onClick={() => void checkForUpdates()}>Buscar actualizaciones</Button>
          <small>PACT verifica la descarga con checksum y firma Ed25519 antes de reemplazar la aplicación.</small>
        </div>
      </section>
      <section className="local-section local-primary-section local-server-section">
        <header>
          <div><span>PACT SERVER LOCAL</span><h2>{server.installed ? (server.ready ? "En ejecución y listo" : server.running ? "Iniciando" : "Instalado y detenido") : "No instalado"}</h2></div>
          <StatusChip tone={server.ready ? "active" : server.installed ? "warning" : "neutral"}>{server.ready ? "Listo" : server.installed ? "Detenido" : "Opcional"}</StatusChip>
        </header>
        {server.installed ? (
          <>
            <dl className="local-runtime-facts">
              <div><dt>URL</dt><dd><code>{server.server_url || "—"}</code></dd></div>
              <div><dt>Versión</dt><dd><code>{server.version || "—"}</code></dd></div>
              <div><dt>Imagen</dt><dd><code>{server.image || "—"}</code></dd></div>
              <div><dt>Datos</dt><dd><code>{server.data_directory || "—"}</code></dd></div>
            </dl>
            {server.error ? <div className="local-inline-alert" role="alert">{server.error}</div> : null}
            <div className="local-server-actions">
              {server.running
                ? <Button variant="secondary" loading={busy === "stop"} onClick={() => void operate("stop")}>Detener</Button>
                : <Button loading={busy === "start"} onClick={() => void operate("start")}>Iniciar</Button>}
              <Button variant="secondary" loading={busy === "backup"} disabled={!server.running} onClick={() => void operate("backup")}>Crear respaldo</Button>
              <Button variant="secondary" loading={busy === "upgrade"} disabled={!server.running} onClick={() => void operate("upgrade")}>Actualizar</Button>
              {server.server_url ? <Button variant="ghost" onClick={() => void bridge?.OpenExternalURL(`${server.server_url}/admin/`)}>Abrir panel</Button> : null}
            </div>
          </>
        ) : (
          <div className="local-server-empty">
            <p>Instala el mismo PACT Server, PostgreSQL y pgvector utilizados por un equipo remoto. Docker Desktop debe estar instalado y abierto.</p>
            <Button loading={busy === "install"} onClick={() => void operate("install")}>Instalar PACT Server local</Button>
          </div>
        )}
        {setupCode ? (
          <div className="local-setup-code">
            <span>CÓDIGO PARA CREAR LA CUENTA PROPIETARIA</span>
            <code>{setupCode}</code>
            <small>Solo se necesita durante la primera configuración. Puedes seleccionarlo y copiarlo.</small>
          </div>
        ) : null}
      </section>
      </div>
      <aside className="local-service-explainer">
        <h2>Qué ocurre cuando abres un chat</h2>
        <ol>
          <li>Codex o Claude inicia el runtime local de PACT.</li>
          <li>El runtime lee la carpeta, su binding y la autorización de este dispositivo.</li>
          <li>PACT registra la sesión, el latido y el trabajo activo en el servidor.</li>
          <li>Al cerrar el cliente, la sesión termina sin depender de la interfaz Desktop.</li>
        </ol>
        <p>El runtime MCP y PACT Server son piezas diferentes. El runtime conecta agentes; el servidor mantiene el contexto y la coordinación compartida.</p>
      </aside>
    </div>
  );
}

function ConnectAgentDialog({
  open,
  preferredClient,
  currentServer,
  workspaces,
  projects,
  onOpenChange,
  onConnected,
}: {
  open: boolean;
  preferredClient: ClientID;
  currentServer?: string;
  workspaces: Workspace[];
  projects: ProjectSummary[];
  onOpenChange: (open: boolean) => void;
  onConnected: (result: ConnectLocalAgentResult) => void;
}) {
  const bridge = desktopBridge();
  const [client, setClient] = useState<ClientID>(preferredClient);
  const [folder, setFolder] = useState<LocalFolderInspection | null>(null);
  const [selecting, setSelecting] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setClient(preferredClient);
    setFolder(null);
    setError("");
    setSelecting(false);
    setConnecting(false);
  }, [open, preferredClient]);

  const serverMatches = useMemo(() => Boolean(
    folder?.connected && folder.server_url && currentServer && folder.server_url === currentServer,
  ), [currentServer, folder]);
  const project = useMemo(
    () => projects.find((item) => item.id === folder?.project_id),
    [folder?.project_id, projects],
  );
  const workspace = useMemo(
    () => workspaces.find((item) => item.id === project?.workspace_id
      || item.projects?.some((candidate) => candidate.id === folder?.project_id)),
    [folder?.project_id, project?.workspace_id, workspaces],
  );

  const selectFolder = async () => {
    if (!bridge) return;
    setSelecting(true);
    setError("");
    try {
      const selected = await bridge.SelectLocalProjectFolder();
      if (!selected.canceled) setFolder(selected);
    } catch (nextError) {
      setError(errorMessage(nextError));
    } finally {
      setSelecting(false);
    }
  };

  const connect = async () => {
    if (!bridge || !folder?.root || !serverMatches) return;
    setConnecting(true);
    setError("");
    try {
      onConnected(await bridge.ConnectLocalAgent({ client, project_root: folder.root }));
    } catch (nextError) {
      setError(errorMessage(nextError));
    } finally {
      setConnecting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="local-connect-dialog">
        <DialogHeader>
          <p className="pact-kicker">INTEGRACIÓN LOCAL</p>
          <DialogTitle>Conectar un cliente a una carpeta</DialogTitle>
          <DialogDescription>La carpeta define el workspace y el repositorio. Después eliges qué cliente puede utilizar PACT dentro de ese alcance.</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <div className="local-wizard-step">
            <span>1</span>
            <div><strong>Selecciona la carpeta</strong><small>Debe ser un repositorio Git ya conectado al mismo PACT Server.</small></div>
          </div>
          <button type="button" className="local-folder-picker" onClick={() => void selectFolder()} disabled={selecting}>
            <Icon name="folder" size="lg" />
            <span>
              <strong>{folder?.name || (selecting ? "Abriendo selector…" : "Elegir carpeta")}</strong>
              <small>{folder?.root || "PACT no escanea tu disco; tú decides qué checkout conectar."}</small>
            </span>
            <b>{folder ? "Cambiar" : "Seleccionar"}</b>
          </button>
          {folder && !folder.connected ? <div className="local-inline-alert" role="alert">{folder.error || "Esta carpeta aún no está conectada a PACT."}</div> : null}
          {folder?.connected && !serverMatches ? <div className="local-inline-alert" role="alert">La carpeta pertenece a {folder.server_url}, pero Desktop está conectado a {currentServer || "otro servidor"}.</div> : null}

          <div className="local-wizard-step">
            <span>2</span>
            <div><strong>Elige el cliente</strong><small>Puedes conectar Codex y Claude por separado en esta misma carpeta.</small></div>
          </div>
          <div className="local-client-choice" role="radiogroup" aria-label="Cliente de IA">
            {(["codex", "claude"] as const).map((candidate) => (
              <button
                key={candidate}
                type="button"
                role="radio"
                aria-checked={client === candidate}
                data-selected={client === candidate || undefined}
                onClick={() => setClient(candidate)}
              >
                <span>{candidate === "codex" ? "CX" : "CL"}</span>
                <strong>{candidate === "codex" ? "Codex" : "Claude Code"}</strong>
                <small>Configuración MCP para esta carpeta</small>
              </button>
            ))}
          </div>

          {folder?.connected && serverMatches ? (
            <div className="local-connection-preview">
              <span><small>CARPETA</small><strong>{folder.name}</strong></span>
              <span><small>WORKSPACE</small><strong>{workspace?.name || "Workspace vinculado"}</strong></span>
              <span><small>REPOSITORIO</small><strong>{project?.name || folder.project_id || "Repositorio vinculado"}</strong></span>
              <span><small>CLIENTE</small><strong>{client === "codex" ? "Codex" : "Claude Code"}</strong></span>
              <span><small>PACT SERVER</small><code>{folder.server_url}</code></span>
            </div>
          ) : null}
          {error ? <div className="local-inline-alert" role="alert">{error}</div> : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>Cancelar</Button>
          <Button disabled={!serverMatches} loading={connecting} onClick={() => void connect()}>Conectar cliente</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
