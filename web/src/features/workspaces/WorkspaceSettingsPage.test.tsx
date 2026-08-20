import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/ui/Toast";

import { WorkspaceContextProvider } from "./WorkspaceContext";
import { WorkspaceSettingsPage } from "./WorkspaceSettingsPage";

afterEach(() => vi.restoreAllMocks());

describe("WorkspaceSettingsPage", () => {
  it("guarda el nombre, la descripción y el color por PATCH", async () => {
    document.cookie = "pact_csrf=test-token";
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ data: {
      id: "ws-1", name: "Footfall nuevo", slug: "footfall", description: "Descripción", color: "#2ab4e8",
    } }), { status: 200, headers: { "Content-Type": "application/json" } }));
    const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <ToastProvider>
          <WorkspaceContextProvider value={{
            workspaces: [], projects: [], workspaceProjects: [],
            workspace: { id: "ws-1", name: "Footfall", slug: "footfall", description: "Anterior", color: "#c9ee4d" },
            principal: { id: "jorge", organization_role: "owner" },
            stream: { status: "idle", events: [] },
            refreshDirectory: async () => undefined,
          }}>
            <WorkspaceSettingsPage />
          </WorkspaceContextProvider>
        </ToastProvider>
      </QueryClientProvider>,
    );
    fireEvent.change(screen.getByLabelText("Nombre"), { target: { value: "Footfall nuevo" } });
    fireEvent.change(screen.getByLabelText("Descripción"), { target: { value: "Descripción" } });
    fireEvent.click(screen.getByLabelText("Cian"));
    fireEvent.click(screen.getByRole("button", { name: "Guardar cambios" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/v1/workspaces/ws-1");
    expect(init?.method).toBe("PATCH");
    expect(JSON.parse(String(init?.body))).toEqual({ name: "Footfall nuevo", description: "Descripción", color: "#2ab4e8" });
    expect(new Headers(init?.headers).get("X-Pact-CSRF")).toBe("test-token");
  });
});
