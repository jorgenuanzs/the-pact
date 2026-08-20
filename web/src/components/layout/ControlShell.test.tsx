import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";

import type { Workspace } from "@/api/types";
import { ToastProvider } from "@/components/ui";
import { useAuth } from "@/features/auth";
import { useControlDirectory } from "@/features/workspaces/queries";
import { desktopBridge, isDesktopRuntime, type DesktopBridge } from "@/platform/desktop";

import { ControlShell } from "./ControlShell";

vi.mock("@/features/auth", () => ({ useAuth: vi.fn() }));
vi.mock("@/platform/desktop", () => ({
  desktopBridge: vi.fn(() => null),
  isDesktopRuntime: vi.fn(() => false),
}));
vi.mock("@/realtime/useWorkspaceDirectoryEvents", () => ({ useWorkspaceDirectoryEvents: vi.fn() }));
vi.mock("@/features/workspaces/queries", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/workspaces/queries")>();
  return { ...actual, useControlDirectory: vi.fn() };
});
vi.mock("./CreateWorkspaceDialog", () => ({ CreateWorkspaceDialog: () => null }));

const workspace: Workspace = {
  id: "ws-1",
  name: "Footfall",
  slug: "footfall",
  color: "#c9ee4d",
};

const mockedUseAuth = vi.mocked(useAuth);
const mockedUseControlDirectory = vi.mocked(useControlDirectory);
const mockedDesktopBridge = vi.mocked(desktopBridge);
const mockedIsDesktopRuntime = vi.mocked(isDesktopRuntime);

function renderShell(path: string, workspaces: Workspace[]) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  mockedUseControlDirectory.mockReturnValue({
    workspaces,
    projects: [],
    github: undefined,
    principal: { id: "owner-1", display_name: "Jorge", organization_role: "owner" },
    isPending: false,
    error: null,
    refetch: vi.fn(async () => ({ workspaces, projects: [], github: undefined, principal: undefined })),
  } as unknown as ReturnType<typeof useControlDirectory>);

  render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route element={<ControlShell />}>
              <Route path="*" element={<div>Contenido de prueba</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </ToastProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockedDesktopBridge.mockReturnValue(null);
  mockedIsDesktopRuntime.mockReturnValue(false);
  mockedUseAuth.mockReturnValue({
    principal: { id: "owner-1", display_name: "Jorge", organization_role: "owner" },
    logout: vi.fn(async () => undefined),
  } as unknown as ReturnType<typeof useAuth>);
});

afterEach(() => {
  cleanup();
  document.body.classList.remove("pact-scroll-lock");
  vi.clearAllMocks();
});

describe("ControlShell", () => {
  it("marca el shell sin workspace para que el contenido use todo el ancho", () => {
    renderShell("/", []);

    const shell = screen.getByText("Contenido de prueba").closest(".control-shell");
    expect(shell).not.toHaveAttribute("data-workspace");
  });

  it("abre y cierra la navegación móvil con foco y Escape", async () => {
    const user = userEvent.setup();
    renderShell("/w/ws-1/settings", [workspace]);

    const trigger = screen.getByRole("button", { name: "Abrir navegación del workspace" });
    const navigation = screen.getByRole("complementary", { name: "Navegación de Footfall" });

    await user.click(trigger);

    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(navigation).toHaveAttribute("data-mobile-open");
    expect(screen.getByRole("link", { name: "Resumen" })).toHaveFocus();
    expect(document.body).toHaveClass("pact-scroll-lock");

    await user.keyboard("{Escape}");

    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(navigation).not.toHaveAttribute("data-mobile-open");
    expect(trigger).toHaveFocus();
    expect(document.body).not.toHaveClass("pact-scroll-lock");
  });

  it("abre la administración sensible del servidor en el navegador desde Desktop", async () => {
    const user = userEvent.setup();
    const openExternalURL = vi.fn(async () => undefined);
    mockedIsDesktopRuntime.mockReturnValue(true);
    mockedDesktopBridge.mockReturnValue({
      Status: vi.fn(async () => ({
        configured: true,
        connected: true,
        server_url: "https://pact.example.com",
        default_url: "",
      })),
      OpenExternalURL: openExternalURL,
    } as unknown as DesktopBridge);
    renderShell("/", []);

    await user.click(screen.getByRole("button", { name: "Abrir menú de Jorge" }));
    await user.click(screen.getByRole("menuitem", { name: /Acceso y seguridad/ }));

    expect(openExternalURL).toHaveBeenCalledWith("https://pact.example.com/admin/organization/access");
  });
});
