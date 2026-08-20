import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppErrorBoundary } from "./AppErrorBoundary";

function BrokenView(): never {
  throw new Error("render failed");
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("AppErrorBoundary", () => {
  it("muestra recuperación en vez de dejar la aplicación en blanco", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    render(<AppErrorBoundary><BrokenView /></AppErrorBoundary>);

    expect(screen.getByRole("heading", { name: "No se pudo mostrar esta sección" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Volver al inicio" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Recargar PACT" })).toBeInTheDocument();
  });
});
