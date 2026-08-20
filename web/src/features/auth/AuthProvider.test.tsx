import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { requestData } from "@/api/client";
import { AuthProvider } from "./AuthProvider";

function response(body: unknown, status = 200): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
  });
}

function renderAuth(children: React.ReactNode, queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
})) {
  return render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider>{children}</AuthProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "/admin/");
});

describe("AuthProvider", () => {
  it("inicia sesión y solo entonces muestra el contenido privado", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ title: "Unauthorized" }, 401))
      .mockResolvedValueOnce(response({ data: { required: false, configured: true } }))
      .mockResolvedValueOnce(response({ data: { id: "session-1" } }))
      .mockResolvedValueOnce(response({
        data: {
          id: "principal-1",
          display_name: "Jorge",
          organization_role: "owner",
        },
      }));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderAuth(<div>Contenido privado</div>);

    expect(await screen.findByRole("heading", { name: "Entrar" })).toBeInTheDocument();
    expect(screen.queryByText("Contenido privado")).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("Usuario o correo"), "jorge@nuanzs.com");
    await user.type(screen.getByLabelText("Contraseña"), "una-clave-correcta");
    await user.click(screen.getByRole("button", { name: "Iniciar sesión" }));

    expect(await screen.findByText("Contenido privado")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/v1/auth/login",
      expect.objectContaining({ method: "POST" }),
    );
    const loginRequest = fetchMock.mock.calls[2]?.[1] as RequestInit;
    expect(JSON.parse(String(loginRequest.body))).toEqual({
      login: "jorge@nuanzs.com",
      password: "una-clave-correcta",
    });
  });

  it("no muestra contenido privado mientras comprueba la sesión", async () => {
    let resolveSession: ((value: Response) => void) | undefined;
    const sessionRequest = new Promise<Response>((resolve) => {
      resolveSession = resolve;
    });
    const fetchMock = vi.fn()
      .mockReturnValueOnce(sessionRequest)
      .mockResolvedValueOnce(response({
        data: {
          id: "principal-1",
          display_name: "Jorge",
          organization_role: "owner",
        },
      }));
    vi.stubGlobal("fetch", fetchMock);

    renderAuth(<div data-testid="private-control-plane">Contenido privado</div>);

    expect(screen.getByLabelText("Comprobando sesión")).toBeInTheDocument();
    expect(screen.queryByTestId("private-control-plane")).not.toBeInTheDocument();

    resolveSession?.(response({ data: { kind: "web" } }));
    expect(await screen.findByTestId("private-control-plane")).toBeInTheDocument();
  });

  it("permite aceptar una invitación con la sesión ya iniciada", async () => {
    window.history.replaceState(null, "", "/admin/#invite=pact_inv_existing");
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ data: { kind: "web" } }))
      .mockResolvedValueOnce(response({
        data: {
          id: "principal-1",
          display_name: "Jorge",
          organization_role: "member",
        },
      }))
      .mockResolvedValueOnce(response({ data: { accepted: true } }));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderAuth(<div>Contenido privado</div>);

    expect(await screen.findByRole("heading", { name: "Añadir acceso a tu cuenta" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Aceptar invitación" }));

    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/v1/auth/invitations/accept",
      expect.objectContaining({ method: "POST" }),
    );
    const invitationRequest = fetchMock.mock.calls[2]?.[1] as RequestInit;
    expect(JSON.parse(String(invitationRequest.body))).toEqual({ secret: "pact_inv_existing" });
    expect(window.location.hash).toBe("");
    expect(screen.queryByRole("heading", { name: "Añadir acceso a tu cuenta" })).not.toBeInTheDocument();
  });

  it("vuelve al acceso y limpia la caché cuando una petición recibe 401", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response({ data: { kind: "web" } }))
      .mockResolvedValueOnce(response({
        data: {
          id: "principal-1",
          display_name: "Jorge",
          organization_role: "owner",
        },
      }))
      .mockResolvedValueOnce(response({ title: "Unauthorized" }, 401))
      .mockResolvedValueOnce(response({ data: { required: false, configured: true } }));
    vi.stubGlobal("fetch", fetchMock);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(["private-data"], { secret: true });
    const user = userEvent.setup();

    function ExpiringRequest() {
      return (
        <button type="button" onClick={() => void requestData("/v1/protected").catch(() => undefined)}>
          Solicitar datos privados
        </button>
      );
    }

    renderAuth(<ExpiringRequest />, queryClient);

    await user.click(await screen.findByRole("button", { name: "Solicitar datos privados" }));

    expect(await screen.findByRole("heading", { name: "Entrar" })).toBeInTheDocument();
    expect(screen.getByText("La sesión venció. Inicia sesión para continuar.")).toBeInTheDocument();
    expect(queryClient.getQueryData(["private-data"])).toBeUndefined();
    expect(fetchMock).toHaveBeenNthCalledWith(4, "/v1/auth/setup", expect.any(Object));
  });
});
