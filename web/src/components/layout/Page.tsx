import type { ReactNode } from "react";

import { cx } from "@/components/ui/utils";
import { PageHeader } from "./PageHeader";

export function Page({
  kicker,
  title,
  actions,
  fullBleed = false,
  showWorkspaceStatus = true,
  className,
  children,
}: {
  kicker?: string;
  title: string;
  description?: string;
  actions?: ReactNode;
  fullBleed?: boolean;
  showWorkspaceStatus?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cx("pact-page", className)} data-layout={fullBleed ? "full-bleed" : undefined}>
      <PageHeader kicker={kicker} title={title} actions={actions} showWorkspaceStatus={showWorkspaceStatus} />
      <div className="pact-page-content">{children}</div>
    </div>
  );
}
