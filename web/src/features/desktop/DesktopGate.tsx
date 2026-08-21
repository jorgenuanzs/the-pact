import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";

import {
  desktopBridge,
  type DesktopDeviceLogin,
  type DesktopServerProfile,
  type DesktopStatus,
  type LocalServerStatus,
} from "@/platform/desktop";

import styles from "./desktop.module.css";

type Phase = "checking" | "server" | "approval" | "unreachable";

export function DesktopGate({ children }: { children: ReactNode }) {
  const native = desktopBridge();
  const [phase, setPhase] = useState<Phase>(native ? "checking" : "server");
  const [status, setStatus] = useState<DesktopStatus | null>(null);
  const [serverURL, setServerURL] = useState("");
  const [authorization, setAuthorization] = useState<DesktopDeviceLogin | null>(null);
  const [localServer, setLocalServer] = useState<LocalServerStatus | null>(null);
  const [profiles, setProfiles] = useState<DesktopServerProfile[]>([]);
  const [localSetupCode, setLocalSetupCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const checkStatus = useCallback(async () => {
    if (!native) return;
    setPhase("checking");
    setError("");
    try {
      const [current, local, profileList] = await Promise.all([
        native.Status(), native.LocalServerStatus(), native.ListServerProfiles(),
      ]);
      setStatus(current);
      setLocalServer(local);
      setProfiles(profileList || []);
      setServerURL(current.server_url || current.default_url || "");
      if (current.connected) return;
      setPhase(current.configured ? "unreachable" : "server");
    } catch (statusError) {
      setError(message(statusError, "No se pudo iniciar PACT Desktop."));
      setPhase("server");
    }
  }, [native]);

  useEffect(() => {
    void checkStatus();
  }, [checkStatus]);

  useEffect(() => {
    if (!native || !authorization) return;
    let active = true;
    let timer = 0;
    const poll = async () => {
      if (!active) return;
      if (Date.now() >= new Date(authorization.expires_at).getTime()) {
        setError("La autorización venció. Inicia una conexión nueva.");
        setAuthorization(null);
        setPhase("server");
        return;
      }
      try {
        const result = await native.PollDeviceLogin(authorization.server_url, authorization.device_code);
        if (!active) return;
        if (result.connected) {
          await checkStatus();
          return;
        }
        if (result.status !== "pending") {
          setError(`El servidor cerró la autorización con estado ${result.status}.`);
          setAuthorization(null);
          setPhase("server");
          return;
        }
      } catch (pollError) {
        if (!active) return;
        setError(message(pollError, "No se pudo completar la autorización."));
      }
      timer = window.setTimeout(poll, Math.max(1, authorization.interval_seconds) * 1_000);
    };
    timer = window.setTimeout(poll, Math.max(1, authorization.interval_seconds) * 1_000);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [authorization, checkStatus, native]);

  if (!native) return children;
  if (status?.connected) return children;

  const beginLogin = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const next = await native.BeginDeviceLogin(serverURL);
      setAuthorization(next);
      setServerURL(next.server_url);
      setPhase("approval");
    } catch (loginError) {
      setError(message(loginError, "No se pudo comenzar la conexión."));
    } finally {
      setBusy(false);
    }
  };

  const forgetConnection = async () => {
    setBusy(true);
    setError("");
    try {
      await native.Disconnect(true);
      await checkStatus();
    } catch (disconnectError) {
      setError(message(disconnectError, "No se pudo eliminar la conexión local."));
    } finally {
      setBusy(false);
    }
  };

  const useProfile = async (profileID: string) => {
    setBusy(true);
    setError("");
    try {
      await native.UseServerProfile(profileID);
      await checkStatus();
    } catch (profileError) {
      setError(message(profileError, "No se pudo abrir esa conexión PACT."));
    } finally {
      setBusy(false);
    }
  };

  const authorizeLocalServer = async (local: LocalServerStatus, setupCode = "") => {
    if (!local.server_url) throw new Error("El servidor local no informó su URL.");
    setLocalSetupCode(setupCode);
    const next = await native.BeginDeviceLogin(local.server_url);
    setAuthorization(next);
    setServerURL(next.server_url);
    setPhase("approval");
  };

  const useLocalServer = async () => {
    setBusy(true);
    setError("");
    try {
      let local = localServer;
      if (!local?.installed) {
        const installed = await native.InstallLocalServer({ port: 8080 });
        local = installed.status;
        setLocalServer(local);
        await authorizeLocalServer(local, installed.setup_code);
        return;
      }
      if (!local.running || !local.ready) {
        local = await native.StartLocalServer();
        setLocalServer(local);
      }
      await authorizeLocalServer(local);
    } catch (localError) {
      setError(message(localError, "No se pudo preparar PACT Server local."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className={styles.shell}>
      <section className={styles.story} aria-label="PACT Desktop">
        <span className={styles.brand}><i /> PACT</span>
        <div>
          <p className={styles.eyebrow}>CONTROL LOCAL</p>
          <h1>Tu proyecto, sus agentes y el contexto compartido en una sola aplicación.</h1>
          <p>PACT Desktop conecta este computador con un PACT Server sin entregar tu contraseña al cliente ni exponer la credencial a la interfaz.</p>
        </div>
        <ol className={styles.steps}>
          <li data-active={phase === "server"}>Servidor</li>
          <li data-active={phase === "approval"}>Autorización</li>
          <li>Workspace</li>
        </ol>
      </section>

      <section className={styles.panel}>
        {phase === "checking" ? (
          <DesktopLoading />
        ) : phase === "approval" && authorization ? (
          <div className={styles.card}>
            <p className={styles.eyebrow}>AUTORIZAR DISPOSITIVO</p>
            <h2>Confirma este computador</h2>
            <p>Se abrió PACT Server en tu navegador. Inicia sesión allí y confirma que aparece este código:</p>
            <strong className={styles.code}>{authorization.user_code}</strong>
            {localSetupCode ? (
              <div className={styles.setupCode}>
                <span>PRIMERA CONFIGURACIÓN</span>
                <p>Como este servidor es nuevo, utiliza también este código para crear la cuenta propietaria:</p>
                <code>{localSetupCode}</code>
              </div>
            ) : null}
            <button className={styles.primary} type="button" onClick={() => void native.OpenExternalURL(authorization.verification_url)}>Abrir PACT Server</button>
            <button className={styles.secondary} type="button" onClick={() => { setAuthorization(null); setPhase("server"); }}>Cancelar</button>
            <small>PACT Desktop está esperando la aprobación. La contraseña solo se introduce en el servidor.</small>
            {error ? <p className={styles.error}>{error}</p> : null}
          </div>
        ) : phase === "unreachable" ? (
          <div className={styles.card}>
            <p className={styles.eyebrow}>SERVIDOR CONFIGURADO</p>
            <h2>No pudimos comprobar la conexión</h2>
            <p>Este computador está vinculado con <strong>{status?.server_url}</strong>, pero el servidor no respondió o la autorización dejó de ser válida.</p>
            <p className={styles.error}>{status?.error || error}</p>
            <button className={styles.primary} type="button" disabled={busy} onClick={() => void checkStatus()}>Intentar nuevamente</button>
            {profiles.filter((profile) => !profile.active).map((profile) => (
              <button key={profile.id} className={styles.secondary} type="button" disabled={busy} onClick={() => void useProfile(profile.id)}>
                Abrir {profile.label}
              </button>
            ))}
            <button className={styles.secondary} type="button" disabled={busy} onClick={() => void forgetConnection()}>Olvidar esta conexión</button>
          </div>
        ) : (
          <form className={styles.card} onSubmit={(event) => void beginLogin(event)}>
            <p className={styles.eyebrow}>CONECTAR PACT SERVER</p>
            <h2>¿A qué PACT Server quieres conectarte?</h2>
            <p>Introduce la URL proporcionada por tu empresa o utiliza una instalación local de PACT.</p>
            <label htmlFor="pact-desktop-server">URL del servidor</label>
            <input
              id="pact-desktop-server"
              type="url"
              autoFocus
              required
              spellCheck={false}
              value={serverURL}
              onChange={(event) => setServerURL(event.target.value)}
              placeholder="https://pact.example.com"
            />
            <button className={styles.primary} type="submit" disabled={busy || !serverURL.trim()}>{busy ? "Conectando…" : "Conectar servidor"}</button>
            <button className={styles.localOption} type="button" disabled={busy} onClick={() => void useLocalServer()}>
              <span>{localServer?.installed ? "Usar PACT Server local" : "Crear PACT Server local"}</span>
              <small>{localServer?.installed
                ? `${localServer.running ? "En ejecución" : "Detenido"}${localServer.server_url ? ` · ${localServer.server_url}` : ""}`
                : "Instala PACT Server, PostgreSQL y pgvector mediante Docker en este computador."}</small>
              <em>{busy ? "Preparando" : localServer?.installed ? "Disponible" : "Docker"}</em>
            </button>
            {error ? <p className={styles.error}>{error}</p> : null}
          </form>
        )}
      </section>
    </main>
  );
}

function DesktopLoading() {
  return (
    <div className={styles.loading} aria-live="polite" aria-busy="true">
      <span><i /></span>
      <strong>Comprobando PACT Desktop</strong>
      <small>Buscando una conexión segura en este computador…</small>
    </div>
  );
}

function message(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}
