import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";

import { idempotencyKey, requestData } from "@/api/client";
import { queryKeys } from "@/api/queryKeys";
import type { AvailableRepository, GitHubStatus, Repository } from "@/api/types";

function listFrom<T>(payload: unknown, keys: string[]): T[] {
  if (Array.isArray(payload)) return payload as T[];
  const record = payload as Record<string, unknown> | null;
  for (const key of keys) {
    if (Array.isArray(record?.[key])) return record[key] as T[];
  }
  return [];
}

export function useRepositories(projectID?: string) {
  return useQuery({
    queryKey: queryKeys.repositories(projectID || "none"),
    queryFn: async () => listFrom<Repository>(
      await requestData<unknown>(`/v1/projects/${encodeURIComponent(projectID!)}/repositories`),
      ["repositories", "project_repositories"],
    ),
    enabled: Boolean(projectID),
  });
}

export function useWorkspaceRepositories(projectIDs: string[]) {
  const uniqueIDs = [...new Set(projectIDs.filter(Boolean))];
  const queries = useQueries({
    queries: uniqueIDs.map((projectID) => ({
      queryKey: queryKeys.repositories(projectID),
      queryFn: async () => listFrom<Repository>(
        await requestData<unknown>(`/v1/projects/${encodeURIComponent(projectID)}/repositories`),
        ["repositories", "project_repositories"],
      ),
    })),
  });
  const data = queries.flatMap((query, index) => (query.data || []).map((repository) => ({
    ...repository,
    project_id: (repository as Repository & { project_id?: string }).project_id || uniqueIDs[index],
  })));
  return {
    data,
    isPending: uniqueIDs.length > 0 && queries.some((query) => query.isPending),
    error: queries.find((query) => query.error)?.error,
    refetch: async () => Promise.all(queries.map((query) => query.refetch())),
  };
}

export function useAvailableRepositories(projectID?: string, enabled = true) {
  return useQuery({
    queryKey: queryKeys.availableRepositories(projectID || "none"),
    queryFn: async () => listFrom<AvailableRepository>(
      await requestData<unknown>(`/v1/integrations/github/repositories?project_id=${encodeURIComponent(projectID!)}`),
      ["repositories"],
    ),
    enabled: Boolean(projectID && enabled),
  });
}

export function useConnectGitHub() {
  return useMutation({
    mutationFn: () => requestData<{ install_url?: string } | GitHubStatus>("/v1/integrations/github/connect", { method: "POST", body: {} }),
    onSuccess: (payload) => {
      if (payload.install_url) window.location.assign(payload.install_url);
    },
  });
}

export function useAttachRepository(projectID: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { github_repository_id: number; purpose: string; required: boolean; primary: boolean }) =>
      requestData<Repository>(`/v1/projects/${encodeURIComponent(projectID)}/repositories`, {
        method: "POST",
        body: input,
      }),
    onSuccess: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.repositories(projectID) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.availableRepositories(projectID) }),
    ]),
  });
}

export function useSyncRepository() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ projectID, repositoryID }: { projectID: string; repositoryID: string }) => requestData<Repository>(
      `/v1/projects/${encodeURIComponent(projectID)}/repositories/${encodeURIComponent(repositoryID)}/sync`,
      { method: "POST", body: {} },
    ),
    onSuccess: (_repository, variables) => queryClient.invalidateQueries({ queryKey: queryKeys.repositories(variables.projectID) }),
  });
}

export function useSyncCanonicalRepository() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (projectID: string) => requestData(
      `/v1/projects/${encodeURIComponent(projectID)}/repository-sync`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey("repository-sync") },
        body: {},
      },
    ),
    onSuccess: (_result, projectID) => Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.overview(projectID) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.repositories(projectID) }),
    ]),
  });
}
