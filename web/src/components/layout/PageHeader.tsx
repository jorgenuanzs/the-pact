import type { ReactNode } from "react";

import { WorkspaceLiveStatus } from "./WorkspaceLiveStatus";

interface PageHeaderProps {
  kicker?: string;
  title: string;
  actions?: ReactNode;
  showWorkspaceStatus?: boolean;
}

export function PageHeader({ kicker, title, actions, showWorkspaceStatus = true }: PageHeaderProps) {
  return (
    <header className="pact-page-header">
      <div>
        {kicker ? <span className="pact-visually-hidden">{kicker}</span> : null}
        <h1>{title}</h1>
      </div>
      {actions || showWorkspaceStatus ? (
        <div className="pact-page-actions">
          {actions}
          {showWorkspaceStatus ? <WorkspaceLiveStatus /> : null}
        </div>
      ) : null}
    </header>
  );
}
