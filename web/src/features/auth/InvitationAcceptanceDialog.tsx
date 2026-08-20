import { useEffect, useRef, useState } from "react";

import { authenticationErrorMessage, useAuth } from "./AuthProvider";
import styles from "./auth.module.css";

export function InvitationAcceptanceDialog() {
  const { principal, acceptInvitation, dismissInvitation } = useAuth();
  const dialog = useRef<HTMLDialogElement>(null);
  const [accepting, setAccepting] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const element = dialog.current;
    if (!element || element.open) return;
    if (typeof element.showModal === "function") element.showModal();
    else element.setAttribute("open", "");
  }, []);

  async function accept() {
    setAccepting(true);
    setError("");
    try {
      await acceptInvitation();
    } catch (cause) {
      setAccepting(false);
      setError(authenticationErrorMessage(cause, "No se pudo aceptar la invitación."));
    }
  }

  return (
    <dialog
      ref={dialog}
      className={styles.dialog}
      aria-labelledby="invitation-acceptance-title"
      aria-describedby="invitation-acceptance-description"
      onCancel={(event) => {
        event.preventDefault();
        dismissInvitation();
      }}
    >
      <div className={styles.dialogContent}>
        <header>
          <p className={styles.eyebrow}>INVITACIÓN PENDIENTE</p>
          <h2 id="invitation-acceptance-title">Añadir acceso a tu cuenta</h2>
        </header>
        <p id="invitation-acceptance-description">
          La invitación se añadirá a la cuenta de {principal?.display_name || principal?.username || "la sesión actual"}.
        </p>
        {error ? <p className={styles.error} role="alert">{error}</p> : null}
        <footer>
          <button className={styles.secondary} type="button" disabled={accepting} onClick={dismissInvitation}>
            Ahora no
          </button>
          <button className={styles.submit} type="button" disabled={accepting} onClick={() => void accept()}>
            {accepting ? "Aceptando…" : "Aceptar invitación"}
          </button>
        </footer>
      </div>
    </dialog>
  );
}
