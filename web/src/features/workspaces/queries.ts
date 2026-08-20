import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { idempotencyKey, requestData } from "@/api/client";
import { queryKeys } from "@/api/queryKeys";
import type { GitHubStatus, Principal, ProjectSummary, Workspace } from "@/api/types";
import { normalizeProjects, normalizeWorkspaces } from "@/api/viewModels";

export function useControlDirectory() {
  const workspaces = useQuery({
    queryKey: queryKeys.workspaces,
    queryFn: async () => normalizeWorkspaces(await requestData<unknown>("/v1/workspaces")),
  });
  const projects = useQuery({
    queryKey: queryKeys.projects,
    queryFn: async () => normalizeProjects(await requestData<unknown>("/v1/projects")),
  });
  const github = useQuery({
    queryKey: queryKeys.github,
    queryFn: () => requestData<GitHubStatus>("/v1/integrations/github"),
  });
  const principal = useQuery({
    queryKey: queryKeys.me,
    queryFn: () => requestData<Principal>("/v1/me"),
  });

  return {
    workspaces: workspaces.data || [],
    projects: projects.data || [],
    github: github.data,
    principal: principal.data,
    isPending: workspaces.isPending || projects.isPending || principal.isPending,
    error: workspaces.error || projects.error || principal.error,
    refetch: async () => Promise.all([
      workspaces.refetch(),
      projects.refetch(),
      github.refetch(),
      principal.refetch(),
    ]),
  };
}

export interface CreateWorkspaceInput {
  name: string;
  slug: string;
  description: string;
  color: string;
}

export function useCreateWorkspace() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateWorkspaceInput) => requestData<Workspace>("/v1/workspaces", {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey("workspace-create") },
      body: input,
    }),
    onSuccess: (workspace) => {
      queryClient.setQueryData<Workspace[]>(queryKeys.workspaces, (current = []) => [
        ...current.filter((item) => item.id !== workspace.id),
        workspace,
      ]);
    },
  });
}

export function useUpdateWorkspace(workspaceID: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Pick<Workspace, "name" | "description" | "color">) =>
      requestData<Workspace>(`/v1/workspaces/${encodeURIComponent(workspaceID)}`, {
        method: "PATCH",
        body: input,
      }),
    onSuccess: (workspace) => {
      queryClient.setQueryData<Workspace[]>(queryKeys.workspaces, (current = []) =>
        current.map((item) => item.id === workspace.id ? workspace : item));
    },
  });
}

export function workspaceProjects(workspace: Workspace | undefined, projects: ProjectSummary[]): ProjectSummary[] {
  if (!workspace) return [];
  const ids = new Set((workspace.projects || []).map((project) => project.id));
  const matched = projects.filter((project) => ids.has(project.id) || project.workspace_id === workspace.id);
  return matched.length ? matched : (workspace.projects || []);
}
