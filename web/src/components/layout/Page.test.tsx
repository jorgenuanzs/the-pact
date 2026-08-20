import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { Page } from "./Page";

afterEach(cleanup);

describe("Page", () => {
  it("mantiene el encabezado fuera del contenedor que aplica padding al contenido", () => {
    const { container } = render(
      <Page title="Repositorios" description="Código del workspace" showWorkspaceStatus={false}>
        <div>Contenido de repositorios</div>
      </Page>,
    );

    const page = container.querySelector(".pact-page");
    const header = screen.getByRole("banner");
    const content = screen.getByText("Contenido de repositorios").parentElement;

    expect(page?.children).toHaveLength(2);
    expect(page?.children[0]).toBe(header);
    expect(page?.children[1]).toBe(content);
    expect(content).toHaveClass("pact-page-content");
    expect(screen.queryByText("Código del workspace")).not.toBeInTheDocument();
  });
});
