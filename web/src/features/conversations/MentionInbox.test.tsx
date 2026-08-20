import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, useLocation } from "react-router-dom";

import type { RoomMention, RoomMessage } from "@/api/types";

import { MentionInbox } from "./MentionInbox";
import { useMentions, useUpdateMention } from "./queries";

vi.mock("./queries", () => ({
  useMentions: vi.fn(),
  useUpdateMention: vi.fn(),
}));

type MentionWithMessage = RoomMention & { message?: RoomMessage };

const mutate = vi.fn();
const pendingMention: MentionWithMessage = {
  id: "mention-1",
  workspace_id: "workspace-2",
  room_id: "room-1",
  room_name: "arquitectura",
  status: "pending",
  created_at: "2026-08-17T10:00:00Z",
  message: {
    id: "message-1",
    body: "¿Puedes revisar esta decisión?",
    author_display_name: "María",
  },
};

const mockedUseMentions = vi.mocked(useMentions);
const mockedUseUpdateMention = vi.mocked(useUpdateMention);

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname}{location.search}</output>;
}

function renderInbox({
  mentions,
  variant = "button",
  workspaceID,
}: {
  mentions: MentionWithMessage[];
  variant?: "rail" | "button";
  workspaceID?: string;
}) {
  mockedUseMentions.mockReturnValue({
    data: mentions,
    isPending: false,
  } as unknown as ReturnType<typeof useMentions>);

  return render(
    <MemoryRouter initialEntries={["/"]}>
      <MentionInbox workspaceID={workspaceID} variant={variant} />
      <LocationProbe />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  mockedUseUpdateMention.mockReturnValue({
    mutate,
    isPending: false,
    variables: undefined,
  } as unknown as ReturnType<typeof useUpdateMention>);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("MentionInbox", () => {
  it("muestra el total global en el rail y abre la bandeja global", async () => {
    const user = userEvent.setup();
    renderInbox({
      variant: "rail",
      mentions: [pendingMention, { ...pendingMention, id: "mention-2" }, { ...pendingMention, id: "mention-3" }],
    });

    expect(mockedUseMentions).toHaveBeenCalledWith(undefined);
    const trigger = screen.getByRole("button", { name: "Menciones pendientes: 3" });
    expect(trigger).toHaveTextContent("3");

    await user.click(trigger);

    expect(screen.getByRole("heading", { name: "Menciones pendientes" })).toBeInTheDocument();
  });

  it("filtra por workspace y marca como leída la mención abierta antes de navegar", async () => {
    const user = userEvent.setup();
    renderInbox({ mentions: [pendingMention], workspaceID: "workspace-1" });

    expect(mockedUseMentions).toHaveBeenCalledWith("workspace-1");
    expect(screen.getByRole("button", { name: "Menciones · 1" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Menciones · 1" }));
    expect(screen.getByRole("heading", { name: "Menciones de este workspace" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /arquitectura/ }));

    expect(mutate).toHaveBeenCalledWith({ mentionID: "mention-1", status: "read" });
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/w/workspace-2/conversations?room=room-1&message=message-1",
    );
  });
});
