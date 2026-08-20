import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";

import { WorkspaceSectionIndex } from "./WorkspaceSectionIndex";

afterEach(cleanup);

describe("WorkspaceSectionIndex", () => {
  it("resume cada sección como una navegación directa", () => {
    render(
      <MemoryRouter>
        <WorkspaceSectionIndex entries={[
          { to: "live", icon: "play", label: "Trabajo en vivo", description: "1 intent bloqueado", value: "3 activos", tone: "warning" },
          { to: "repositories", icon: "repository", label: "Repositorios", description: "Todos sincronizados", value: "2 repos", tone: "active" },
        ]} />
      </MemoryRouter>,
    );

    expect(screen.getByRole("navigation", { name: "Secciones del workspace" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Trabajo en vivo/ })).toHaveAttribute("href", "/live");
    expect(screen.getByRole("link", { name: /Repositorios/ })).toHaveAttribute("href", "/repositories");
  });
});
