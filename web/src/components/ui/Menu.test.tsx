import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AccountMenu } from "./AccountMenu";
import { Menu, MenuContent, MenuItem, MenuTrigger } from "./Menu";

afterEach(cleanup);

describe("Menu", () => {
  it("abre, permite seleccionar y devuelve el estado al trigger", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();

    render(
      <Menu>
        <MenuTrigger>Acciones</MenuTrigger>
        <MenuContent side="bottom">
          <MenuItem onSelect={onSelect}>Configurar</MenuItem>
        </MenuContent>
      </Menu>,
    );

    const trigger = screen.getByRole("button", { name: "Acciones" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await user.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("menu")).toBeVisible();

    await user.click(screen.getByRole("menuitem", { name: "Configurar" }));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("se cierra con Escape y devuelve el foco", async () => {
    const user = userEvent.setup();
    render(
      <Menu>
        <MenuTrigger>Acciones</MenuTrigger>
        <MenuContent><MenuItem>Configurar</MenuItem></MenuContent>
      </Menu>,
    );

    const trigger = screen.getByRole("button", { name: "Acciones" });
    await user.click(trigger);
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });
});

describe("AccountMenu", () => {
  it("expone la identidad y ejecuta una opción del menú", async () => {
    const user = userEvent.setup();
    const onAccess = vi.fn();

    render(
      <AccountMenu
        name="Jorge Nuanz"
        email="jorge@nuanzs.com"
        items={[
          {
            id: "access",
            label: "Personas y acceso",
            description: "Usuarios, agentes y permisos",
            onSelect: onAccess,
          },
        ]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Abrir menú de Jorge Nuanz" }));
    expect(screen.getByText("jorge@nuanzs.com")).toBeVisible();

    await user.click(screen.getByRole("menuitem", { name: /Personas y acceso/ }));
    expect(onAccess).toHaveBeenCalledOnce();
  });
});
