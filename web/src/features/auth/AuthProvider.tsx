import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";

import { APIError, request, requestData, SESSION_EXPIRED_EVENT } from "../../api/client";
import type { InvitationPreview, Principal, SetupStatus } from "../../api/types";
import { desktopBridge } from "@/platform/desktop";

import { AuthPage } from "./AuthPage";
import { DeviceApprovalDialog } from "./DeviceApprovalDialog";
import { InvitationAcceptanceDialog } from "./InvitationAcceptanceDialog";
import styles from "./auth.module.css";

export type AuthStatus = "checking" | "authenticated" | "anonymous";
export type AuthMode = "login" | "setup" | "invitation";

export interface LoginInput {
  login: string;
  password: string;
}

export interface RegistrationInput {
  displayName: string;
  email: string;
  username: string;
  password: string;
  setupCode?: string;
}

interface AuthenticationIntent {
  invitationSecret: string;
  deviceCode: string;
}

interface AuthContextValue {
  status: AuthStatus;
  mode: AuthMode;
  principal: Principal | null;
  setupStatus: SetupStatus | null;
  invitationPreview: InvitationPreview | null;
  deviceCode: string;
  preparationError: string;
  login: (input: LoginInput) => Promise<void>;
  register: (input: RegistrationInput) => Promise<void>;
  logout: () => Promise<void>;
  refreshSession: () => Promise<void>;
  acceptInvitation: () => Promise<void>;
  dismissInvitation: () => void;
  approveDevice: () => Promise<void>;
  dismissDeviceApproval: () => void;
}

interface AuthProviderProps {
  children: ReactNode;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function readAuthenticationIntent(): AuthenticationIntent {
  const parameters = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  return {
    invitationSecret: parameters.get("invite") || "",
    deviceCode: (parameters.get("device") || "").toUpperCase(),
  };
}

function clearAuthenticationIntent(kind: "invite" | "device"): void {
  const parameters = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  parameters.delete(kind);
  const nextHash = parameters.toString();
  window.history.replaceState(
    null,
    "",
    `${window.location.pathname}${window.location.search}${nextHash ? `#${nextHash}` : ""}`,
  );
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof APIError && error.status === 401) {
    return "El usuario, el correo o la contraseña no son válidos.";
  }
  return requestErrorMessage(error, fallback);
}

function requestErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const queryClient = useQueryClient();
  const initialIntent = useMemo(readAuthenticationIntent, []);
  const [status, setStatusState] = useState<AuthStatus>("checking");
  const statusRef = useRef<AuthStatus>("checking");
  const [mode, setMode] = useState<AuthMode>(
    initialIntent.invitationSecret ? "invitation" : "login",
  );
  const [principal, setPrincipal] = useState<Principal | null>(null);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [invitationPreview, setInvitationPreview] = useState<InvitationPreview | null>(null);
  const [invitationSecret, setInvitationSecret] = useState(initialIntent.invitationSecret);
  const [deviceCode, setDeviceCode] = useState(initialIntent.deviceCode);
  const [preparationError, setPreparationError] = useState("");

  const setStatus = useCallback((nextStatus: AuthStatus) => {
    statusRef.current = nextStatus;
    setStatusState(nextStatus);
  }, []);

  const loadPrincipal = useCallback(async () => {
    const currentPrincipal = await requestData<Principal>("/v1/me");
    setPrincipal(currentPrincipal);
    setStatus("authenticated");
  }, [setStatus]);

  const prepareAnonymousSession = useCallback(async (secret: string) => {
    setPrincipal(null);

    if (secret) {
      setMode("invitation");
      try {
        const preview = await requestData<InvitationPreview>("/v1/auth/invitations/preview", {
          method: "POST",
          body: { secret },
        });
        setInvitationPreview(preview);
      } catch (error) {
        setInvitationPreview(null);
        setPreparationError(requestErrorMessage(error, "No se pudo comprobar la invitación."));
      }
      setStatus("anonymous");
      return;
    }

    try {
      const setup = await requestData<SetupStatus>("/v1/auth/setup");
      setSetupStatus(setup);
      setMode(setup.required ? "setup" : "login");
    } catch (error) {
      setSetupStatus(null);
      setMode("login");
      setPreparationError(requestErrorMessage(error, "No se pudo consultar el estado del servidor."));
    }
    setStatus("anonymous");
  }, [setStatus]);

  const refreshSession = useCallback(async () => {
    setStatus("checking");
    setPreparationError("");
    try {
      await requestData<unknown>("/v1/auth/session", { skipSessionExpirySignal: true });
      await loadPrincipal();
    } catch (error) {
      if (!(error instanceof APIError) || error.status !== 401) {
        setPreparationError(requestErrorMessage(error, "No se pudo comprobar la sesión."));
      }
      await prepareAnonymousSession(invitationSecret);
    }
  }, [invitationSecret, loadPrincipal, prepareAnonymousSession]);

  useEffect(() => {
    const handleSessionExpired = () => {
      if (statusRef.current !== "authenticated") return;
      setStatus("checking");
      setPrincipal(null);
      queryClient.clear();
      setPreparationError("La sesión venció. Inicia sesión para continuar.");
      void prepareAnonymousSession("");
    };

    window.addEventListener(SESSION_EXPIRED_EVENT, handleSessionExpired);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, handleSessionExpired);
  }, [prepareAnonymousSession, queryClient, setStatus]);

  useEffect(() => {
    let active = true;

    const boot = async () => {
      try {
        await requestData<unknown>("/v1/auth/session", { skipSessionExpirySignal: true });
        if (!active) return;
        const currentPrincipal = await requestData<Principal>("/v1/me");
        if (!active) return;
        setPrincipal(currentPrincipal);
        setStatus("authenticated");
      } catch (error) {
        if (!active) return;
        if (!(error instanceof APIError) || error.status !== 401) {
          setPreparationError(requestErrorMessage(error, "No se pudo comprobar la sesión."));
        }
        await prepareAnonymousSession(initialIntent.invitationSecret);
      }
    };

    void boot();
    return () => {
      active = false;
    };
  }, [initialIntent.invitationSecret, prepareAnonymousSession, setStatus]);

  const login = useCallback(async (input: LoginInput) => {
    queryClient.clear();
    await requestData<unknown>("/v1/auth/login", {
      method: "POST",
      body: input,
      skipSessionExpirySignal: true,
    });
    await loadPrincipal();
  }, [loadPrincipal, queryClient]);

  const register = useCallback(async (input: RegistrationInput) => {
    queryClient.clear();
    const body = {
      display_name: input.displayName,
      email: input.email,
      username: input.username,
      password: input.password,
    };

    if (mode === "setup") {
      await requestData<unknown>("/v1/auth/setup", {
        method: "POST",
        body: { ...body, setup_code: input.setupCode || "" },
      });
    } else if (mode === "invitation") {
      await requestData<unknown>("/v1/auth/invitations/register", {
        method: "POST",
        body: { ...body, secret: invitationSecret },
      });
      clearAuthenticationIntent("invite");
      setInvitationSecret("");
    } else {
      throw new Error("Este formulario no puede registrar una cuenta.");
    }

    await loadPrincipal();
  }, [invitationSecret, loadPrincipal, mode, queryClient]);

  const logout = useCallback(async () => {
    const native = desktopBridge();
    if (native) {
      await native.Disconnect(false);
      queryClient.clear();
      window.location.reload();
      return;
    }
    try {
      await request<void>("/v1/auth/session", { method: "DELETE", skipSessionExpirySignal: true });
    } finally {
      queryClient.clear();
      setPrincipal(null);
      setStatus("checking");
      setPreparationError("");
      await prepareAnonymousSession("");
    }
  }, [prepareAnonymousSession, queryClient, setStatus]);

  const acceptInvitation = useCallback(async () => {
    if (!invitationSecret) return;
    await requestData<unknown>("/v1/auth/invitations/accept", {
      method: "POST",
      body: { secret: invitationSecret },
    });
    clearAuthenticationIntent("invite");
    setInvitationSecret("");
    await queryClient.invalidateQueries();
  }, [invitationSecret, queryClient]);

  const dismissInvitation = useCallback(() => {
    clearAuthenticationIntent("invite");
    setInvitationSecret("");
  }, []);

  const approveDevice = useCallback(async () => {
    if (!deviceCode) return;
    await request<void>("/v1/auth/devices/approve", {
      method: "POST",
      body: { user_code: deviceCode },
    });
  }, [deviceCode]);

  const dismissDeviceApproval = useCallback(() => {
    clearAuthenticationIntent("device");
    setDeviceCode("");
  }, []);

  const value = useMemo<AuthContextValue>(() => ({
    status,
    mode,
    principal,
    setupStatus,
    invitationPreview,
    deviceCode,
    preparationError,
    login,
    register,
    logout,
    refreshSession,
    acceptInvitation,
    dismissInvitation,
    approveDevice,
    dismissDeviceApproval,
  }), [
    approveDevice,
    acceptInvitation,
    deviceCode,
    dismissInvitation,
    dismissDeviceApproval,
    invitationPreview,
    login,
    logout,
    mode,
    preparationError,
    principal,
    refreshSession,
    register,
    setupStatus,
    status,
  ]);

  return (
    <AuthContext.Provider value={value}>
      {status === "checking" ? (
        <AuthBootScreen />
      ) : status === "anonymous" ? (
        <AuthPage />
      ) : (
        <>
          {children}
          {invitationSecret ? <InvitationAcceptanceDialog /> : deviceCode ? <DeviceApprovalDialog /> : null}
        </>
      )}
    </AuthContext.Provider>
  );
}

function AuthBootScreen() {
  return (
    <main className={styles.boot} aria-label="Comprobando sesión" aria-busy="true">
      <span className={styles.mark} aria-hidden="true"><i /></span>
      <strong>PACT</strong>
      <span>Comprobando sesión segura…</span>
    </main>
  );
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) throw new Error("useAuth debe utilizarse dentro de AuthProvider.");
  return context;
}

export { errorMessage as authenticationErrorMessage };
