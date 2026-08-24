import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";

import { ToastProvider } from "@/components/ui/Toast";
import { WorkspaceContextProvider } from "@/features/workspaces/WorkspaceContext";

import { PeoplePage } from "./AccessPages";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("PeoplePage", () => {
  it("muestra al propietario aunque el workspace todavía no tenga repositorios", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      data: {
        workspace_id: "ws-empty",
        members: [{
          principal_id: "owner-1",
          display_name: "Jorge",
          principal_type: "human",
          status: "active",
          organization_role: "owner",
          effective_role: "owner",
          access_source: "organization",
        }],
        agents: [],
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter><ToastProvider><WorkspaceContextProvider value={{
          workspaces: [],
          projects: [],
          workspaceProjects: [],
          workspace: { id: "ws-empty", name: "Magi", slug: "magi" },
          principal: { id: "owner-1", organization_role: "owner" },
          stream: { status: "idle", events: [] },
          refreshDirectory: async () => undefined,
        }}>
          <PeoplePage />
        </WorkspaceContextProvider></ToastProvider></MemoryRouter>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(screen.getByText("Jorge")).toBeInTheDocument());
    expect(screen.getByText("Aún no hay agentes registrados")).toBeInTheDocument();
    expect(screen.getByText(/Añade Codex o Claude para autorizarlo/)).toBeInTheDocument();
    expect(screen.queryByText("Workspace sin unidad operativa")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/v1/workspaces/ws-empty/access", expect.anything());
  });

  it("muestra Codex como pendiente inmediatamente después de autorizarlo", async () => {
    let enrolled = false;
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path.endsWith("/agent-enrollments") && init?.method === "POST") {
        enrolled = true;
        return new Response(JSON.stringify({ data: { created: true, enrollment: {
          id: "enrollment-1", workspace_id: "ws-magi", project_id: "project-magi", agent_id: "agent-codex",
          agent_name: "Codex", sponsor_principal_id: "owner-1", agent_type: "codex", client_type: "codex-mcp",
          status: "pending", created_at: "2026-08-24T12:00:00Z", updated_at: "2026-08-24T12:00:00Z",
        } } }), { status: 201, headers: { "Content-Type": "application/json" } });
      }
      return new Response(JSON.stringify({ data: {
        workspace_id: "ws-magi",
        members: [{ principal_id: "owner-1", display_name: "Jorge", principal_type: "human", status: "active", organization_role: "owner", effective_role: "owner", access_source: "organization" }],
        agents: enrolled ? [{ agent_id: "agent-codex", display_name: "Codex", agent_type: "codex", status: "active", sponsor_principal_id: "owner-1", sponsor_display_name: "Jorge", sponsor_effective_role: "owner", access_active: true, connected: false, active_sessions: 0, session_count: 0, enrollment_count: 1, pending_enrollments: 1, enrollment_project_id: "project-magi" }] : [],
      } }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={queryClient}><MemoryRouter><ToastProvider><WorkspaceContextProvider value={{
      workspaces: [], projects: [], workspaceProjects: [{ id: "project-magi", name: "magi", workspace_id: "ws-magi" }],
      workspace: { id: "ws-magi", name: "Magi", slug: "magi" }, principal: { id: "owner-1", organization_role: "owner" },
      stream: { status: "idle", events: [] }, refreshDirectory: async () => undefined,
    }}><PeoplePage /></WorkspaceContextProvider></ToastProvider></MemoryRouter></QueryClientProvider>);

    await screen.findByText("Jorge");
    fireEvent.click(screen.getByRole("button", { name: "Agregar" }));
    fireEvent.click(screen.getByRole("button", { name: /Agregar agente de IA/ }));
    const repositorySelect = screen.getByLabelText(/Repositorio técnico/);
    await waitFor(() => expect(repositorySelect).toHaveValue("project-magi"));
    const submitButton = screen.getByRole("button", { name: "Agregar agente" });
    await waitFor(() => expect(submitButton).not.toBeDisabled());
    fireEvent.submit(submitButton.closest("form")!);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/v1/workspaces/ws-magi/agent-enrollments", expect.objectContaining({ method: "POST" })));
    await screen.findByText("Codex está pendiente");
    fireEvent.click(screen.getByRole("button", { name: "Cerrar" }));
    await waitFor(() => expect(screen.getByText("Pendiente de iniciar")).toBeInTheDocument());
    expect(screen.getByText("Nunca")).toBeInTheDocument();
  });
});
