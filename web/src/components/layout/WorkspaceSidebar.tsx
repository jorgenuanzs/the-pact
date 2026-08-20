import { forwardRef } from "react";
import { NavLink } from "react-router-dom";

import type { Workspace } from "@/api/types";
import { Icon, type IconName } from "@/components/ui/Icon";
import { initials, workspaceColorKey } from "@/lib/format";

const groups: ReadonlyArray<{ label: string; items: ReadonlyArray<{ path: string; label: string; icon: IconName; end?: boolean }> }> = [
  { label: "GENERAL", items: [{ path: "", label: "Resumen", icon: "home", end: true }] },
  { label: "OPERACIÓN", items: [
    { path: "live", label: "Trabajo en vivo", icon: "play" },
    { path: "activity", label: "Actividad", icon: "activity" },
  ] },
  { label: "COLABORACIÓN", items: [
    { path: "conversations", label: "Conversaciones", icon: "hash" },
    { path: "context", label: "Contexto", icon: "context" },
  ] },
  { label: "CÓDIGO", items: [{ path: "repositories", label: "Repositorios", icon: "repository" }] },
  { label: "GESTIÓN", items: [
    { path: "people", label: "Usuarios y agentes", icon: "people" },
    { path: "settings", label: "Configuración", icon: "settings" },
  ] },
];

interface WorkspaceSidebarProps {
  workspace: Workspace;
  repositoryCount?: number;
  mobileOpen?: boolean;
  onNavigate?: () => void;
}

export const WorkspaceSidebar = forwardRef<HTMLElement, WorkspaceSidebarProps>(function WorkspaceSidebar(
  { workspace, repositoryCount = 0, mobileOpen = false, onNavigate },
  ref,
) {
  const base = `/w/${encodeURIComponent(workspace.id)}`;
  return (
    <aside
      ref={ref}
      id="workspace-navigation"
      className="workspace-sidebar"
      data-mobile-open={mobileOpen || undefined}
      aria-label={`Navegación de ${workspace.name}`}
    >
      <header className="workspace-sidebar-header">
        <span className="workspace-sidebar-avatar" data-color={workspaceColorKey(workspace.color)} aria-hidden="true">
          {initials(workspace.name).slice(0, 1)}
        </span>
        <span>
          <strong>{workspace.name}</strong>
          <small>{workspace.description || `${repositoryCount} repositorios conectados`}</small>
        </span>
      </header>
      <label className="workspace-sidebar-search">
        <Icon name="search" size="sm" />
        <span className="pact-visually-hidden">Buscar en {workspace.name}</span>
        <input type="search" placeholder={`Buscar en ${workspace.name}`} disabled title="La búsqueda global llegará próximamente" />
      </label>
      <nav className="workspace-sidebar-navigation">
        {groups.map((group) => (
          <section key={group.label}>
            <p>{group.label}</p>
            {group.items.map((item) => (
              <NavLink
                key={item.path}
                to={item.path ? `${base}/${item.path}` : base}
                end={item.end}
                className={({ isActive }) => isActive ? "workspace-nav-item is-active" : "workspace-nav-item"}
                onClick={onNavigate}
              >
                <span className="workspace-nav-icon" aria-hidden="true"><Icon name={item.icon} /></span>
                <strong>{item.label}</strong>
              </NavLink>
            ))}
          </section>
        ))}
      </nav>
      <footer className="workspace-sidebar-footer">
        <span className="connection-dot" />
        <span>PACT SERVER</span>
      </footer>
    </aside>
  );
});
