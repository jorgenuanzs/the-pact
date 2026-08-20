import { useEffect, useMemo, useState } from "react";

import type { PactEvent } from "@/api/types";
import {
  Button,
  DataTable,
  DataTableBody,
  DataTableCell,
  DataTableHead,
  DataTableHeaderCell,
  DataTableRow,
  Icon,
} from "@/components/ui";
import { EmptyState } from "@/components/ui/States";
import { formatDate, shortID, text } from "@/lib/format";

import { activityLabel } from "./ActivityTimeline";

const PAGE_SIZE = 20;

function eventSequence(event: PactEvent): bigint {
  try {
    return BigInt(String(event.sequence ?? 0));
  } catch {
    return 0n;
  }
}

function eventTimestamp(event: PactEvent): number {
  const timestamp = Date.parse(String(event.occurred_at || event.created_at || ""));
  return Number.isNaN(timestamp) ? 0 : timestamp;
}

function actorName(event: PactEvent): string {
  return text(event.actor?.display_name || event.actor_name || event.actor_id, "PACT");
}

function eventData(event: PactEvent): Record<string, unknown> {
  return event.data || event.payload || {};
}

function reference(event: PactEvent): string {
  const data = eventData(event);
  return text(
    data.repository
      || data.repository_name
      || data.scope
      || data.path
      || data.intent_title
      || event.aggregate_type,
    "—",
  );
}

function searchableText(event: PactEvent): string {
  return [
    activityLabel(String(event.type || event.event_type || "")),
    event.type,
    event.event_type,
    actorName(event),
    event.actor_id,
    event.sequence,
    reference(event),
    JSON.stringify(eventData(event)),
  ].join(" ").toLocaleLowerCase("es");
}

export function ActivityTable({ events }: { events: PactEvent[] }) {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const normalizedSearch = search.trim().toLocaleLowerCase("es");
  const searchTerms = useMemo(() => normalizedSearch.split(/\s+/).filter(Boolean), [normalizedSearch]);
  const filtered = useMemo(() => [...events]
    .sort((left, right) => eventTimestamp(right) - eventTimestamp(left)
      || (eventSequence(right) > eventSequence(left) ? 1 : -1))
    .filter((event) => {
      const searchable = searchableText(event);
      return searchTerms.every((term) => searchable.includes(term));
    }), [events, searchTerms]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const start = (page - 1) * PAGE_SIZE;
  const visible = filtered.slice(start, start + PAGE_SIZE);

  useEffect(() => {
    setPage((current) => Math.min(current, pageCount));
  }, [pageCount]);

  const updateSearch = (value: string) => {
    setSearch(value);
    setPage(1);
  };

  return (
    <section className="activity-register" aria-label="Registro de actividad">
      <div className="activity-register-toolbar">
        <label className="activity-register-search">
          <Icon name="search" size="sm" />
          <input
            type="search"
            placeholder="Buscar por evento, actor, repositorio o scope"
            value={search}
            onChange={(event) => updateSearch(event.target.value)}
          />
        </label>
        <span className="activity-register-result-count">
          {filtered.length === 1 ? "1 actividad" : `${filtered.length} actividades`}
        </span>
      </div>

      {events.length === 0 ? (
        <EmptyState title="Aún no hay actividad" description="Los eventos durables del workspace aparecerán aquí." />
      ) : (
        <DataTable containerClassName="activity-table-wrap" className="activity-table">
          <DataTableHead>
            <DataTableRow>
              <DataTableHeaderCell>Actividad</DataTableHeaderCell>
              <DataTableHeaderCell>Actor</DataTableHeaderCell>
              <DataTableHeaderCell>Referencia</DataTableHeaderCell>
              <DataTableHeaderCell>Secuencia</DataTableHeaderCell>
              <DataTableHeaderCell>Fecha</DataTableHeaderCell>
              <DataTableHeaderCell><span className="sr-only">Datos</span></DataTableHeaderCell>
            </DataTableRow>
          </DataTableHead>
          <DataTableBody>
            {visible.length ? visible.map((event, index) => {
              const data = eventData(event);
              const hasData = Object.keys(data).length > 0;
              return (
                <DataTableRow key={text(event.id || event.sequence, String(start + index))}>
                  <DataTableCell>
                    <strong>{activityLabel(String(event.type || event.event_type || ""))}</strong>
                    <span className="table-detail activity-event-type">{text(event.type || event.event_type)}</span>
                  </DataTableCell>
                  <DataTableCell>{actorName(event)}</DataTableCell>
                  <DataTableCell><code>{reference(event)}</code></DataTableCell>
                  <DataTableCell><code>{shortID(event.sequence, 12)}</code></DataTableCell>
                  <DataTableCell><time>{formatDate(event.occurred_at || event.created_at)}</time></DataTableCell>
                  <DataTableCell className="activity-data-cell">
                    {hasData ? <details><summary>Ver datos</summary><pre>{JSON.stringify(data, null, 2)}</pre></details> : <span>—</span>}
                  </DataTableCell>
                </DataTableRow>
              );
            }) : (
              <DataTableRow>
                <DataTableCell colSpan={6}>
                  <div className="activity-no-results">No hay actividades que coincidan con “{search.trim()}”.</div>
                </DataTableCell>
              </DataTableRow>
            )}
          </DataTableBody>
        </DataTable>
      )}

      {events.length > 0 && filtered.length > 0 ? (
        <footer className="activity-pagination">
          <span>{start + 1}–{Math.min(start + PAGE_SIZE, filtered.length)} de {filtered.length}</span>
          <div>
            <Button variant="secondary" size="sm" disabled={page === 1} onClick={() => setPage((current) => current - 1)}>← Anterior</Button>
            <span>Página {page} de {pageCount}</span>
            <Button variant="secondary" size="sm" disabled={page === pageCount} onClick={() => setPage((current) => current + 1)}>Siguiente →</Button>
          </div>
        </footer>
      ) : null}
    </section>
  );
}
