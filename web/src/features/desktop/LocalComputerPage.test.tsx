import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";

import { ToastProvider } from "@/components/ui";
import type { DesktopBridge } from "@/platform/desktop";
import type { LocalComputerStatus } from "@/platform/desktop";

import { LocalComputerPage } from "./LocalComputerPage";

function installBridge(): DesktopBridge {
  const bridge: DesktopBridge = {
    Status: vi.fn(),
    BeginDeviceLogin: vi.fn(),
    PollDeviceLogin: vi.fn(),
    OpenExternalURL: vi.fn(),
    Disconnect: vi.fn(),
    APIRequest: vi.fn(),
    StartProjectEventStream: vi.fn(),
    StopProjectEventStream: vi.fn(),
    LocalComputerStatus: vi.fn().mockResolvedValue({
      hostname: "Mac de Jorge",
      operating_system: "darwin",
      architecture: "arm64",
      runtime_ready: true,
      runtime_path: "/local/pact-runtime",
      runtime_version: "abc123",
      server_url: "https://pact.example.com",
      clients: [
        { id: "codex", name: "Codex", detected: true, connected_folders: 0 },
        { id: "claude", name: "Claude Code", detected: false, connected_folders: 0 },
      ],
      folders: [],
      managed_server: { installed: false, running: false, ready: false },
    }),
    SelectLocalProjectFolder: vi.fn().mockResolvedValue({
      canceled: false,
      connected: true,
      root: "/projects/footfall",
      name: "Footfall",
      server_url: "https://pact.example.com",
      project_id: "project-1",
      clients: [],
    }),
    InspectLocalProjectFolder: vi.fn(),
    ConnectLocalAgent: vi.fn().mockResolvedValue({
      client: "codex",
      project_root: "/projects/footfall",
      config_path: "/projects/footfall/.codex/config.toml",
      runtime_path: "/local/pact-runtime",
      changed: true,
      restart_needed: true,
    }),
    LocalServerStatus: vi.fn().mockResolvedValue({ installed: false, running: false, ready: false }),
    InstallLocalServer: vi.fn(),
    StartLocalServer: vi.fn(),
    StopLocalServer: vi.fn(),
    BackupLocalServer: vi.fn(),
    UpgradeLocalServer: vi.fn(),
  };
  window.go = { main: { Desktop: bridge } };
  return bridge;
}

afterEach(() => {
  cleanup();
  delete window.go;
  vi.restoreAllMocks();
});

describe("LocalComputerPage", () => {
  it("tolera colecciones nulas de una compilación nativa anterior", async () => {
    const bridge = installBridge();
    vi.mocked(bridge.LocalComputerStatus).mockResolvedValue({
      hostname: "Mac de Jorge",
      operating_system: "darwin",
      architecture: "arm64",
      runtime_ready: true,
      clients: null,
      folders: null,
    } as unknown as LocalComputerStatus);

    render(
      <MemoryRouter>
        <ToastProvider><LocalComputerPage /></ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("Mac de Jorge")).toBeInTheDocument();
    expect(screen.getByText("Checkouts recordados por este computador")).toBeInTheDocument();
  });

  it("conecta Codex a una carpeta elegida por el usuario", async () => {
    const user = userEvent.setup();
    const bridge = installBridge();
    render(
      <MemoryRouter>
        <ToastProvider><LocalComputerPage view="agents" /></ToastProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("Codex")).toBeInTheDocument();
    await user.click(screen.getAllByRole("button", { name: "Conectar" })[0]);
    await user.click(screen.getByRole("button", { name: /Elegir carpeta/ }));

    expect((await screen.findAllByText("Footfall")).length).toBeGreaterThan(0);
    await user.click(screen.getByRole("button", { name: "Instalar integración" }));

    expect(bridge.ConnectLocalAgent).toHaveBeenCalledWith({
      client: "codex",
      project_root: "/projects/footfall",
    });
    expect(await screen.findByText("Agente conectado")).toBeInTheDocument();
  });
});
