import { useState } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ColorPicker, type WorkspaceColor } from "./ColorPicker";

afterEach(cleanup);

function ControlledPicker({ onChange }: { onChange: (value: WorkspaceColor) => void }) {
  const [value, setValue] = useState<WorkspaceColor>("#c9ee4d");
  return (
    <ColorPicker
      value={value}
      onValueChange={(nextValue) => {
        setValue(nextValue);
        onChange(nextValue);
      }}
    />
  );
}

describe("ColorPicker", () => {
  it("presenta los colores como radios y actualiza el valor seleccionado", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<ControlledPicker onChange={onChange} />);

    expect(screen.getByRole("radio", { name: "Lima" })).toBeChecked();
    await user.click(screen.getByRole("radio", { name: "Cobalto" }));

    expect(onChange).toHaveBeenCalledWith("#3c6ee0");
    expect(screen.getByRole("radio", { name: "Cobalto" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Lima" })).not.toBeChecked();
  });

  it("no utiliza estilos inline para representar el color", () => {
    render(<ColorPicker value="#c9ee4d" onValueChange={() => undefined} />);
    for (const swatch of document.querySelectorAll(".pact-color-swatch")) {
      expect(swatch).not.toHaveAttribute("style");
      expect(swatch).toHaveAttribute("data-color");
    }
  });
});
