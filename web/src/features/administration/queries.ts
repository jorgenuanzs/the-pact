import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { requestData } from "@/api/client";
import { queryKeys } from "@/api/queryKeys";
import type { AdminUser, Invitation, UserDirectory } from "@/api/types";

function normalizeDirectory(payload: unknown): UserDirectory {
  const value = payload as Partial<UserDirectory> | null;
  return {
    users: value?.users || [],
    invitations: value?.invitations || [],
    events: value?.events || [],
  };
}

export function useUserDirectory(enabled = true) {
  return useQuery({
    queryKey: queryKeys.users,
    queryFn: async () => normalizeDirectory(await requestData<unknown>("/v1/admin/users")),
    enabled,
  });
}

export function useUpdateUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ principalID, ...body }: Partial<AdminUser> & { principalID: string }) =>
      requestData<AdminUser>(`/v1/admin/users/${encodeURIComponent(principalID)}`, {
        method: "PATCH",
        body,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

export function useToggleUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ principalID, disabled }: { principalID: string; disabled: boolean }) =>
      requestData<AdminUser>(`/v1/admin/users/${encodeURIComponent(principalID)}`, {
        method: "PATCH",
        body: { status: disabled ? "disabled" : "active" },
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

export function useRevokeSessions() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (principalID: string) => requestData(`/v1/admin/users/${encodeURIComponent(principalID)}/sessions`, { method: "DELETE" }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

export function useSetProjectRole() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ principalID, projectID, role }: { principalID: string; projectID: string; role?: string }) =>
      role
        ? requestData(`/v1/admin/users/${encodeURIComponent(principalID)}/projects/${encodeURIComponent(projectID)}`, { method: "PUT", body: { role } })
        : requestData(`/v1/admin/users/${encodeURIComponent(principalID)}/projects/${encodeURIComponent(projectID)}`, { method: "DELETE" }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

export function useCreateInvitation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { email: string; organization_role: string; project_id?: string; project_role?: string; expires_in_hours: number }) =>
      requestData<{ invitation: Invitation; secret?: string } | Invitation>("/v1/admin/invitations", { method: "POST", body }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users }),
  });
}

export function useRevokeInvitation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (invitationID: string) => requestData(`/v1/admin/invitations/${encodeURIComponent(invitationID)}`, { method: "DELETE" }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.users }),
  });
}
