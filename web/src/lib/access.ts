import type { Actor, Principal, ProjectAccess } from "@/api/types";

const ROLE_RANK: Record<string, number> = {
  viewer: 0,
  contributor: 1,
  maintainer: 2,
  owner: 3,
};

export function roleAtLeast(role: unknown, minimum: keyof typeof ROLE_RANK): boolean {
  return (ROLE_RANK[String(role || "")] ?? -1) >= ROLE_RANK[minimum];
}

export function currentProjectRole(access: ProjectAccess | undefined, principal: Principal | undefined): string {
  const members = Array.isArray(access?.members) ? access.members : [];
  const current = members.find((member: Actor) => String(member.principal_id || member.id) === principal?.id);
  return String(current?.effective_role || current?.project_role || "");
}
