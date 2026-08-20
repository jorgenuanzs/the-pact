import { Link } from "react-router-dom";

import { Icon, type IconName } from "@/components/ui/Icon";

export interface WorkspaceSectionEntry {
  to: string;
  icon: IconName;
  label: string;
  description: string;
  value: string;
  tone?: "active" | "warning" | "neutral";
}

export function WorkspaceSectionIndex({ entries }: { entries: WorkspaceSectionEntry[] }) {
  return (
    <nav className="workspace-section-index" aria-label="Secciones del workspace">
      <header className="overview-section-heading">
        <span><p className="pact-kicker">NAVEGACIÓN</p><h2>Explorar el workspace</h2></span>
      </header>
      <div>
        {entries.map((entry) => (
          <Link to={entry.to} key={entry.to} className="workspace-section-link" data-tone={entry.tone || "neutral"}>
            <span className="workspace-section-icon"><Icon name={entry.icon} /></span>
            <span className="workspace-section-copy"><strong>{entry.label}</strong><small>{entry.description}</small></span>
            <span className="workspace-section-value">{entry.value}</span>
            <span className="workspace-section-arrow" aria-hidden="true">→</span>
          </Link>
        ))}
      </div>
    </nav>
  );
}
