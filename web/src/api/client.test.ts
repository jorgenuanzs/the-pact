import { afterEach, describe, expect, it, vi } from "vitest";

import { APIError, request, SESSION_EXPIRED_EVENT } from "./client";

afterEach(() => {
  delete window.go;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("API client", () => {
  it("muestra el detalle de respuestas problem+json", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: "forbidden",
      title: "Forbidden",
      detail: "No tienes acceso a este recurso.",
    }), { status: 403, headers: { "Content-Type": "application/problem+json" } })));

    await expect(request("/v1/protected")).rejects.toMatchObject({
      status: 403,
      code: "forbidden",
      message: "No tienes acceso a este recurso.",
    });
  });

  it("notifica globalmente cuando vence una sesión", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      code: "unauthorized",
      detail: "Session expired.",
    }), { status: 401, headers: { "Content-Type": "application/problem+json" } })));
    const listener = vi.fn();
    window.addEventListener(SESSION_EXPIRED_EVENT, listener);

    await expect(request("/v1/protected")).rejects.toBeInstanceOf(APIError);
    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener(SESSION_EXPIRED_EVENT, listener);
  });

  it("usa el puente nativo sin exponer una credencial en React", async () => {
    const APIRequest = vi.fn().mockResolvedValue({
      status: 200,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ data: { id: "workspace-1" } }),
    });
    window.go = { main: { Desktop: {
      Status: vi.fn(),
      BeginDeviceLogin: vi.fn(),
      PollDeviceLogin: vi.fn(),
      OpenExternalURL: vi.fn(),
      Disconnect: vi.fn(),
      LocalComputerStatus: vi.fn(),
      SelectLocalProjectFolder: vi.fn(),
      InspectLocalProjectFolder: vi.fn(),
      ConnectLocalAgent: vi.fn(),
      LocalServerStatus: vi.fn(),
      InstallLocalServer: vi.fn(),
      StartLocalServer: vi.fn(),
      StopLocalServer: vi.fn(),
      BackupLocalServer: vi.fn(),
      UpgradeLocalServer: vi.fn(),
      APIRequest,
      StartProjectEventStream: vi.fn(),
      StopProjectEventStream: vi.fn(),
    } } };
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(request("/v1/workspaces", {
      method: "POST",
      body: { name: "Footfall" },
    })).resolves.toEqual({ data: { id: "workspace-1" } });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(APIRequest).toHaveBeenCalledWith(expect.objectContaining({
      method: "POST",
      path: "/v1/workspaces",
      body: JSON.stringify({ name: "Footfall" }),
    }));
    expect(JSON.stringify(APIRequest.mock.calls[0])).not.toContain("credential");
  });
});
