import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { PactEvent } from "@/api/types";

import { ActivityTable } from "./ActivityTable";

afterEach(cleanup);

function activities(count: number): PactEvent[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `event-${index + 1}`,
    sequence: String(index + 1),
    type: index === 20 ? "pact.intent.blocked.v1" : "pact.intent.active.v1",
    actor_name: index === 20 ? "Claude" : "Codex",
    occurred_at: new Date(Date.UTC(2026, 7, 17, 10, index)).toISOString(),
    data: { repository: index === 20 ? "footfall-api" : "footfall-web" },
  }));
}

describe("ActivityTable", () => {
  it("pagina el historial y mantiene disponible la búsqueda", () => {
    render(<ActivityTable events={activities(21)} />);

    expect(screen.getByRole("searchbox")).toBeInTheDocument();
    expect(screen.getByText("1–20 de 21")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "← Anterior" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Siguiente →" }));
    expect(screen.getByText("21–21 de 21")).toBeInTheDocument();
  });

  it("busca por actor, evento y datos técnicos", () => {
    render(<ActivityTable events={activities(21)} />);

    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "Claude footfall-api" } });

    expect(screen.getByText("Intent bloqueado")).toBeInTheDocument();
    expect(screen.getByText("Claude")).toBeInTheDocument();
    expect(screen.getByText("1 actividad")).toBeInTheDocument();
    expect(screen.queryByText("Intent activo")).not.toBeInTheDocument();
  });
});
