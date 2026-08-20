import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";

import { ToastProvider } from "@/components/ui";
import type { DesktopBridge } from "@/platform/desktop";
import type { LocalComputerStatus } from "@/platform/desktop";
import { WorkspaceContextProvider } from "@/features/workspaces/WorkspaceContext";

import { LocalComputerPage } from "./LocalComputerPage";

function installBridge(): DesktopBridge {
  const bridge: DesktopBridge = {
    Status: vi.fn(),
    BeginDeviceLogin: vi.fn(),
    PollDeviceLogin: vi.fn(),
    OpenExternalURL: vi.fn(),
    Disconnect: vi.fn(),
    APIRequest: vi.fn(),
    StartWorkspaceDirectoryStream: vi.fn(),
    StopWorkspaceDirectoryStream: vi.fn(),
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
    UpdateStatus: vi.fn().mockResolvedValue({
      configured: true,
      current_version: "0.16.0",
      commit: "abcdef0",
      state: "idle",
    }),
    CheckForUpdates: vi.fn(),
  };
  window.go = { main: { Desktop: bridge } };
  return bridge;
}

function renderPage(view: "overview" | "agents" | "service" = "overview") {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <WorkspaceContextProvider value={{
          workspaces: [{
            id: "workspace-1",
            name: "Footfall",
            slug: "footfall",
            projects: [{ id: "project-1", name: "footfall-web", workspace_id: "workspace-1" }],
          }],
          projects: [{ id: "project-1", name: "footfall-web", workspace_id: "workspace-1" }],
          workspaceProjects: [],
          stream: { status: "idle", events: [] },
          refreshDirectory: vi.fn().mockResolvedValue(undefined),
        }}>
          <LocalComputerPage view={view} />
        </WorkspaceContextProvider>
      </ToastProvider>
    </MemoryRouter>,
  );
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

    renderPage();

    expect(await screen.findByText("Mac de Jorge")).toBeInTheDocument();
    expect(screen.getByText("Checkouts recordados por este computador")).toBeInTheDocument();
  });

  it("conecta Codex a una carpeta elegida por el usuario", async () => {
    const user = userEvent.setup();
    const bridge = installBridge();
    renderPage("agents");

    expect(await screen.findByText("Codex")).toBeInTheDocument();
    await user.click(screen.getAllByRole("button", { name: "Conectar" })[0]);
    await user.click(screen.getByRole("button", { name: /Elegir carpeta/ }));

    expect((await screen.findAllByText("Footfall")).length).toBeGreaterThan(0);
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Conectar cliente" }));

    expect(bridge.ConnectLocalAgent).toHaveBeenCalledWith({
      client: "codex",
      project_root: "/projects/footfall",
    });
    expect(await screen.findByText("Cliente conectado")).toBeInTheDocument();
  });

  it("permite comprobar una actualización firmada desde la aplicación", async () => {
    const user = userEvent.setup();
    const bridge = installBridge();
    renderPage("service");

    await user.click(await screen.findByRole("button", { name: "Buscar actualizaciones" }));

    expect(bridge.CheckForUpdates).toHaveBeenCalledOnce();
  });
});
