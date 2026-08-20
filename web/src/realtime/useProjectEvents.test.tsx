import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SESSION_EXPIRED_EVENT } from "@/api/client";

import { useProjectEvents } from "./useProjectEvents";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useProjectEvents", () => {
  it("notifica la sesión caducada y no reintenta el stream tras un 401", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
    const expired = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    window.addEventListener(SESSION_EXPIRED_EVENT, expired);

    function Harness() {
      const stream = useProjectEvents("project-1");
      return <span>{stream.status}</span>;
    }

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <Harness />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("offline")).toBeInTheDocument();
    await waitFor(() => expect(expired).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledTimes(1);

    window.removeEventListener(SESSION_EXPIRED_EVENT, expired);
  });
});
