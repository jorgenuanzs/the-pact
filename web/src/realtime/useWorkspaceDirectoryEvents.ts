import { useEffect, useState } from "react";

import { notifySessionExpired } from "@/api/client";
import { desktopBridge, onDesktopStreamMessage } from "@/platform/desktop";

import type { StreamStatus } from "./useProjectEvents";

const RECONNECT_DELAYS = [1_000, 2_000, 4_000, 8_000, 10_000];

function containsDirectoryEvent(block: string): boolean {
  const data = block
    .split(/\r?\n/)
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trimStart())
    .join("\n");
  if (!data) return false;
  try {
    const event = JSON.parse(data) as { type?: string };
    return event.type === "pact.workspace.directory.updated.v1";
  } catch {
    return false;
  }
}

export function useWorkspaceDirectoryEvents(enabled: boolean, refresh: () => void | Promise<void>) {
  const [status, setStatus] = useState<StreamStatus>(enabled ? "connecting" : "idle");

  useEffect(() => {
    if (!enabled) {
      setStatus("idle");
      return;
    }

    const refreshOnFocus = () => { void refresh(); };
    window.addEventListener("focus", refreshOnFocus);

    const native = desktopBridge();
    if (native) {
      let disposed = false;
      let subscriptionID = "";
      const stopListening = onDesktopStreamMessage((message) => {
        if (message.stream !== "directory") return;
        if (message.kind === "event") {
          void refresh();
          return;
        }
        if (!message.status) return;
        setStatus(message.status);
        if (message.status === "connected") void refresh();
        if (message.status === "offline" && message.error?.includes("autorización")) {
          notifySessionExpired();
        }
      });
      void native.StartWorkspaceDirectoryStream()
        .then((id) => {
          if (disposed) {
            void native.StopWorkspaceDirectoryStream(id);
            return;
          }
          subscriptionID = id;
        })
        .catch(() => setStatus("offline"));
      return () => {
        disposed = true;
        window.removeEventListener("focus", refreshOnFocus);
        stopListening();
        if (subscriptionID) void native.StopWorkspaceDirectoryStream(subscriptionID);
      };
    }

    const controller = new AbortController();
    const connect = async () => {
      let attempt = 0;
      while (!controller.signal.aborted) {
        setStatus(attempt ? "reconnecting" : "connecting");
        const connectedAt = Date.now();
        try {
          const response = await fetch("/v1/workspaces/events/stream", {
            headers: { Accept: "text/event-stream" },
            credentials: "same-origin",
            cache: "no-store",
            signal: controller.signal,
          });
          if (response.status === 401) {
            setStatus("offline");
            notifySessionExpired();
            return;
          }
          if (!response.ok || !response.body) throw new Error(`SSE ${response.status}`);
          setStatus("connected");
          void refresh();
          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          let buffer = "";
          while (!controller.signal.aborted) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
            let boundary = buffer.indexOf("\n\n");
            while (boundary >= 0) {
              const block = buffer.slice(0, boundary);
              buffer = buffer.slice(boundary + 2);
              if (containsDirectoryEvent(block)) void refresh();
              boundary = buffer.indexOf("\n\n");
            }
          }
          if (Date.now() - connectedAt > 10_000) attempt = 0;
        } catch {
          if (controller.signal.aborted) break;
          setStatus("offline");
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

    void connect();
    return () => {
      window.removeEventListener("focus", refreshOnFocus);
      controller.abort();
    };
  }, [enabled, refresh]);

  return status;
}
