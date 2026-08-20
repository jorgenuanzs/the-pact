import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DesktopBridge } from "@/platform/desktop";

import { DesktopGate } from "./DesktopGate";

function installDesktopBridge(overrides: Partial<DesktopBridge> = {}): DesktopBridge {
  const bridge: DesktopBridge = {
    Status: vi.fn().mockResolvedValue({
      configured: false,
      connected: false,
      default_url: "",
    }),
    BeginDeviceLogin: vi.fn().mockResolvedValue({
      server_url: "https://pact.example.com",
      device_code: "device-code",
      user_code: "PACT-42",
      verification_url: "https://pact.example.com/device",
      expires_at: new Date(Date.now() + 60_000).toISOString(),
      interval_seconds: 30,
    }),
    PollDeviceLogin: vi.fn().mockResolvedValue({ status: "pending", connected: false }),
    OpenExternalURL: vi.fn().mockResolvedValue(undefined),
    Disconnect: vi.fn().mockResolvedValue(undefined),
    LocalComputerStatus: vi.fn().mockResolvedValue({
      hostname: "Test",
      operating_system: "darwin",
      architecture: "arm64",
      runtime_ready: true,
      clients: [],
      folders: [],
    }),
    SelectLocalProjectFolder: vi.fn().mockResolvedValue({ canceled: true, connected: false }),
    InspectLocalProjectFolder: vi.fn().mockResolvedValue({ canceled: false, connected: false }),
    ConnectLocalAgent: vi.fn(),
    LocalServerStatus: vi.fn().mockResolvedValue({ installed: false, running: false, ready: false }),
    InstallLocalServer: vi.fn().mockResolvedValue({
      status: { installed: true, running: true, ready: true, server_url: "http://127.0.0.1:8080" },
      setup_code: "local-setup-code",
    }),
    StartLocalServer: vi.fn(),
    StopLocalServer: vi.fn(),
    BackupLocalServer: vi.fn(),
    UpgradeLocalServer: vi.fn(),
    UpdateStatus: vi.fn().mockResolvedValue({
      configured: false,
      current_version: "dev",
      commit: "unknown",
      state: "unconfigured",
    }),
    CheckForUpdates: vi.fn(),
    APIRequest: vi.fn(),
    StartWorkspaceDirectoryStream: vi.fn(),
    StopWorkspaceDirectoryStream: vi.fn(),
    StartProjectEventStream: vi.fn(),
    StopProjectEventStream: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  window.go = { main: { Desktop: bridge } };
  return bridge;
}

afterEach(() => {
  cleanup();
  delete window.go;
  vi.restoreAllMocks();
});

describe("DesktopGate", () => {
  it("deja pasar la aplicación web cuando no existe el runtime nativo", () => {
    render(<DesktopGate><span>PACT Control</span></DesktopGate>);
    expect(screen.getByText("PACT Control")).toBeInTheDocument();
  });

  it("solicita un servidor sin asumir una URL cuando el equipo aún no está configurado", async () => {
    installDesktopBridge();
    render(<DesktopGate><span>Área conectada</span></DesktopGate>);

    expect(await screen.findByRole("heading", { name: "¿A qué PACT Server quieres conectarte?" })).toBeInTheDocument();
    expect(screen.getByLabelText("URL del servidor")).toHaveValue("");
    expect(screen.getByRole("button", { name: "Conectar servidor" })).toBeDisabled();
    expect(screen.queryByText("Área conectada")).not.toBeInTheDocument();
  });

  it("inicia el device flow y muestra el código que debe confirmar el usuario", async () => {
    const bridge = installDesktopBridge();
    render(<DesktopGate><span>Área conectada</span></DesktopGate>);

    const connect = await screen.findByRole("button", { name: "Conectar servidor" });
    fireEvent.change(screen.getByLabelText("URL del servidor"), {
      target: { value: "https://pact.example.com" },
    });
    fireEvent.click(connect);

    expect(await screen.findByText("PACT-42")).toBeInTheDocument();
    expect(bridge.BeginDeviceLogin).toHaveBeenCalledWith("https://pact.example.com");
    expect(screen.getByRole("button", { name: "Abrir PACT Server" })).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Conectando…")).not.toBeInTheDocument());
  });

  it("instala un servidor local y comienza su autorización", async () => {
    const bridge = installDesktopBridge();
    render(<DesktopGate><span>Área conectada</span></DesktopGate>);

    fireEvent.click(await screen.findByRole("button", { name: /Crear PACT Server local/ }));

    expect(await screen.findByText("local-setup-code")).toBeInTheDocument();
    expect(bridge.InstallLocalServer).toHaveBeenCalledWith({ port: 8080 });
    expect(bridge.BeginDeviceLogin).toHaveBeenCalledWith("http://127.0.0.1:8080");
  });
});
