import { useId } from "react";

export const WORKSPACE_COLORS = [
  { id: "lime", label: "Lima", value: "#c9ee4d" },
  { id: "cyan", label: "Cian", value: "#2ab4e8" },
  { id: "cobalt", label: "Cobalto", value: "#3c6ee0" },
  { id: "orange", label: "Naranja", value: "#ee8a3c" },
  { id: "terracotta", label: "Terracota", value: "#d2683d" },
  { id: "mauve", label: "Malva", value: "#9b5fc0" },
] as const;

export type WorkspaceColor = (typeof WORKSPACE_COLORS)[number]["value"];

export interface ColorPickerProps {
  value: string;
  onValueChange: (value: WorkspaceColor) => void;
  label?: string;
  description?: string;
  name?: string;
  disabled?: boolean;
}

export function ColorPicker({
  value,
  onValueChange,
  label = "Color del workspace",
  description = "Se utiliza para reconocer el workspace; no comunica estados.",
  name,
  disabled = false,
}: ColorPickerProps) {
  const generatedName = useId();
  const groupName = name ?? generatedName;

  return (
    <fieldset className="pact-color-picker" disabled={disabled}>
      <legend>{label}</legend>
      {description && <p className="pact-color-picker-description">{description}</p>}
      <div className="pact-color-picker-options">
        {WORKSPACE_COLORS.map((color) => (
          <label className="pact-color-option" key={color.id}>
            <input
              type="radio"
              name={groupName}
              value={color.value}
              checked={value.toLowerCase() === color.value}
              onChange={() => onValueChange(color.value)}
            />
            <span className="pact-color-swatch" data-color={color.id} aria-hidden="true" />
            <span>{color.label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}
