import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { PactEvent } from "@/api/types";

import { ActivityTimeline } from "./ActivityTimeline";

afterEach(cleanup);

const events: PactEvent[] = [
  { id: "event-one", type: "pact.intent.active.v1", actor_id: "Codex", data: { objective: "Primero" } },
  { id: "event-two", type: "pact.intent.blocked.v1", actor_id: "Claude", data: { objective: "Segundo" } },
];

describe("ActivityTimeline feed", () => {
  it("expande los datos inline y mantiene una sola fila abierta", () => {
    render(<ActivityTimeline events={events} variant="feed" />);

    const buttons = screen.getAllByRole("button", { name: "Ver datos" });
    fireEvent.click(buttons[0]);
    expect(screen.getByText(/Primero/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ocultar datos" })).toHaveAttribute("aria-expanded", "true");

    fireEvent.click(screen.getByRole("button", { name: "Ver datos" }));
    expect(screen.queryByText(/Primero/)).not.toBeInTheDocument();
    expect(screen.getByText(/Segundo/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Ocultar datos" }));
    expect(screen.queryByText(/Segundo/)).not.toBeInTheDocument();
  });

  it("cierra la fila expandida al hacer clic fuera o pulsar Escape", () => {
    render(<ActivityTimeline events={events} variant="feed" />);

    fireEvent.click(screen.getAllByRole("button", { name: "Ver datos" })[0]);
    expect(screen.getByText(/Primero/)).toBeInTheDocument();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByText(/Primero/)).not.toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Ver datos" })[0]);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByText(/Primero/)).not.toBeInTheDocument();
  });
});
