import { useMutation, useQueryClient } from "@tanstack/react-query";

import { requestData } from "@/api/client";
import { queryKeys } from "@/api/queryKeys";
import type { AgentEnrollmentResult } from "@/api/types";

export function useEnrollAgent(workspaceID: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { project_id: string; agent_type: "codex" | "claude"; client_type: string }) =>
      requestData<AgentEnrollmentResult>(
        `/v1/workspaces/${encodeURIComponent(workspaceID)}/agent-enrollments`,
        { method: "POST", body },
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.workspaceAccess(workspaceID) }),
  });
}
