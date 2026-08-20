import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Icon } from "./Icon";

describe("Icon", () => {
  it("usa una geometría SVG estable y el tamaño solicitado", () => {
    render(<Icon name="settings" size="lg" data-testid="icon" />);

    const icon = screen.getByTestId("icon");
    expect(icon).toHaveAttribute("viewBox", "0 0 24 24");
    expect(icon).toHaveAttribute("data-size", "lg");
    expect(icon).toHaveClass("pact-icon");
    expect(icon).toHaveAttribute("aria-hidden", "true");
  });
});
