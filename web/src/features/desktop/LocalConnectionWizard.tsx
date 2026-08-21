import { useEffect, useMemo, useState } from "react";

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
} from "@/components/ui";
import {
  desktopBridge,
  type BindLocalFolderResult,
  type DesktopServerProfile,
  type LocalFolderInspection,
  type LocalFolderResolution,
  type RepositoryBindingMatch,
} from "@/platform/desktop";

type ClientID = "codex" | "claude";

interface LocalConnectionWizardProps {
  open: boolean;
  profiles: DesktopServerProfile[];
  preferredClient: ClientID;
  onOpenChange: (open: boolean) => void;
  onConnected: (result: BindLocalFolderResult) => void;
  onManageConnections: () => void;
}

const steps = ["Carpeta", "Conexión", "Destino", "Clientes", "Confirmar"];

function message(error: unknown): string {
  return error instanceof Error ? error.message : "No se pudo completar la configuración local.";
}

export function LocalConnectionWizard({
  open,
  profiles,
  preferredClient,
  onOpenChange,
  onConnected,
  onManageConnections,
}: LocalConnectionWizardProps) {
  const bridge = desktopBridge();
  const [step, setStep] = useState(0);
  const [folder, setFolder] = useState<LocalFolderInspection | null>(null);
  const [profileID, setProfileID] = useState("");
  const [resolution, setResolution] = useState<LocalFolderResolution | null>(null);
  const [workspaceID, setWorkspaceID] = useState("");
  const [match, setMatch] = useState<RepositoryBindingMatch | null>(null);
  const [createIfNeeded, setCreateIfNeeded] = useState(false);
  const [clients, setClients] = useState<ClientID[]>([preferredClient]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    setStep(0);
    setFolder(null);
    setProfileID(profiles.find((profile) => profile.active)?.id || profiles[0]?.id || "");
    setResolution(null);
    setWorkspaceID("");
    setMatch(null);
    setCreateIfNeeded(false);
    setClients([preferredClient]);
    setBusy(false);
    setError("");
  }, [open, preferredClient, profiles]);

  const availableMatches = useMemo(
    () => (resolution?.matches || []).filter((candidate) => candidate.workspace_id === workspaceID),
    [resolution?.matches, workspaceID],
  );
  const selectedWorkspace = resolution?.workspaces.find((workspace) => workspace.id === workspaceID);
  const selectedProfile = profiles.find((profile) => profile.id === profileID);

  const selectFolder = async () => {
    if (!bridge) return;
    setBusy(true);
    setError("");
    try {
      const selected = await bridge.SelectLocalProjectFolder();
      if (!selected.canceled) {
        setFolder(selected);
        if (selected.profile_id && profiles.some((profile) => profile.id === selected.profile_id)) {
          setProfileID(selected.profile_id);
        }
      }
    } catch (nextError) {
      setError(message(nextError));
    } finally {
      setBusy(false);
    }
  };

  const resolveFolder = async () => {
    if (!bridge || !folder?.root || !profileID) return false;
    setBusy(true);
    setError("");
    try {
      const next = await bridge.ResolveLocalFolder({ project_root: folder.root, profile_id: profileID });
      setResolution(next);
      const existingWorkspace = next.folder.profile_id === profileID ? next.folder.workspace_id : "";
      const initialWorkspace = existingWorkspace
        || next.matches[0]?.workspace_id
        || next.workspaces[0]?.id
        || "";
      setWorkspaceID(initialWorkspace);
      const initialMatch = next.matches.find((candidate) => candidate.workspace_id === initialWorkspace) || null;
      setMatch(initialMatch);
      setCreateIfNeeded(false);
      return true;
    } catch (nextError) {
      setError(message(nextError));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const continueFlow = async () => {
    if (step === 1) {
      if (await resolveFolder()) setStep(2);
      return;
    }
    setStep((current) => Math.min(4, current + 1));
  };

  const selectWorkspace = (id: string) => {
    setWorkspaceID(id);
    const nextMatch = resolution?.matches.find((candidate) => candidate.workspace_id === id) || null;
    setMatch(nextMatch);
    setCreateIfNeeded(false);
  };

  const toggleClient = (client: ClientID) => {
    setClients((current) => current.includes(client)
      ? current.filter((candidate) => candidate !== client)
      : [...current, client]);
  };

  const finish = async () => {
    if (!bridge || !folder?.root || !profileID || !workspaceID || (!match && !createIfNeeded) || clients.length === 0) return;
    setBusy(true);
    setError("");
    try {
      const result = await bridge.BindLocalFolder({
        project_root: folder.root,
        profile_id: profileID,
        workspace_id: workspaceID,
        project_id: match?.project_id,
        repository_id: match?.repository_id,
        create_if_needed: createIfNeeded,
        rebind: Boolean(folder.connected && (
          folder.profile_id !== profileID
          || folder.workspace_id !== workspaceID
          || folder.repository_id !== match?.repository_id
        )),
        clients,
      });
      onConnected(result);
    } catch (nextError) {
      setError(message(nextError));
    } finally {
      setBusy(false);
    }
  };

  const canContinue = step === 0
    ? Boolean(folder?.root && !folder.error)
    : step === 1
      ? Boolean(profileID)
      : step === 2
        ? Boolean(workspaceID && (match || createIfNeeded))
        : step === 3
          ? clients.length > 0
          : true;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="local-connect-dialog local-onboarding-dialog" closeOnBackdrop={!busy}>
        <DialogHeader>
          <p className="pact-kicker">CONFIGURAR ESTE COMPUTADOR</p>
          <DialogTitle>Conectar una carpeta a PACT</DialogTitle>
          <DialogDescription>PACT vincula un checkout local con un repositorio compartido y configura los clientes de IA que elijas.</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <ol className="local-wizard-progress" aria-label="Progreso de configuración">
            {steps.map((label, index) => (
              <li key={label} data-current={index === step || undefined} data-complete={index < step || undefined}>
                <span>{index < step ? "✓" : index + 1}</span><small>{label}</small>
              </li>
            ))}
          </ol>

          {step === 0 ? (
            <section className="local-wizard-panel">
              <p className="local-wizard-eyebrow">1 · CARPETA LOCAL</p>
              <h3>¿En qué checkout trabajarán los clientes?</h3>
              <p>Debe ser un repositorio Git con un remote <code>origin</code> y al menos un commit. No necesita tener PACT configurado todavía.</p>
              <button type="button" className="local-folder-picker" onClick={() => void selectFolder()} disabled={busy}>
                <Icon name="folder" size="lg" />
                <span><strong>{folder?.name || "Elegir carpeta Git"}</strong><small>{folder?.root || "PACT solo inspeccionará la carpeta que selecciones."}</small></span>
                <b>{folder ? "Cambiar" : "Seleccionar"}</b>
              </button>
              {folder?.remote_url ? <dl className="local-folder-facts"><div><dt>REMOTE</dt><dd>{folder.remote_url}</dd></div><div><dt>RAMA</dt><dd>{folder.branch}</dd></div></dl> : null}
            </section>
          ) : null}

          {step === 1 ? (
            <section className="local-wizard-panel">
              <p className="local-wizard-eyebrow">2 · CONEXIÓN PACT</p>
              <h3>¿En qué PACT Server vive este proyecto?</h3>
              <p>Las conexiones pertenecen a este computador. Cada carpeta conserva su servidor y no depende de la conexión activa de la interfaz.</p>
              <div className="local-profile-choice" role="radiogroup" aria-label="Conexiones PACT Server">
                {profiles.map((profile) => (
                  <button key={profile.id} type="button" role="radio" aria-checked={profileID === profile.id} data-selected={profileID === profile.id || undefined} onClick={() => setProfileID(profile.id)}>
                    <span className="connection-dot" />
                    <strong>{profile.label}</strong>
                    <code>{profile.server_url}</code>
                    <small>{profile.kind === "managed_local" ? "Servidor local" : profile.principal_label || "Servidor remoto"}</small>
                  </button>
                ))}
              </div>
              {!profiles.length ? <div className="local-inline-alert">Este computador todavía no tiene conexiones PACT autorizadas.</div> : null}
              <Button variant="ghost" size="sm" onClick={onManageConnections}>Administrar conexiones</Button>
            </section>
          ) : null}

          {step === 2 ? (
            <section className="local-wizard-panel">
              <p className="local-wizard-eyebrow">3 · DESTINO COMPARTIDO</p>
              <h3>Elige el workspace y el repositorio</h3>
              <p>PACT encontró el remote de la carpeta en <strong>{selectedProfile?.label}</strong>. La vinculación local será explícita y verificable.</p>
              <label className="local-wizard-field">Workspace
                <select value={workspaceID} onChange={(event) => selectWorkspace(event.target.value)}>
                  <option value="">Selecciona un workspace</option>
                  {(resolution?.workspaces || []).map((workspace) => <option key={workspace.id} value={workspace.id}>{workspace.name}</option>)}
                </select>
              </label>
              {workspaceID && availableMatches.length ? (
                <div className="local-repository-choice" role="radiogroup" aria-label="Repositorio PACT">
                  {availableMatches.map((candidate) => (
                    <button key={candidate.repository_id} type="button" role="radio" aria-checked={match?.repository_id === candidate.repository_id} data-selected={match?.repository_id === candidate.repository_id || undefined} onClick={() => { setMatch(candidate); setCreateIfNeeded(false); }}>
                      <span><strong>{candidate.repository_name}</strong><small>{candidate.project_name} · {candidate.primary ? "principal" : candidate.repository_slug}</small></span>
                      <small>{candidate.permission || "acceso"}</small>
                    </button>
                  ))}
                </div>
              ) : workspaceID ? (
                <button type="button" className="local-create-target" data-selected={createIfNeeded || undefined} onClick={() => { setCreateIfNeeded((value) => !value); setMatch(null); }}>
                  <Icon name="repository" />
                  <span><strong>Registrar este repositorio en {selectedWorkspace?.name}</strong><small>No existe una vinculación para <code>{folder?.remote_url}</code>. PACT creará el proyecto y lo añadirá a este workspace.</small></span>
                </button>
              ) : null}
              {resolution && !resolution.workspaces.length ? <div className="local-inline-alert">No tienes workspaces visibles en esta conexión. Crea uno desde PACT Server antes de continuar.</div> : null}
            </section>
          ) : null}

          {step === 3 ? (
            <section className="local-wizard-panel">
              <p className="local-wizard-eyebrow">4 · CLIENTES DE IA</p>
              <h3>¿Qué clientes podrán usar PACT en esta carpeta?</h3>
              <p>La configuración MCP se escribe únicamente dentro del checkout seleccionado.</p>
              <div className="local-client-choice local-client-choice-wide">
                {(["codex", "claude"] as const).map((candidate) => (
                  <button key={candidate} type="button" role="checkbox" aria-checked={clients.includes(candidate)} data-selected={clients.includes(candidate) || undefined} onClick={() => toggleClient(candidate)}>
                    <span>{candidate === "codex" ? "CX" : "CL"}</span>
                    <strong>{candidate === "codex" ? "Codex" : "Claude Code"}</strong>
                    <small>{clients.includes(candidate) ? "Se configurará" : "No seleccionado"}</small>
                  </button>
                ))}
              </div>
            </section>
          ) : null}

          {step === 4 ? (
            <section className="local-wizard-panel">
              <p className="local-wizard-eyebrow">5 · CONFIRMACIÓN</p>
              <h3>Todo listo para conectar</h3>
              <p>PACT creará los archivos locales necesarios, conservará esta carpeta en este computador y configurará los clientes seleccionados.</p>
              <dl className="local-confirmation-list">
                <div><dt>CARPETA</dt><dd><strong>{folder?.name}</strong><code>{folder?.root}</code></dd></div>
                <div><dt>CONEXIÓN</dt><dd><strong>{selectedProfile?.label}</strong><code>{selectedProfile?.server_url}</code></dd></div>
                <div><dt>WORKSPACE</dt><dd><strong>{selectedWorkspace?.name}</strong><small>{match ? match.repository_name : "Se registrará este repositorio"}</small></dd></div>
                <div><dt>CLIENTES</dt><dd><strong>{clients.map((client) => client === "codex" ? "Codex" : "Claude Code").join(" y ")}</strong></dd></div>
              </dl>
            </section>
          ) : null}

          {error ? <div className="local-inline-alert" role="alert">{error}</div> : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="secondary" disabled={busy} onClick={() => step === 0 ? onOpenChange(false) : setStep((current) => current - 1)}>{step === 0 ? "Cancelar" : "Atrás"}</Button>
          {step < 4
            ? <Button disabled={!canContinue} loading={busy} onClick={() => void continueFlow()}>Continuar</Button>
            : <Button disabled={!canContinue} loading={busy} loadingLabel="Conectando" onClick={() => void finish()}>Conectar carpeta</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
