import { useQueries, useQuery } from "@tanstack/react-query";

import { requestData } from "@/api/client";
import { queryKeys } from "@/api/queryKeys";
import type { ProjectAccess, ProjectOverview, WorkspaceContext } from "@/api/types";

type OverviewRecord = Record<string, unknown>;

function records(value: unknown): OverviewRecord[] {
  return Array.isArray(value) ? value as OverviewRecord[] : [];
}

function mergeCounts(overviews: ProjectOverview[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const overview of overviews) {
    for (const [key, value] of Object.entries(overview.counts || {})) {
      if (typeof value === "number") counts[key] = (counts[key] || 0) + value;
    }
  }
  return counts;
}

/**
 * The public product model is one workspace with any number of repositories.
 * Older servers can still contain more than one internal project partition, so
 * the UI folds those partitions into one workspace-wide read model.
 */
export function mergeProjectOverviews(overviews: ProjectOverview[]): ProjectOverview {
  const generatedAt = overviews
    .map((overview) => overview.generated_at)
    .filter((value): value is string => Boolean(value))
    .sort()
    .at(-1);

  return {
    project: overviews[0]?.project,
    counts: mergeCounts(overviews),
    active_work: overviews.flatMap((overview) => overview.active_work || []),
    work_items: overviews.flatMap((overview) => records(overview.work_items)),
    handoffs: overviews.flatMap((overview) => records(overview.handoffs)),
    recent_events: overviews.flatMap((overview) => overview.recent_events || overview.events || []),
    repository_sync: overviews.find((overview) => overview.repository_sync)?.repository_sync,
    code_activity: overviews
      .map((overview) => overview.code_activity)
      .find((activity) => activity?.state === "editing")
      || overviews.map((overview) => overview.code_activity).find((activity) => activity?.state === "recent")
      || overviews.map((overview) => overview.code_activity).find(Boolean),
    generated_at: generatedAt,
    project_overviews: overviews,
  };
}

export function useProjectOverview(projectID?: string) {
  return useQuery({
    queryKey: queryKeys.overview(projectID || "none"),
    queryFn: () => requestData<ProjectOverview>(`/v1/projects/${encodeURIComponent(projectID!)}/overview`),
    enabled: Boolean(projectID),
    refetchInterval: 5_000,
  });
}

export function useWorkspaceOverview(projectIDs: string[]) {
  const uniqueIDs = [...new Set(projectIDs.filter(Boolean))];
  const queries = useQueries({
    queries: uniqueIDs.map((projectID) => ({
      queryKey: queryKeys.overview(projectID),
      queryFn: () => requestData<ProjectOverview>(`/v1/projects/${encodeURIComponent(projectID)}/overview`),
      refetchInterval: 5_000,
    })),
  });
  const overviews = queries.flatMap((query) => query.data ? [query.data] : []);
  return {
    data: mergeProjectOverviews(overviews),
    overviews,
    isPending: uniqueIDs.length > 0 && queries.some((query) => query.isPending),
    error: queries.find((query) => query.error)?.error,
    refetch: async () => Promise.all(queries.map((query) => query.refetch())),
  };
}

export function useWorkspaceContext(workspaceID?: string) {
  return useQuery({
    queryKey: queryKeys.context(workspaceID || "none"),
    queryFn: () => requestData<WorkspaceContext>(`/v1/workspaces/${encodeURIComponent(workspaceID!)}/context`),
    enabled: Boolean(workspaceID),
  });
}

export function useProjectAccess(projectID?: string) {
  return useQuery({
    queryKey: queryKeys.access(projectID || "none"),
    queryFn: () => requestData<ProjectAccess>(`/v1/projects/${encodeURIComponent(projectID!)}/access`),
    enabled: Boolean(projectID),
  });
}

