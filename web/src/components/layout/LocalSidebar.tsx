import { forwardRef } from "react";
import { NavLink } from "react-router-dom";

import { Icon, type IconName } from "@/components/ui/Icon";

const entries: ReadonlyArray<{ path: string; label: string; icon: IconName; end?: boolean }> = [
  { path: "/local", label: "Resumen local", icon: "home", end: true },
  { path: "/local/connections", label: "Conexiones PACT", icon: "server" },
  { path: "/local/folders", label: "Carpetas", icon: "folder" },
  { path: "/local/agents", label: "Clientes de IA", icon: "people" },
  { path: "/local/service", label: "Servidor local", icon: "repository" },
];

interface LocalSidebarProps {
  mobileOpen?: boolean;
  onNavigate?: () => void;
}

export const LocalSidebar = forwardRef<HTMLElement, LocalSidebarProps>(function LocalSidebar(
  { mobileOpen = false, onNavigate },
  ref,
) {
  return (
    <aside
      ref={ref}
      id="workspace-navigation"
      className="workspace-sidebar local-sidebar"
      data-mobile-open={mobileOpen || undefined}
      aria-label="Navegación de este computador"
    >
      <header className="workspace-sidebar-header">
        <span className="workspace-sidebar-avatar local-sidebar-avatar" aria-hidden="true"><Icon name="computer" /></span>
        <span>
          <strong>Este computador</strong>
          <small>Configuración privada de este equipo</small>
        </span>
      </header>
      <nav className="workspace-sidebar-navigation local-sidebar-navigation">
        <section>
          <p>MI COMPUTADOR</p>
          {entries.map((entry) => (
            <NavLink
              key={entry.path}
              to={entry.path}
              end={entry.end}
              className={({ isActive }) => isActive ? "workspace-nav-item is-active" : "workspace-nav-item"}
              onClick={onNavigate}
            >
              <span className="workspace-nav-icon" aria-hidden="true"><Icon name={entry.icon} /></span>
              <strong>{entry.label}</strong>
            </NavLink>
          ))}
        </section>
      </nav>
      <footer className="workspace-sidebar-footer">
        <span className="connection-dot" />
        <span>CONFIGURACIÓN LOCAL</span>
      </footer>
    </aside>
  );
});
