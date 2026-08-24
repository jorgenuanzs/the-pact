import { describe, expect, it } from "vitest";

import { normalizeWorkspaces } from "./viewModels";

describe("normalizeWorkspaces", () => {
  it("hides archived compatibility workspaces from the active directory", () => {
    expect(normalizeWorkspaces({
      workspaces: [
        { id: "magi", name: "Magi", slug: "magi", status: "active" },
        { id: "infra", name: "infra", slug: "infra", status: "archived" },
      ],
    }).map((workspace) => workspace.id)).toEqual(["magi"]);
  });
});
