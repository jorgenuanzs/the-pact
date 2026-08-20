import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";

import { OverviewActiveWork } from "./OverviewActiveWork";

afterEach(cleanup);

describe("OverviewActiveWork", () => {
  it("condensa actor, objetivo, scope y estado en una sola fila", () => {
    render(
      <MemoryRouter>
        <OverviewActiveWork items={[{
          id: "work-1",
          actor_name: "Codex",
          actor_kind: "agent",
          intent: { id: "intent-1", title: "Ajustar navegación", status: "blocked" },
          scopes: [{ resource: { locator: "web/src/features/overview" } }],
          last_seen_at: "2026-08-17T18:00:00Z",
        }]} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Codex")).toBeInTheDocument();
    expect(screen.getByText("Ajustar navegación")).toBeInTheDocument();
    expect(screen.getByText("web/src/features/overview")).toBeInTheDocument();
    expect(screen.getByText("Bloqueado")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Ver trabajo en vivo/ })).toHaveAttribute("href", "/live");
  });
});
