import { useEffect, useMemo, useRef, useState } from "react";
import { matchPath, Outlet, useLocation, useNavigate } from "react-router-dom";

import { AccountMenu } from "@/components/ui/AccountMenu";
import { Icon } from "@/components/ui/Icon";
import { ErrorState, LoadingState } from "@/components/ui/States";
import { useAuth } from "@/features/auth";
import { MentionInbox } from "@/features/conversations/MentionInbox";
import { projectForWorkspace } from "@/api/viewModels";
import { useWorkspaceAccess } from "@/features/overview/queries";
import { canManage, workspaceColorKey } from "@/lib/format";
import { isDesktopRuntime } from "@/platform/desktop";
import { useWorkspaceEvents } from "@/realtime/useProjectEvents";
import { useControlDirectory, workspaceProjects } from "@/features/workspaces/queries";
import { WorkspaceContextProvider } from "@/features/workspaces/WorkspaceContext";

import { Brand } from "./Brand";
import { CreateWorkspaceDialog } from "./CreateWorkspaceDialog";
import { LocalSidebar } from "./LocalSidebar";
import { WorkspaceSidebar } from "./WorkspaceSidebar";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";

export function ControlShell() {
  const directory = useControlDirectory();
  const auth = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const [createOpen, setCreateOpen] = useState(false);
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
  const mobileNavigationTriggerRef = useRef<HTMLButtonElement>(null);
  const workspaceSidebarRef = useRef<HTMLElement>(null);
  const match = matchPath({ path: "/w/:workspaceId/*" }, location.pathname)
    || matchPath({ path: "/w/:workspaceId" }, location.pathname);
  const workspaceID = match?.params.workspaceId;
  const workspace = directory.workspaces.find((item) => item.id === workspaceID);
  const selectedWorkspaceProjects = workspaceProjects(workspace, directory.projects);
  const project = projectForWorkspace(workspace, directory.projects);
  const stream = useWorkspaceEvents(selectedWorkspaceProjects.map((item) => item.id));
  const access = useWorkspaceAccess(selectedWorkspaceProjects.map((item) => item.id));
  const principal = directory.principal || auth.principal || undefined;
  const organizationMode = location.pathname.startsWith("/organization/");
  const localMode = location.pathname === "/local" || location.pathname.startsWith("/local/");

  useEffect(() => {
    const result = new URL(window.location.href).searchParams.get("github");
    if (!result) return;
    const url = new URL(window.location.href);
    url.searchParams.delete("github");
    url.searchParams.delete("reason");
    window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
  }, []);

  useEffect(() => {
    setMobileNavigationOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!mobileNavigationOpen) return;
    document.body.classList.add("pact-scroll-lock");
    workspaceSidebarRef.current?.querySelector<HTMLAnchorElement>(".workspace-nav-item")?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setMobileNavigationOpen(false);
      mobileNavigationTriggerRef.current?.focus();
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.body.classList.remove("pact-scroll-lock");
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [mobileNavigationOpen]);

  const accountItems = useMemo(() => [
    ...(canManage(principal?.organization_role) ? [{
      id: "access",
      label: "Acceso y seguridad",
      description: "Personas, permisos e invitaciones",
      icon: <Icon name="access" />,
      onSelect: (): void => { navigate("/organization/access"); },
    }] : []),
    {
      id: "logout",
      label: "Cerrar sesión",
      description: "Salir de PACT Control",
      icon: <Icon name="logout" />,
      tone: "danger" as const,
      onSelect: (): void => { void auth.logout(); },
    },
  ], [auth, navigate, principal?.organization_role]);

  if (directory.isPending) return <LoadingState label="Preparando PACT Control" />;
  if (directory.error) return <ErrorState title="No se pudo cargar PACT Control" description={(directory.error as Error).message} actionLabel="Reintentar" onAction={() => void directory.refetch()} />;

  return (
    <WorkspaceContextProvider value={{
      workspaces: directory.workspaces,
      projects: directory.projects,
      workspaceProjects: selectedWorkspaceProjects,
      workspace,
      project,
      github: directory.github,
      principal,
      access: access.data,
      stream,
      refreshDirectory: directory.refetch,
    }}>
      <div
        className="control-shell"
        data-color={localMode ? "cyan" : workspaceColorKey(workspace?.color)}
        data-organization={organizationMode || undefined}
        data-local={localMode || undefined}
        data-workspace={workspace ? "true" : undefined}
      >
        <aside className="global-rail" aria-label="Navegación global">
          <button className="global-brand-button" type="button" aria-label="Ir al inicio" onClick={() => navigate("/")}><Brand /></button>
          {(workspace && !organizationMode) || localMode ? (
            <button
              ref={mobileNavigationTriggerRef}
              className="workspace-sidebar-toggle"
              type="button"
              aria-label={localMode
                ? (mobileNavigationOpen ? "Cerrar navegación de este computador" : "Abrir navegación de este computador")
                : (mobileNavigationOpen ? "Cerrar navegación del workspace" : "Abrir navegación del workspace")}
              aria-controls="workspace-navigation"
              aria-expanded={mobileNavigationOpen}
              onClick={() => setMobileNavigationOpen((open) => !open)}
            >
              <Icon name="menu" size="lg" />
            </button>
          ) : null}
          <WorkspaceSwitcher
            workspaces={directory.workspaces}
            selectedWorkspaceID={workspace?.id}
            canCreate={canManage(principal?.organization_role)}
            onSelect={(item) => navigate(`/w/${encodeURIComponent(item.id)}`)}
            onCreate={() => {
              setMobileNavigationOpen(false);
              setCreateOpen(true);
            }}
          />
          <div className="global-rail-actions">
            {isDesktopRuntime() ? (
              <button
                type="button"
                className="global-local-button"
                data-selected={localMode || undefined}
                aria-label="Administrar este computador"
                title="Este computador"
                onClick={() => navigate("/local")}
              >
                <Icon name="computer" size="lg" />
              </button>
            ) : null}
            <MentionInbox variant="rail" />
            <AccountMenu
              name={principal?.display_name || principal?.username || "Cuenta"}
              email={principal?.email || principal?.username || "Sesión activa"}
              avatarColor={organizationMode || localMode ? "cyan" : workspaceColorKey(workspace?.color) as "lime"}
              items={accountItems}
            />
          </div>
        </aside>
        {localMode ? (
          <LocalSidebar
            ref={workspaceSidebarRef}
            mobileOpen={mobileNavigationOpen}
            onNavigate={() => {
              if (!mobileNavigationOpen) return;
              setMobileNavigationOpen(false);
              requestAnimationFrame(() => document.getElementById("main-content")?.focus());
            }}
          />
        ) : !organizationMode && workspace ? (
          <WorkspaceSidebar
            ref={workspaceSidebarRef}
            workspace={workspace}
            mobileOpen={mobileNavigationOpen}
            onNavigate={() => {
              if (!mobileNavigationOpen) return;
              setMobileNavigationOpen(false);
              requestAnimationFrame(() => document.getElementById("main-content")?.focus());
            }}
          />
        ) : null}
        {mobileNavigationOpen ? (
          <button
            className="workspace-sidebar-backdrop"
            type="button"
            aria-label="Cerrar navegación del workspace"
            onClick={() => {
              setMobileNavigationOpen(false);
              mobileNavigationTriggerRef.current?.focus();
            }}
          />
        ) : null}
        <main className="control-main" id="main-content" tabIndex={-1}>
          <Outlet />
        </main>
        <CreateWorkspaceDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          onCreated={(id) => navigate(`/w/${encodeURIComponent(id)}/settings`)}
        />
      </div>
    </WorkspaceContextProvider>
  );
}
