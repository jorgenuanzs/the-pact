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
    ListServerProfiles: vi.fn(),
    UseServerProfile: vi.fn(),
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
      runtime_version: "0.16.0",
      runtime_digest: "abc123abc123",
      mcp_migrated: 0,
      mcp_migration_errors: [],
      server_url: "https://pact.example.com",
      active_profile_id: "profile-1",
      profiles: [{
        id: "profile-1", label: "PACT Example", server_url: "https://pact.example.com",
        kind: "remote", principal_label: "Jorge", active: true, reachable: true,
        version: "0.16.0", commit: "abcdef012345", protocol_version: 1,
        compatibility: "compatible", update_available: false,
      }],
      clients: [
        { id: "codex", name: "Codex", detected: true, connected_folders: 0 },
        { id: "claude", name: "Claude Code", detected: false, connected_folders: 0 },
      ],
      folders: [],
      managed_server: { installed: false, running: false, ready: false },
    }),
    SelectLocalProjectFolder: vi.fn().mockResolvedValue({
      canceled: false,
      connected: false,
      root: "/projects/footfall",
      name: "Footfall",
      remote_url: "https://github.com/nuanzs/footfall",
      branch: "main",
      clients: [],
    }),
    InspectLocalProjectFolder: vi.fn(),
    ResolveLocalFolder: vi.fn().mockResolvedValue({
      folder: {
        canceled: false,
        connected: false,
        root: "/projects/footfall",
        name: "Footfall",
        remote_url: "https://github.com/nuanzs/footfall",
      },
      profile: { id: "profile-1", label: "PACT Example", server_url: "https://pact.example.com", kind: "remote", active: true },
      workspaces: [{ id: "workspace-1", name: "Footfall", slug: "footfall" }],
      matches: [{
        workspace_id: "workspace-1", workspace_name: "Footfall", workspace_slug: "footfall",
        project_id: "project-1", project_name: "footfall-web", repository_id: "repository-1",
        repository_name: "footfall-web", repository_slug: "primary", primary: true, match: "exact",
      }],
    }),
    BindLocalFolder: vi.fn().mockResolvedValue({
      folder: { canceled: false, connected: true, root: "/projects/footfall", name: "Footfall" },
      clients: [{
        client: "codex", project_root: "/projects/footfall",
        config_path: "/projects/footfall/.codex/config.toml", runtime_path: "/local/pact-runtime",
        changed: true, restart_needed: true,
      }],
      created: false,
    }),
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

function renderPage(view: "overview" | "connections" | "agents" | "folders" | "service" = "overview") {
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
    await user.click(screen.getByRole("button", { name: "Configurar Codex en una carpeta" }));
    await user.click(screen.getByRole("button", { name: /Elegir carpeta Git/ }));

    expect((await screen.findAllByText("Footfall")).length).toBeGreaterThan(0);
    const dialog = within(screen.getByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: "Continuar" }));
    await user.click(dialog.getByRole("button", { name: "Continuar" }));
    expect(await dialog.findByText("footfall-web")).toBeInTheDocument();
    await user.click(dialog.getByRole("button", { name: "Continuar" }));
    await user.click(dialog.getByRole("button", { name: "Continuar" }));
    await user.click(dialog.getByRole("button", { name: "Conectar carpeta" }));

    expect(bridge.BindLocalFolder).toHaveBeenCalledWith(expect.objectContaining({
      project_root: "/projects/footfall",
      profile_id: "profile-1",
      workspace_id: "workspace-1",
      project_id: "project-1",
      repository_id: "repository-1",
      clients: ["codex"],
    }));
    expect(await screen.findByText("Carpeta conectada")).toBeInTheDocument();
  });

  it("ubica la conexión de carpetas en las secciones locales y no en el encabezado global", async () => {
    installBridge();
    const { unmount } = renderPage("agents");

    expect(await screen.findByText("Aplicaciones disponibles en este equipo")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Conectar cliente" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Configurar Codex en una carpeta" })).toBeInTheDocument();

    unmount();
    renderPage("folders");
    expect(await screen.findByRole("button", { name: "Conectar una carpeta" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Conectar cliente" })).not.toBeInTheDocument();
  });

  it("permite comprobar una actualización firmada desde la aplicación", async () => {
    const user = userEvent.setup();
    const bridge = installBridge();
    renderPage("service");

    await user.click(await screen.findByRole("button", { name: "Buscar actualizaciones" }));

    expect(bridge.CheckForUpdates).toHaveBeenCalledOnce();
  });

  it("muestra la versión y compatibilidad de cada PACT Server", async () => {
    installBridge();
    renderPage("connections");

    expect(await screen.findByText("Compatible")).toBeInTheDocument();
    expect(screen.getByText("0.16.0")).toBeInTheDocument();
  });
});