function mergeIdentityLists(overviews: ProjectAccess[], keys: string[]): OverviewRecord[] {
  const roleRank: Record<string, number> = { viewer: 0, contributor: 1, maintainer: 2, owner: 3 };
  const merged = new Map<string, OverviewRecord>();
  for (const access of overviews) {
    for (const key of keys) {
      const values = records(access[key]);
      for (const value of values) {
        const identity = String(value.principal_id || value.agent_id || value.actor_id || `${key}-${merged.size}`);
        const previous = merged.get(identity);
        const previousRole = String(previous?.effective_role || previous?.project_role || "");
        const nextRole = String(value.effective_role || value.project_role || "");
        const strongestRole = (roleRank[previousRole] ?? -1) > (roleRank[nextRole] ?? -1) ? previousRole : nextRole;
        merged.set(identity, {
          ...previous,
          ...value,
          ...(strongestRole ? { effective_role: strongestRole } : {}),
        });
      }
    }
  }
  return [...merged.values()];
}

function agentTypeLabel(agentType: string): string {
  const acronyms: Record<string, string> = { ai: "AI", api: "API", cli: "CLI", mcp: "MCP", pact: "PACT" };
  return agentType
    .split(/[-_.\s]+/)
    .filter(Boolean)
    .map((part) => acronyms[part.toLowerCase()] || `${part.charAt(0).toUpperCase()}${part.slice(1).toLowerCase()}`)
    .join(" ");
}

function latestRecord(previous: OverviewRecord | undefined, next: OverviewRecord): OverviewRecord {
  const previousTime = Date.parse(String(previous?.last_seen_at || ""));
  const nextTime = Date.parse(String(next.last_seen_at || ""));
  return !previous || (!Number.isNaN(nextTime) && (Number.isNaN(previousTime) || nextTime >= previousTime)) ? next : previous;
}

/**
 * Agent names used by older clients often described a run (for example,
 * `codex-release-check`) instead of a durable identity. At workspace level an
 * agent is the runtime type sponsored by one user; its runs remain available
 * in live work and activity.
 */
export function mergeWorkspaceAgents(overviews: ProjectAccess[]): OverviewRecord[] {
  const groups = new Map<string, { value: OverviewRecord; ids: Set<string>; aliases: Set<string> }>();
  for (const access of overviews) {
    for (const agent of records(access.agents)) {
      const agentType = String(agent.agent_type || agent.last_client_type || "agent").replace(/-mcp$/i, "").toLowerCase();
      const sponsorID = String(agent.sponsor_principal_id || "unsponsored");
      const key = `${sponsorID}:${agentType}`;
      const previous = groups.get(key);
      const recent = latestRecord(previous?.value, agent);
      const ids = previous?.ids || new Set<string>();
      const aliases = previous?.aliases || new Set<string>();
      ids.add(String(agent.agent_id || `${key}:${ids.size}`));
      if (agent.display_name) aliases.add(String(agent.display_name));
      groups.set(key, {
        ids,
        aliases,
        value: {
          ...previous?.value,
          ...recent,
          logical_agent_key: key,
          display_name: agentTypeLabel(agentType),
          agent_type: agentType,
          status: previous?.value.status === "active" || agent.status === "active" ? "active" : recent.status,
          connected: Boolean(previous?.value.connected) || Boolean(agent.connected),
          access_active: Boolean(previous?.value.access_active) || Boolean(agent.access_active),
          active_sessions: Number(previous?.value.active_sessions || 0) + Number(agent.active_sessions || 0),
          session_count: Number(previous?.value.session_count || 0) + Number(agent.session_count || 0),
          last_seen_at: recent.last_seen_at,
        },
      });
    }
  }
  return [...groups.values()].map(({ value, ids, aliases }) => ({
    ...value,
    identity_count: ids.size,
    aliases: [...aliases],
  }));
}

export function mergeWorkspaceAccess(overviews: ProjectAccess[]): ProjectAccess {
  return {
    members: mergeIdentityLists(overviews, ["members", "people", "principals"]),
    agents: mergeWorkspaceAgents(overviews),
    sessions: mergeIdentityLists(overviews, ["sessions"]),
  };
}

export function useWorkspaceAccess(workspaceID?: string) {
  return useQuery({
    queryKey: queryKeys.workspaceAccess(workspaceID || "none"),
    queryFn: () => requestData<ProjectAccess>(`/v1/workspaces/${encodeURIComponent(workspaceID!)}/access`),
    enabled: Boolean(workspaceID),
    refetchInterval: 5_000,
    select: (access) => mergeWorkspaceAccess([access]),
  });
}
