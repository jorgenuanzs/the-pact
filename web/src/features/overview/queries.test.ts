import { describe, expect, it } from "vitest";

import type { ProjectAccess, ProjectOverview } from "@/api/types";

import { mergeProjectOverviews, mergeWorkspaceAccess } from "./queries";

describe("mergeProjectOverviews", () => {
  it("agrega las particiones internas en una única vista de workspace", () => {
    const first: ProjectOverview = {
      project: { id: "project-1", name: "API" },
      counts: { active_intents: 2, active_sessions: 1 },
      active_work: [{ id: "session-1" }],
      work_items: [{ id: "intent-1" }],
      handoffs: [{ id: "handoff-1" }],
      recent_events: [{ id: "event-1" }],
      repository_sync: { state: "synced", repository_id: "repository-1" },
      code_activity: { state: "recent", project_id: "project-1" },
      generated_at: "2026-08-17T10:00:00Z",
    };
    const second: ProjectOverview = {
      project: { id: "project-2", name: "Web" },
      counts: { active_intents: 3, repositories: 2 },
      active_work: [{ id: "session-2" }],
      work_items: [{ id: "intent-2" }],
      handoffs: [{ id: "handoff-2" }],
      events: [{ id: "event-2" }],
      repository_sync: { state: "failed", repository_id: "repository-2" },
      code_activity: { state: "editing", project_id: "project-2" },
      generated_at: "2026-08-17T11:00:00Z",
    };

    const merged = mergeProjectOverviews([first, second]);

    expect(merged.project).toBe(first.project);
    expect(merged.counts).toEqual({ active_intents: 5, active_sessions: 1, repositories: 2 });
    expect(merged.active_work).toEqual([{ id: "session-1" }, { id: "session-2" }]);
    expect(merged.work_items).toEqual([{ id: "intent-1" }, { id: "intent-2" }]);
    expect(merged.handoffs).toEqual([{ id: "handoff-1" }, { id: "handoff-2" }]);
    expect(merged.recent_events).toEqual([{ id: "event-1" }, { id: "event-2" }]);
    expect(merged.repository_sync).toBe(first.repository_sync);
    expect(merged.code_activity).toBe(second.code_activity);
    expect(merged.generated_at).toBe("2026-08-17T11:00:00Z");
    expect(merged.project_overviews).toEqual([first, second]);
  });

  it("devuelve colecciones vacías cuando el workspace no tiene particiones", () => {
    expect(mergeProjectOverviews([])).toMatchObject({
      counts: {},
      active_work: [],
      work_items: [],
      handoffs: [],
      recent_events: [],
      project_overviews: [],
    });
  });
});

describe("mergeWorkspaceAccess", () => {
  it("consolida las ejecuciones del mismo tipo y responsable en un agente lógico", () => {
    const access: ProjectAccess = {
      members: [],
      agents: [
        { agent_id: "codex-base", display_name: "Codex", agent_type: "codex", sponsor_principal_id: "jorge", sponsor_display_name: "Jorge", status: "active", connected: true, active_sessions: 1, session_count: 4, last_seen_at: "2026-08-17T10:00:00Z" },
        { agent_id: "codex-task", display_name: "codex-release-check", agent_type: "codex", sponsor_principal_id: "jorge", sponsor_display_name: "Jorge", status: "active", connected: false, active_sessions: 0, session_count: 2, last_seen_at: "2026-08-17T09:00:00Z" },
        { agent_id: "claude", display_name: "Claude", agent_type: "claude", sponsor_principal_id: "jorge", sponsor_display_name: "Jorge", status: "active", connected: false, active_sessions: 0, session_count: 1, last_seen_at: "2026-08-16T09:00:00Z" },
        { agent_id: "codex-maria", display_name: "Codex", agent_type: "codex", sponsor_principal_id: "maria", sponsor_display_name: "María", status: "active", connected: false, active_sessions: 0, session_count: 3, last_seen_at: "2026-08-15T09:00:00Z" },
      ],
    };

    const merged = mergeWorkspaceAccess([access]);

    expect(merged.agents).toHaveLength(3);
    expect(merged.agents).toContainEqual(expect.objectContaining({
      logical_agent_key: "jorge:codex",
      display_name: "Codex",
      connected: true,
      active_sessions: 1,
      session_count: 6,
      identity_count: 2,
      aliases: ["Codex", "codex-release-check"],
      last_seen_at: "2026-08-17T10:00:00Z",
    }));
  });
});
