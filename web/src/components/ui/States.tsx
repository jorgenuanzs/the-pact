import type { ReactNode } from "react";

import { Button } from "./Button";
import { Icon } from "./Icon";

interface StateProps {
  title: string;
  description?: string;
  icon?: ReactNode;
  actionLabel?: string;
  onAction?: () => void;
}

function StateFrame({
  title,
  description,
  icon,
  actionLabel,
  onAction,
  tone,
}: StateProps & { tone?: "neutral" | "danger" }) {
  return (
    <div className="pact-state" data-tone={tone}>
      <div className="pact-state-content">
        {icon && <span className="pact-state-glyph" aria-hidden="true">{icon}</span>}
        <h3 className="pact-state-title">{title}</h3>
        {description && <p className="pact-state-description">{description}</p>}
        {actionLabel && onAction && (
          <Button variant="secondary" size="sm" onClick={onAction}>
            {actionLabel}
          </Button>
        )}
      </div>
    </div>
  );
}

export function EmptyState(props: StateProps) {
  return <StateFrame icon={<Icon name="empty" size="lg" />} {...props} />;
}

export function ErrorState(props: StateProps) {
  return <StateFrame tone="danger" icon="!" {...props} />;
}

export interface LoadingStateProps {
  label?: string;
  compact?: boolean;
}

export function LoadingState({ label = "Cargando", compact = false }: LoadingStateProps) {
  if (compact) {
    return (
      <span role="status" aria-label={label}>
        <span className="pact-loading-spinner" aria-hidden="true" />
        <span className="pact-visually-hidden">{label}</span>
      </span>
    );
  }

  return (
    <div className="pact-state" role="status" aria-label={label}>
      <div className="pact-state-content">
        <span className="pact-loading-spinner" aria-hidden="true" />
        <strong className="pact-state-title">{label}</strong>
        <div className="pact-loading-lines" aria-hidden="true">
          <span className="pact-loading-line" />
          <span className="pact-loading-line" />
          <span className="pact-loading-line" />
        </div>
      </div>
    </div>
  );
}
