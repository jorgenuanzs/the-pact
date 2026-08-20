import { useEffect, useRef, useState } from "react";

import { authenticationErrorMessage, useAuth } from "./AuthProvider";
import styles from "./auth.module.css";

export function DeviceApprovalDialog() {
  const { deviceCode, approveDevice, dismissDeviceApproval } = useAuth();
  const dialog = useRef<HTMLDialogElement>(null);
  const [state, setState] = useState<"idle" | "approving" | "approved">("idle");
  const [error, setError] = useState("");

  useEffect(() => {
    const element = dialog.current;
    if (!element || element.open) return;
    if (typeof element.showModal === "function") element.showModal();
    else element.setAttribute("open", "");
  }, []);

  async function approve() {
    setState("approving");
    setError("");
    try {
      await approveDevice();
      setState("approved");
    } catch (cause) {
      setState("idle");
      setError(authenticationErrorMessage(cause, "No se pudo autorizar el dispositivo."));
    }
  }

  return (
    <dialog
      ref={dialog}
      className={styles.dialog}
      aria-labelledby="device-approval-title"
      onCancel={(event) => {
        event.preventDefault();
        dismissDeviceApproval();
      }}
    >
      <div className={styles.dialogContent}>
        <header>
          <p className={styles.eyebrow}>AUTORIZACIÓN DE HERRAMIENTA</p>
          <h2 id="device-approval-title">
            {state === "approved" ? "Dispositivo autorizado" : "Autoriza este dispositivo"}
          </h2>
        </header>

        {state === "approved" ? (
          <p>La herramienta ya puede completar la conexión con PACT.</p>
        ) : (
          <>
            <p>Comprueba que este código coincide con el mostrado por tu CLI o agente.</p>
            <code className={styles.deviceCode}>{deviceCode}</code>
          </>
        )}

        {error ? <p className={styles.error} role="alert">{error}</p> : null}

        <footer>
          {state === "approved" ? (
            <button className={styles.submit} type="button" onClick={dismissDeviceApproval}>
              Cerrar
            </button>
          ) : (
            <>
              <button className={styles.secondary} type="button" onClick={dismissDeviceApproval}>
                Cancelar
              </button>
              <button
                className={styles.submit}
                type="button"
                disabled={state === "approving"}
                onClick={() => void approve()}
              >
                {state === "approving" ? "Autorizando…" : "Autorizar dispositivo"}
              </button>
            </>
          )}
        </footer>
      </div>
    </dialog>
  );
}
