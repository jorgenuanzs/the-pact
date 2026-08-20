import type { Workspace } from "@/api/types";
import { Icon } from "@/components/ui/Icon";
import { initials, workspaceColorKey } from "@/lib/format";

interface WorkspaceSwitcherProps {
  workspaces: Workspace[];
  selectedWorkspaceID?: string;
  canCreate: boolean;
  onSelect: (workspace: Workspace) => void;
  onCreate: () => void;
}

export function WorkspaceSwitcher({ workspaces, selectedWorkspaceID, canCreate, onSelect, onCreate }: WorkspaceSwitcherProps) {
  return (
    <nav className="workspace-switcher" aria-label="Workspaces">
      <div className="workspace-switcher-list">
        {workspaces.map((workspace) => (
          <button
            key={workspace.id}
            type="button"
            className="workspace-switcher-item"
            data-selected={workspace.id === selectedWorkspaceID || undefined}
            data-color={workspaceColorKey(workspace.color)}
            aria-current={workspace.id === selectedWorkspaceID ? "page" : undefined}
            aria-label={`Abrir workspace ${workspace.name}`}
            title={workspace.name}
            onClick={() => onSelect(workspace)}
          >
            {initials(workspace.name).slice(0, 1)}
          </button>
        ))}
      </div>
      {canCreate ? (
        <button
          type="button"
          className="workspace-switcher-item workspace-switcher-create"
          aria-label="Crear workspace"
          aria-haspopup="dialog"
          title="Crear workspace"
          onClick={onCreate}
        >
          <Icon name="plus" size="lg" />
        </button>
      ) : null}
    </nav>
  );
}
