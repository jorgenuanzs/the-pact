import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";

import { LiveWorkTabs } from "./LiveWorkTabs";

afterEach(cleanup);

describe("LiveWorkTabs", () => {
  it("separa intents, código observado y handoffs en vistas navegables", () => {
    render(
      <MemoryRouter>
        <LiveWorkTabs intents={[]} codeActivity={{ state: "idle" }} handoffs={[]} />
      </MemoryRouter>,
    );

    expect(screen.getByRole("tab", { name: /Intents y scopes/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("heading", { name: "Intents y scopes" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Código en vivo" }));
    expect(screen.getByRole("heading", { name: "Estado observado del código" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: /Handoffs/ }));
    expect(screen.getByRole("heading", { name: "Handoffs estructurados" })).toBeInTheDocument();
    expect(screen.getByText("No hay handoffs pendientes")).toBeInTheDocument();
  });
});
