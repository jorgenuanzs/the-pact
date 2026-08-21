import { Navigate, Route, Routes } from "react-router-dom";

import { ControlShell } from "@/components/layout/ControlShell";
import { EmptyState } from "@/components/ui/States";
import { OrganizationAccessPage } from "@/features/administration/OrganizationAccessPage";
import { PeoplePage } from "@/features/access/AccessPages";
import { ActivityPage } from "@/features/activity/ActivityPage";
import { ConversationsPage } from "@/features/conversations/ConversationsPage";
import { ContextPage } from "@/features/context/ContextPage";
import { LiveWorkPage } from "@/features/live-work/LiveWorkPage";
import { WorkspaceOverviewPage } from "@/features/overview/WorkspaceOverviewPage";
import { RepositoriesPage, RepositoryDetailPage } from "@/features/repositories/RepositoriesPage";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";
import { WorkspaceSettingsPage } from "@/features/workspaces/WorkspaceSettingsPage";
import { LocalComputerPage } from "@/features/desktop/LocalComputerPage";

function HomeRoute() {
  const { workspaces } = useWorkspace();
  if (workspaces[0]) return <Navigate to={`/w/${encodeURIComponent(workspaces[0].id)}`} replace />;
  return <div className="pact-page pact-standalone-page"><EmptyState title="Crea el primer workspace" description="Usa el botón + del rail para reunir repositorios, contexto, conversaciones y colaboradores." /></div>;
}

function MissingRoute() {
  return <div className="pact-page pact-standalone-page"><EmptyState title="Esta página no existe" actionLabel="Volver al inicio" onAction={() => window.location.assign("/admin/")} /></div>;
}

export function App() {
  return (
    <Routes>
      <Route element={<ControlShell />}>
        <Route index element={<HomeRoute />} />
        <Route path="w/:workspaceId" element={<WorkspaceOverviewPage />} />
        <Route path="w/:workspaceId/live" element={<LiveWorkPage />} />
        <Route path="w/:workspaceId/activity" element={<ActivityPage />} />
        <Route path="w/:workspaceId/conversations" element={<ConversationsPage />} />
        <Route path="w/:workspaceId/context" element={<ContextPage />} />
        <Route path="w/:workspaceId/repositories" element={<RepositoriesPage />} />
        <Route path="w/:workspaceId/repositories/:repositoryId" element={<RepositoryDetailPage />} />
        <Route path="w/:workspaceId/people" element={<PeoplePage />} />
        <Route path="w/:workspaceId/access" element={<Navigate to="../people" replace relative="path" />} />
        <Route path="w/:workspaceId/settings" element={<WorkspaceSettingsPage />} />
        <Route path="organization/access" element={<OrganizationAccessPage />} />
        <Route path="local" element={<LocalComputerPage />} />
        <Route path="local/connections" element={<LocalComputerPage view="connections" />} />
        <Route path="local/agents" element={<LocalComputerPage view="agents" />} />
        <Route path="local/folders" element={<LocalComputerPage view="folders" />} />
        <Route path="local/service" element={<LocalComputerPage view="service" />} />
        <Route path="*" element={<MissingRoute />} />
      </Route>
    </Routes>
  );
}
