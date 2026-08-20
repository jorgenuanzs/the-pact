import { createContext, useContext, type ReactNode } from "react";

import type { GitHubStatus, PactEvent, Principal, ProjectAccess, ProjectSummary, Workspace } from "@/api/types";
import type { StreamStatus } from "@/realtime/useProjectEvents";

interface WorkspaceContextValue {
  workspaces: Workspace[];
  projects: ProjectSummary[];
  workspace?: Workspace;
  workspaceProjects: ProjectSummary[];
  /** Primary internal partition used only for commands that still require a project ID. */
  project?: ProjectSummary;
  github?: GitHubStatus;
  principal?: Principal;
  access?: ProjectAccess;
  stream: { status: StreamStatus; events: PactEvent[] };
  refreshDirectory: () => Promise<unknown>;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceContextProvider({ value, children }: { value: WorkspaceContextValue; children: ReactNode }) {
  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}

export function useWorkspace() {
  const context = useContext(WorkspaceContext);
  if (!context) throw new Error("useWorkspace debe utilizarse dentro de WorkspaceContextProvider.");
  return context;
}
