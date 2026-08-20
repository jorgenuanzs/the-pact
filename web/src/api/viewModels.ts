import type { ProjectSummary, Workspace } from "./types";

export function projectForWorkspace(workspace: Workspace | undefined, projects: ProjectSummary[]): ProjectSummary | undefined {
  if (!workspace) return undefined;
  const projectIDs = new Set((workspace.projects || []).map((project) => project.id));
  return projects.find((project) => projectIDs.has(project.id) && project.status !== "archived")
    || projects.find((project) => project.workspace_id === workspace.id && project.status !== "archived")
    || (workspace.projects || []).find((project) => project.status !== "archived")
    || workspace.projects?.[0];
}

export function normalizeWorkspaces(payload: unknown): Workspace[] {
  if (Array.isArray(payload)) return payload as Workspace[];
  const value = payload as { workspaces?: Workspace[] } | null;
  return value?.workspaces || [];
}

export function normalizeProjects(payload: unknown): ProjectSummary[] {
  if (Array.isArray(payload)) return payload as ProjectSummary[];
  const value = payload as { projects?: ProjectSummary[] } | null;
  return value?.projects || [];
}
