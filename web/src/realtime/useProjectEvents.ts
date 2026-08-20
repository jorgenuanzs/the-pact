import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { queryKeys } from "@/api/queryKeys";
import type { PactEvent } from "@/api/types";
import { notifySessionExpired } from "@/api/client";
import { desktopBridge, onDesktopStreamMessage } from "@/platform/desktop";

export type StreamStatus = "idle" | "connecting" | "connected" | "reconnecting" | "offline";

const RECONNECT_DELAYS = [1_000, 2_000, 4_000, 8_000, 10_000];

function eventSequence(event: PactEvent): bigint {
  try {
    return BigInt(String(event.sequence ?? event.id ?? 0));
  } catch {
    return 0n;
  }
}

function mergeEvents(current: PactEvent[], incoming: PactEvent[]): PactEvent[] {
  const byID = new Map<string, PactEvent>();
  for (const event of [...current, ...incoming]) {
    const key = String(event.id ?? event.sequence ?? JSON.stringify(event));
    byID.set(key, event);
  }
  return [...byID.values()]
    .sort((left, right) => eventSequence(left) > eventSequence(right) ? -1 : 1)
    .slice(0, 50);
}

function parseMessage(block: string): { id?: string; data?: PactEvent } {
  let id = "";
  const data: string[] = [];
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith("id:")) id = line.slice(3).trim();
    if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
  }
  if (!data.length) return {};
  try {
    const parsed = JSON.parse(data.join("\n")) as PactEvent;
    if (id && !parsed.id) parsed.id = id;
    return { id, data: parsed };
  } catch {
    return {};
  }
}

export function useProjectEvents(projectID?: string) {
  return useWorkspaceEvents(projectID ? [projectID] : []);
}

export function useWorkspaceEvents(projectIDs: string[]) {
  const queryClient = useQueryClient();
  const projectKey = [...new Set(projectIDs.filter(Boolean))].sort().join(",");
  const activeProjectIDs = projectKey ? projectKey.split(",") : [];
  const [statuses, setStatuses] = useState<Record<string, StreamStatus>>({});
  const [events, setEvents] = useState<PactEvent[]>([]);
  const cursors = useRef(new Map<string, string>());

  useEffect(() => {
    setEvents([]);
    cursors.current.clear();
    if (!projectKey) {
      setStatuses({});
      return;
    }

    const controller = new AbortController();
    const refreshTimers = new Map<string, number>();
    setStatuses(Object.fromEntries(activeProjectIDs.map((projectID) => [projectID, "connecting"] as const)));

    const setProjectStatus = (projectID: string, status: StreamStatus) => {
      setStatuses((current) => ({ ...current, [projectID]: status }));
    };

    const receive = (projectID: string, event: PactEvent, id?: string) => {
      if (id) cursors.current.set(projectID, id);
      else if (event.sequence !== undefined) cursors.current.set(projectID, String(event.sequence));
      setEvents((current) => mergeEvents(current, [event]));
      const currentTimer = refreshTimers.get(projectID);
      if (currentTimer) window.clearTimeout(currentTimer);
      refreshTimers.set(projectID, window.setTimeout(() => {
        void queryClient.invalidateQueries({ queryKey: queryKeys.overview(projectID) });
      }, 250));
    };

    const native = desktopBridge();
    if (native) {
      let disposed = false;
      const subscriptions = new Set<string>();
      const stopListening = onDesktopStreamMessage((message) => {
        if (!activeProjectIDs.includes(message.project_id)) return;
        if (message.kind === "event" && message.data) {
          receive(message.project_id, message.data, message.event_id);
          return;
        }
        if (message.kind === "status" && message.status) {
          setProjectStatus(message.project_id, message.status);
          if (message.status === "offline" && message.error?.includes("autorización")) {
            notifySessionExpired();
          }
        }
      });
      for (const projectID of activeProjectIDs) {
        void native.StartProjectEventStream(projectID, cursors.current.get(projectID) || "")
          .then((subscriptionID) => {
            if (disposed) {
              void native.StopProjectEventStream(subscriptionID);
              return;
            }
            subscriptions.add(subscriptionID);
          })
          .catch(() => setProjectStatus(projectID, "offline"));
      }
      return () => {
        disposed = true;
        stopListening();
        for (const subscriptionID of subscriptions) {
          void native.StopProjectEventStream(subscriptionID);
        }
        for (const timer of refreshTimers.values()) window.clearTimeout(timer);
      };
    }

    const connect = async (projectID: string) => {
      let attempt = 0;
      while (!controller.signal.aborted) {
        setProjectStatus(projectID, attempt ? "reconnecting" : "connecting");
        const connectedAt = Date.now();
        try {
          const headers = new Headers({ Accept: "text/event-stream" });
          const cursor = cursors.current.get(projectID);
          if (cursor) headers.set("Last-Event-ID", cursor);
          const response = await fetch(`/v1/projects/${encodeURIComponent(projectID)}/events/stream`, {
            headers,
            credentials: "same-origin",
            cache: "no-store",
            signal: controller.signal,
          });
          if (response.status === 401) {
            setProjectStatus(projectID, "offline");
            notifySessionExpired();
            return;
          }
          if (!response.ok || !response.body) throw new Error(`SSE ${response.status}`);
          setProjectStatus(projectID, "connected");
          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          let buffer = "";
          while (!controller.signal.aborted) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
            let boundary = buffer.indexOf("\n\n");
            while (boundary >= 0) {
              const message = parseMessage(buffer.slice(0, boundary));
              buffer = buffer.slice(boundary + 2);
              if (message.data) receive(projectID, message.data, message.id);
              boundary = buffer.indexOf("\n\n");
            }
          }
          if (Date.now() - connectedAt > 10_000) attempt = 0;
        } catch (error) {
          if (controller.signal.aborted) break;
          setProjectStatus(projectID, "offline");
        }
        const delay = RECONNECT_DELAYS[Math.min(attempt, RECONNECT_DELAYS.length - 1)];
        attempt += 1;
        await new Promise<void>((resolve) => {
          const timeout = window.setTimeout(resolve, delay);
          controller.signal.addEventListener("abort", () => {
            window.clearTimeout(timeout);
            resolve();
          }, { once: true });
        });
      }
    };

    for (const projectID of activeProjectIDs) void connect(projectID);
    return () => {
      controller.abort();
      for (const timer of refreshTimers.values()) window.clearTimeout(timer);
    };
  // projectKey is a canonical representation of the IDs and prevents callers'
  // freshly allocated arrays from reconnecting every render.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectKey, queryClient]);

  const values = Object.values(statuses);
  const status: StreamStatus = !projectKey
    ? "idle"
    : values.length && values.every((value) => value === "connected")
      ? "connected"
      : values.some((value) => value === "offline")
        ? "offline"
        : values.some((value) => value === "reconnecting")
          ? "reconnecting"
          : "connecting";
  return { status, events };
}
