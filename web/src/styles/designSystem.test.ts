import { describe, expect, it } from "vitest";

import fonts from "./index.css?raw";
import layout from "./layout.css?raw";
import tokens from "./tokens.css?raw";

describe("contrato visual del handoff", () => {
  it("mantiene la escala tipográfica canónica", () => {
    expect(tokens).toContain("--pact-text-view: 1.3125rem");
    expect(tokens).toContain("--pact-text-panel: 0.9375rem");
    expect(tokens).toContain("--pact-text-section: 0.875rem");
    expect(tokens).toContain("--pact-text-body: 0.8125rem");
    expect(tokens).toContain("--pact-text-table: 0.78125rem");
    expect(tokens).toContain("--pact-text-help: 0.71875rem");
    expect(tokens).toContain("--pact-text-label: 0.625rem");
  });

  it("carga únicamente las familias y pesos aprobados", () => {
    expect(fonts).toContain("Instrument+Sans:wght@400;500;600");
    expect(fonts).toContain("JetBrains+Mono:wght@400;700");
    expect(fonts).not.toContain("Instrument+Sans:wght@400;500;600;700");
    expect(fonts).not.toContain("JetBrains+Mono:wght@400;500;700");
  });

  it("mantiene controles e iconos en la métrica del sistema", () => {
    expect(tokens).toContain("--pact-control-height-sm: 1.625rem");
    expect(tokens).toContain("--pact-control-height-md: 2.125rem");
    expect(tokens).toContain("--pact-control-height-lg: 2.5rem");
    expect(tokens).toContain("--pact-icon-size-sm: 0.875rem");
    expect(tokens).toContain("--pact-icon-size-md: 1rem");
    expect(tokens).toContain("--pact-icon-size-lg: 1.125rem");
  });

  it("mantiene el encabezado fuera del área desplazable", () => {
    expect(layout).toMatch(/\.control-shell\s*\{[^}]*height:\s*100dvh;[^}]*overflow:\s*hidden;/s);
    expect(layout).toMatch(/\.pact-page\s*\{[^}]*grid-template-rows:\s*auto minmax\(0, 1fr\);[^}]*overflow:\s*hidden;/s);
    expect(layout).toMatch(/\.pact-page-content\s*\{[^}]*overflow:\s*auto;/s);
  });
});
