import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { idempotencyKey, requestData } from "@/api/client";
import { queryKeys } from "@/api/queryKeys";
import type { Actor, Room, RoomMention, RoomMessage } from "@/api/types";

function listFrom<T>(payload: unknown, key: string): T[] {
  if (Array.isArray(payload)) return payload as T[];
  const record = payload as Record<string, unknown> | null;
  return Array.isArray(record?.[key]) ? record[key] as T[] : [];
}

export function useRooms(workspaceID?: string) {
  return useQuery({
    queryKey: queryKeys.rooms(workspaceID || "none"),
    queryFn: async () => listFrom<Room>(
      await requestData<unknown>(`/v1/workspaces/${encodeURIComponent(workspaceID!)}/rooms`),
      "rooms",
    ),
    enabled: Boolean(workspaceID),
    refetchInterval: 5_000,
  });
}

export function useParticipants(workspaceID?: string) {
  return useQuery({
    queryKey: queryKeys.participants(workspaceID || "none"),
    queryFn: async () => listFrom<Actor>(
      await requestData<unknown>(`/v1/workspaces/${encodeURIComponent(workspaceID!)}/participants`),
      "participants",
    ),
    enabled: Boolean(workspaceID),
  });
}

export function useRoomMessages(workspaceID?: string, roomID?: string) {
  return useQuery({
    queryKey: queryKeys.messages(workspaceID || "none", roomID || "none"),
    queryFn: async () => listFrom<RoomMessage>(
      await requestData<unknown>(`/v1/workspaces/${encodeURIComponent(workspaceID!)}/rooms/${encodeURIComponent(roomID!)}/messages?limit=50`),
      "messages",
    ),
    enabled: Boolean(workspaceID && roomID),
    refetchInterval: 5_000,
  });
}

export function useMentions(workspaceID?: string) {
  const query = workspaceID
    ? `?workspace_id=${encodeURIComponent(workspaceID)}&status=pending&limit=100`
    : "?status=pending&limit=100";
  return useQuery({
    queryKey: queryKeys.mentions(workspaceID),
    queryFn: async () => listFrom<RoomMention>(
      await requestData<unknown>(`/v1/me/room-mentions${query}`),
      "mentions",
    ),
    refetchInterval: 5_000,
  });
}

export function useCreateRoom(workspaceID: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; description: string }) => requestData<Room>(
      `/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey("room-create") },
        body: input,
      },
    ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.rooms(workspaceID) }),
  });
}

export function usePostMessage(workspaceID: string, roomID: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { body: string; reply_to_message_id?: string; mention_actor_ids?: string[] }) => requestData<RoomMessage>(
      `/v1/workspaces/${encodeURIComponent(workspaceID)}/rooms/${encodeURIComponent(roomID)}/messages`,
      {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey("room-message") },
        body: input,
      },
    ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.messages(workspaceID, roomID) }),
  });
}

export function useUpdateMention(workspaceID?: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ mentionID, status }: { mentionID: string; status: "read" | "dismissed" }) =>
      requestData(`/v1/me/room-mentions/${encodeURIComponent(mentionID)}/status`, {
        method: "POST",
        body: { status },
      }),
    onSuccess: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.mentions(workspaceID) }),
      queryClient.invalidateQueries({ queryKey: queryKeys.mentions(undefined) }),
    ]),
  });
}
