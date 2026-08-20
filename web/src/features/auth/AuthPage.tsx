import { useEffect, useRef, useState, type FormEvent } from "react";

import { authenticationErrorMessage, useAuth } from "./AuthProvider";
import styles from "./auth.module.css";

export function AuthPage() {
  const {
    mode,
    setupStatus,
    invitationPreview,
    deviceCode,
    preparationError,
    login,
    register,
    refreshSession,
  } = useAuth();
  const firstField = useRef<HTMLInputElement>(null);
  const [submitting, setSubmitting] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const createsAccount = mode === "setup" || mode === "invitation";

  useEffect(() => {
    firstField.current?.focus();
  }, [mode]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    setSubmitting(true);
    setError("");
    try {
      if (createsAccount) {
        await register({
          displayName: String(data.get("displayName") || "").trim(),
          email: String(data.get("email") || "").trim(),
          username: String(data.get("username") || "").trim(),
          password: String(data.get("password") || ""),
          setupCode: String(data.get("setupCode") || "").trim(),
        });
      } else {
        await login({
          login: String(data.get("login") || "").trim(),
          password: String(data.get("password") || ""),
        });
      }
    } catch (cause) {
      setError(authenticationErrorMessage(cause, "No se pudo iniciar la sesión."));
    } finally {
      setSubmitting(false);
    }
  }

  const copy = modeCopy(mode, deviceCode);
  const setupUnavailable = mode === "setup" && setupStatus?.configured === false;

  return (
    <main className={styles.page}>
      <section className={styles.introduction} aria-labelledby="auth-title">
        <div className={styles.brand}>
          <span className={styles.mark} aria-hidden="true"><i /></span>
          <strong>PACT</strong>
          <span>Control</span>
        </div>
        <p className={styles.eyebrow}>CONTEXTO COMPARTIDO · COORDINACIÓN EN VIVO</p>
        <h1 id="auth-title">Personas y agentes trabajando con la misma realidad.</h1>
        <p>
          Entra al centro de control para consultar workspaces, conversaciones,
          repositorios, permisos y actividad en tiempo real.
        </p>
        <div className={styles.serverStatus}>
          <i aria-hidden="true" />
          <span>PACT Server</span>
          <strong>Disponible en este origen</strong>
        </div>
      </section>

      <section className={styles.formRegion} aria-labelledby="auth-form-title">
        <form className={styles.card} onSubmit={submit}>
          <header>
            <p className={styles.eyebrow}>{copy.kicker}</p>
            <h2 id="auth-form-title">{copy.title}</h2>
            <p>{copy.description}</p>
          </header>

          {mode === "setup" ? (
            <label className={styles.field}>
              <span>Código de configuración</span>
              <input
                ref={firstField}
                name="setupCode"
                type="password"
                autoComplete="off"
                minLength={24}
                required
              />
            </label>
          ) : null}

          {createsAccount ? (
            <div className={styles.registrationFields}>
              <label className={styles.field}>
                <span>Nombre visible</span>
                <input
                  ref={mode === "invitation" ? firstField : undefined}
                  name="displayName"
                  autoComplete="name"
                  maxLength={200}
                  required
                />
              </label>
              <label className={styles.field}>
                <span>Correo</span>
                <input
                  name="email"
                  type="email"
                  autoComplete="email"
                  maxLength={320}
                  defaultValue={invitationPreview?.email || ""}
                  readOnly={mode === "invitation" && Boolean(invitationPreview?.email)}
                  required
                />
              </label>
              <label className={styles.field}>
                <span>Usuario</span>
                <input
                  name="username"
                  autoComplete="username"
                  minLength={3}
                  maxLength={32}
                  pattern="[a-z0-9][a-z0-9._-]{2,31}"
                  spellCheck={false}
                  required
                />
              </label>
            </div>
          ) : (
            <label className={styles.field}>
              <span>Usuario o correo</span>
              <input
                ref={firstField}
                name="login"
                autoComplete="username"
                spellCheck={false}
                required
              />
            </label>
          )}

          <label className={styles.field}>
            <span>Contraseña</span>
            <span className={styles.passwordControl}>
              <input
                name="password"
                type={showPassword ? "text" : "password"}
                autoComplete={createsAccount ? "new-password" : "current-password"}
                minLength={createsAccount ? 15 : undefined}
                maxLength={128}
                required
              />
              <button
                type="button"
                aria-label={showPassword ? "Ocultar contraseña" : "Mostrar contraseña"}
                aria-pressed={showPassword}
                onClick={() => setShowPassword((visible) => !visible)}
              >
                {showPassword ? "Ocultar" : "Mostrar"}
              </button>
            </span>
            {createsAccount ? <small>Utiliza al menos 15 caracteres.</small> : null}
          </label>

          {preparationError || error ? (
            <div className={styles.error} role="alert">
              <span>{error || preparationError}</span>
              {preparationError ? (
                <button type="button" onClick={() => void refreshSession()}>Reintentar</button>
              ) : null}
            </div>
          ) : null}

          <button
            className={styles.submit}
            type="submit"
            disabled={submitting || setupUnavailable}
          >
            <span>{submitting ? "Verificando acceso…" : copy.action}</span>
            <span aria-hidden="true">→</span>
          </button>

          {setupUnavailable ? (
            <p className={styles.configurationWarning} role="status">
              El servidor necesita configurar <code>PACT_SETUP_TOKEN</code> antes de crear la primera cuenta.
            </p>
          ) : null}
        </form>
      </section>
    </main>
  );
}

function modeCopy(mode: "login" | "setup" | "invitation", deviceCode: string) {
  if (mode === "setup") {
    return {
      kicker: "PRIMER ACCESO",
      title: "Crea la cuenta propietaria",
      description: "El código de configuración solo puede utilizarse una vez.",
      action: "Crear cuenta propietaria",
    };
  }
  if (mode === "invitation") {
    return {
      kicker: "INVITACIÓN",
      title: "Únete a PACT",
      description: "Crea tu cuenta para aceptar la invitación y entrar al workspace.",
      action: "Crear cuenta y entrar",
    };
  }
  return {
    kicker: "ACCESO SEGURO",
    title: "Entrar",
    description: deviceCode
      ? "Inicia sesión para revisar la autorización solicitada por tu herramienta."
      : "Usa tu usuario o correo y contraseña.",
    action: "Iniciar sesión",
  };
}
