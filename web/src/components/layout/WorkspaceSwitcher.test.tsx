import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { WorkspaceSwitcher } from "./WorkspaceSwitcher";

const workspaces = [
  { id: "ws-1", name: "Footfall", slug: "footfall", color: "#2ab4e8" },
  { id: "ws-2", name: "Magi", slug: "magi", color: "#9b5fc0" },
];

afterEach(cleanup);

describe("WorkspaceSwitcher", () => {
  it("muestra el botón para crear workspaces a quien puede administrarlos", () => {
    render(<WorkspaceSwitcher workspaces={workspaces} canCreate selectedWorkspaceID="ws-1" onSelect={() => undefined} onCreate={() => undefined} />);
    expect(screen.getByRole("button", { name: "Crear workspace" })).toHaveAttribute("aria-haspopup", "dialog");
    expect(screen.getByRole("button", { name: "Abrir workspace Footfall" })).toHaveAttribute("aria-current", "page");
  });

  it("selecciona un workspace y oculta creación sin permiso", () => {
    const onSelect = vi.fn();
    render(<WorkspaceSwitcher workspaces={workspaces} canCreate={false} onSelect={onSelect} onCreate={() => undefined} />);
    expect(screen.queryByRole("button", { name: "Crear workspace" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Abrir workspace Magi" }));
    expect(onSelect).toHaveBeenCalledWith(workspaces[1]);
  });
});
