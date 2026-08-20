import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { HandoffList } from "./HandoffList";

afterEach(cleanup);

describe("HandoffList", () => {
  it("muestra el estado vacío cuando no existen relevos", () => {
    render(<HandoffList handoffs={[]} />);

    expect(screen.getByRole("heading", { name: "No hay handoffs pendientes" })).toBeInTheDocument();
    expect(screen.getByText(/bloqueos y próximos pasos/)).toBeInTheDocument();
  });

  it("renderiza identidad, estado, resumen, bloqueos y próximos pasos", () => {
    const { container } = render(
      <HandoffList
        handoffs={[{
          id: "handoff-1",
          from_actor_name: "Codex",
          to_actor_name: "María",
          status: "offered",
          summary: "La migración está preparada para revisión.",
          blockers: ["Falta validar PostgreSQL"],
          next_steps: ["Ejecutar la integración", "Revisar el plan"],
          offered_at: "2026-08-17T10:00:00Z",
        }]}
      />,
    );

    const list = container.querySelector<HTMLOListElement>(".handoff-list");
    expect(list).not.toBeNull();
    if (!list) return;
    expect(within(list).getByText("Codex → María")).toBeInTheDocument();
    expect(within(list).getByText("Ofrecido")).toBeInTheDocument();
    expect(within(list).getByText("La migración está preparada para revisión.")).toBeInTheDocument();
    expect(within(list).getByText("Falta validar PostgreSQL")).toBeInTheDocument();
    expect(within(list).getByText("Ejecutar la integración")).toBeInTheDocument();
    expect(within(list).getByText("Revisar el plan")).toBeInTheDocument();
  });
});
