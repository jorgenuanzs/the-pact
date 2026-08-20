import type { ReactNode } from "react";

import { Avatar, type AvatarColor } from "./Avatar";
import { Menu, MenuContent, MenuItem, MenuLabel, MenuSeparator, MenuTrigger } from "./Menu";

export interface AccountMenuItem {
  id: string;
  label: string;
  description?: string;
  icon?: ReactNode;
  count?: ReactNode;
  tone?: "default" | "danger";
  disabled?: boolean;
  onSelect: () => void;
}

export interface AccountMenuProps {
  name: string;
  email: string;
  avatarColor?: AvatarColor;
  label?: string;
  items: AccountMenuItem[];
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function AccountMenu({
  name,
  email,
  avatarColor = "lime",
  label = "Gestión global",
  items,
  open,
  defaultOpen,
  onOpenChange,
}: AccountMenuProps) {
  return (
    <Menu open={open} defaultOpen={defaultOpen} onOpenChange={onOpenChange}>
      <MenuTrigger className="pact-account-menu-trigger" aria-label={`Abrir menú de ${name}`}>
        <Avatar name={name} color={avatarColor} />
      </MenuTrigger>
      <MenuContent className="pact-account-menu-content" side="top" align="start" aria-label="Menú de cuenta">
        <div className="pact-account-menu-identity">
          <Avatar name={name} color={avatarColor} size="lg" />
          <span className="pact-account-menu-copy">
            <strong>{name}</strong>
            <small>{email}</small>
          </span>
        </div>
        <MenuLabel>{label}</MenuLabel>
        {items.map((item, index) => (
          <div key={item.id} role="none">
            {item.tone === "danger" && index > 0 ? <MenuSeparator /> : null}
            <MenuItem tone={item.tone} disabled={item.disabled} onSelect={item.onSelect}>
              {item.icon && <span aria-hidden="true">{item.icon}</span>}
              <span className="pact-account-menu-item-copy">
                <strong>{item.label}</strong>
                {item.description && <small>{item.description}</small>}
              </span>
              {item.count}
            </MenuItem>
          </div>
        ))}
      </MenuContent>
    </Menu>
  );
}
