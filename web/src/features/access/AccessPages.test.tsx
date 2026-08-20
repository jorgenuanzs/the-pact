import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { WorkspaceContextProvider } from "@/features/workspaces/WorkspaceContext";

import { PeoplePage } from "./AccessPages";

afterEach(() => vi.restoreAllMocks());

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
        <WorkspaceContextProvider value={{
          workspaces: [],
          projects: [],
          workspaceProjects: [],
          workspace: { id: "ws-empty", name: "Magi", slug: "magi" },
          principal: { id: "owner-1", organization_role: "owner" },
          stream: { status: "idle", events: [] },
          refreshDirectory: async () => undefined,
        }}>
          <PeoplePage />
        </WorkspaceContextProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(screen.getByText("Jorge")).toBeInTheDocument());
    expect(screen.getByText("No hay agentes autorizados")).toBeInTheDocument();
    expect(screen.queryByText("Workspace sin unidad operativa")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/v1/workspaces/ws-empty/access", expect.anything());
  });
});
