import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useWorkspaceDirectoryEvents } from "./useWorkspaceDirectoryEvents";

afterEach(() => {
  cleanup();
  delete window.go;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("useWorkspaceDirectoryEvents", () => {
  it("actualiza el directorio cuando llega un evento del servidor", async () => {
    const encoder = new TextEncoder();
    const body = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode(
          ": pact workspace directory stream\n\n"
          + "event: pact.workspace.directory.updated.v1\n"
          + "data: {\"type\":\"pact.workspace.directory.updated.v1\"}\n\n",
        ));
        controller.close();
      },
    });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(body, {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    })));
    const refresh = vi.fn().mockResolvedValue(undefined);

    function Harness() {
      const status = useWorkspaceDirectoryEvents(true, refresh);
      return <span>{status}</span>;
    }

    render(<Harness />);

    expect(await screen.findByText("connected")).toBeInTheDocument();
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(2));
  });

  it("vuelve a consultar al recuperar el foco", async () => {
    const refresh = vi.fn().mockResolvedValue(undefined);
    const body = new ReadableStream({ start() {} });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(body, { status: 200 })));

    function Harness() {
      useWorkspaceDirectoryEvents(true, refresh);
      return null;
    }

    render(<Harness />);
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
    window.dispatchEvent(new Event("focus"));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(2));
  });
});
