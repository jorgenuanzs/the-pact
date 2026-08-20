import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { RoomMessage } from "@/api/types";

import { MessageComposer } from "./MessageComposer";
import { MessageList } from "./MessageList";

const backendMessage: RoomMessage = {
  id: "message-1",
  room_id: "room-1",
  author_actor_id: "actor-1",
  author_display_name: "Agente Revisor",
  author_kind: "agent",
  body: "La revisión terminó.",
  created_at: "2026-08-17T10:00:00Z",
};

afterEach(() => cleanup());

describe("mensajes de conversación", () => {
  it("renderiza los campos author_* devueltos por el backend", () => {
    const { container } = render(
      <MessageList messages={[backendMessage]} loading={false} onReply={vi.fn()} />,
    );

    expect(screen.getByText("Agente Revisor")).toBeInTheDocument();
    expect(screen.getByText("La revisión terminó.")).toBeInTheDocument();
    expect(container.querySelector('.pact-avatar[data-kind="agent"]')).toBeInTheDocument();
  });

  it("conserva el autor correcto al responder", async () => {
    const user = userEvent.setup();
    render(
      <MessageComposer
        participants={[]}
        reply={backendMessage}
        sending={false}
        onCancelReply={vi.fn()}
        onSend={vi.fn()}
      />,
    );

    expect(screen.getByText("Agente Revisor")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Mensaje"), "Entendido");
    expect(screen.getByDisplayValue("Entendido")).toBeInTheDocument();
  });
});
