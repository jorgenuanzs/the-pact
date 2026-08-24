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
    ListServerProfiles: vi.fn().mockResolvedValue([]),
    UseServerProfile: vi.fn(),
    LocalComputerStatus: vi.fn().mockResolvedValue({
      hostname: "Test",
      operating_system: "darwin",
      architecture: "arm64",
      runtime_ready: true,
      profiles: [],
      clients: [],
      folders: [],
    }),
    SelectLocalProjectFolder: vi.fn().mockResolvedValue({ canceled: true, connected: false }),
    InspectLocalProjectFolder: vi.fn().mockResolvedValue({ canceled: false, connected: false }),
    ResolveLocalFolder: vi.fn(),
    BindLocalFolder: vi.fn(),
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

  it("explica que cada cuenta pertenece a un servidor antes de pedir una URL", async () => {
    installDesktopBridge();
    render(<DesktopGate><span>Área conectada</span></DesktopGate>);

    expect(await screen.findByRole("heading", { name: "Elige dónde vive tu proyecto" })).toBeInTheDocument();
    expect(screen.getByText("Las cuentas pertenecen a un servidor")).toBeInTheDocument();
    expect(screen.queryByLabelText("URL del servidor")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Conectar un servidor existente/ }));

    expect(await screen.findByRole("heading", { name: "Conecta tu PACT Server" })).toBeInTheDocument();
    expect(screen.getByLabelText("URL del servidor")).toHaveValue("");
    expect(screen.getByRole("button", { name: "Conectar servidor" })).toBeDisabled();
    expect(screen.queryByText("Área conectada")).not.toBeInTheDocument();
  });

  it("inicia el device flow y muestra el código que debe confirmar el usuario", async () => {
    const bridge = installDesktopBridge();
    render(<DesktopGate><span>Área conectada</span></DesktopGate>);

    fireEvent.click(await screen.findByRole("button", { name: /Conectar un servidor existente/ }));
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

  it("no instala un servidor local hasta recibir una confirmación explícita", async () => {
    const bridge = installDesktopBridge();
    render(<DesktopGate><span>Área conectada</span></DesktopGate>);

    fireEvent.click(await screen.findByRole("button", { name: /Crear un servidor local/ }));

    expect(await screen.findByRole("heading", { name: "Crea PACT en este computador" })).toBeInTheDocument();
    expect(screen.getByText("Cuenta independiente")).toBeInTheDocument();
    expect(bridge.InstallLocalServer).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Confirmar e instalar" })).toBeDisabled();

    fireEvent.click(screen.getByLabelText(/Confirmo que quiero crear este servidor local/));
    fireEvent.click(screen.getByRole("button", { name: "Confirmar e instalar" }));

    expect(await screen.findByText("local-setup-code")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Volver (el servidor seguirá instalado)" })).toBeInTheDocument();
    expect(bridge.InstallLocalServer).toHaveBeenCalledWith({ port: 8080 });
    expect(bridge.BeginDeviceLogin).toHaveBeenCalledWith("http://127.0.0.1:8080");
  });

  it("permite volver desde la explicación local sin instalar nada", async () => {
    const bridge = installDesktopBridge();
    render(<DesktopGate><span>Área conectada</span></DesktopGate>);

    fireEvent.click(await screen.findByRole("button", { name: /Crear un servidor local/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Volver sin instalar" }));

    expect(await screen.findByRole("heading", { name: "Elige dónde vive tu proyecto" })).toBeInTheDocument();
    expect(bridge.InstallLocalServer).not.toHaveBeenCalled();
  });
});
