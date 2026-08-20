import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { requestData } from "@/api/client";
import type { PactEvent } from "@/api/types";

const PAGE_SIZE = 40;

interface EventPage {
  events: PactEvent[];
  next_cursor?: string | null;
  has_more: boolean;
}

interface ProjectFeed {
  events: PactEvent[];
  cursor?: string;
  hasMore: boolean;
}

async function loadProjectPage(projectID: string, query: string, before?: string): Promise<EventPage> {
  const search = new URLSearchParams({ order: "desc", limit: String(PAGE_SIZE) });
  if (before) search.set("before", before);
  if (query) search.set("q", query);
  return requestData<EventPage>(`/v1/projects/${encodeURIComponent(projectID)}/events?${search.toString()}`);
}

function appendUnique(current: PactEvent[], incoming: PactEvent[]): PactEvent[] {
  const events = new Map<string, PactEvent>();
  for (const event of [...current, ...incoming]) {
    const key = String(event.id || `${event.project_id || "project"}:${event.sequence}`);
    events.set(key, event);
  }
  return [...events.values()];
}

export function useWorkspaceActivity(projectIDs: string[], query: string) {
  const projectKey = [...new Set(projectIDs.filter(Boolean))].sort().join(",");
  const activeProjectIDs = useMemo(() => projectKey ? projectKey.split(",") : [], [projectKey]);
  const [feeds, setFeeds] = useState<Record<string, ProjectFeed>>({});
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const requestGeneration = useRef(0);

  useEffect(() => {
    const generation = ++requestGeneration.current;
    setFeeds({});
    setError(null);
    setLoadingMore(false);
    if (!activeProjectIDs.length) {
      setLoading(false);
      return;
    }
    setLoading(true);
    void Promise.all(activeProjectIDs.map(async (projectID) => ({
      projectID,
      page: await loadProjectPage(projectID, query),
    }))).then((pages) => {
      if (requestGeneration.current !== generation) return;
      setFeeds(Object.fromEntries(pages.map(({ projectID, page }) => [projectID, {
        events: page.events || [],
        cursor: page.next_cursor || undefined,
        hasMore: page.has_more,
      }])));
    }).catch((reason: unknown) => {
      if (requestGeneration.current !== generation) return;
      setError(reason instanceof Error ? reason : new Error("No se pudo cargar la actividad."));
    }).finally(() => {
      if (requestGeneration.current === generation) setLoading(false);
    });
  }, [activeProjectIDs, query]);

  const loadMore = useCallback(async () => {
    const targets = activeProjectIDs.filter((projectID) => feeds[projectID]?.hasMore);
    if (!targets.length || loadingMore) return;
    const generation = requestGeneration.current;
    setLoadingMore(true);
    setError(null);
    try {
      const pages = await Promise.all(targets.map(async (projectID) => ({
        projectID,
        page: await loadProjectPage(projectID, query, feeds[projectID]?.cursor),
      })));
      if (requestGeneration.current !== generation) return;
      setFeeds((current) => {
        const next = { ...current };
        for (const { projectID, page } of pages) {
          next[projectID] = {
            events: appendUnique(current[projectID]?.events || [], page.events || []),
            cursor: page.next_cursor || undefined,
            hasMore: page.has_more,
          };
        }
        return next;
      });
    } catch (reason) {
      if (requestGeneration.current !== generation) return;
      setError(reason instanceof Error ? reason : new Error("No se pudo cargar más actividad."));
    } finally {
      if (requestGeneration.current === generation) setLoadingMore(false);
    }
  }, [activeProjectIDs, feeds, loadingMore, query]);

  return {
    events: Object.values(feeds).flatMap((feed) => feed.events),
    hasMore: Object.values(feeds).some((feed) => feed.hasMore),
    loading,
    loadingMore,
    error,
    loadMore,
  };
}
