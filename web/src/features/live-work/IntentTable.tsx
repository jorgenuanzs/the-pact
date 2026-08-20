import { useState } from "react";

import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { DataTable, DataTableBody, DataTableCell, DataTableHead, DataTableHeaderCell, DataTableRow } from "@/components/ui/DataTable";
import { Dialog, DialogBody, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/Dialog";
import { EmptyState } from "@/components/ui/States";
import { StatusChip, type StatusTone } from "@/components/ui/StatusChip";
import { relativeDate, shortID, text } from "@/lib/format";

type WorkItem = Record<string, unknown>;

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" ? value as Record<string, unknown> : {};
}

function statusTone(status: unknown): StatusTone {
  if (status === "active" || status === "completed") return "active";
  if (status === "blocked") return "warning";
  if (status === "cancelled" || status === "abandoned") return "danger";
  return "neutral";
}

function scopes(item: WorkItem): Array<Record<string, unknown>> {
  return Array.isArray(item.scopes) ? item.scopes as Array<Record<string, unknown>> : [];
}

export function IntentTable({ items }: { items: WorkItem[] }) {
  const [selected, setSelected] = useState<WorkItem | null>(null);
  if (!items.length) return <EmptyState title="No hay trabajo declarado" description="Cuando una persona o agente cree un intent, aparecerá aquí." />;

  return (
    <>
      <DataTable aria-label="Trabajo activo">
        <DataTableHead><tr><DataTableHeaderCell>Actor</DataTableHeaderCell><DataTableHeaderCell>Objetivo</DataTableHeaderCell><DataTableHeaderCell>Scope / rama</DataTableHeaderCell><DataTableHeaderCell>Estado</DataTableHeaderCell><DataTableHeaderCell>Latido</DataTableHeaderCell><DataTableHeaderCell><span className="pact-visually-hidden">Acciones</span></DataTableHeaderCell></tr></DataTableHead>
        <DataTableBody>
          {items.map((item, index) => {
            const intent = record(item.intent);
            const workspace = record(item.workspace);
            const actorName = text(item.responsible_name || item.actor_name || record(item.actor).display_name || item.actor_id, "Actor desconocido");
            const status = text(intent.status || item.status, "unknown");
            const firstScope = record(scopes(item)[0]?.resource);
            return (
              <DataTableRow key={text(intent.id || item.id, String(index))} state={status === "blocked" ? "warning" : "default"}>
                <DataTableCell><span className="identity-cell"><Avatar name={actorName} kind={text(item.actor_kind).toLowerCase().includes("agent") ? "agent" : "person"} size="sm" /><span><strong>{actorName}</strong><small>{text(item.actor_kind || item.client_type, "Colaborador")}</small></span></span></DataTableCell>
                <DataTableCell><strong>{text(intent.title || item.objective, "Sin objetivo")}</strong><small className="table-detail">{text(intent.summary || intent.goal || item.summary, "Sin resumen")}</small></DataTableCell>
                <DataTableCell><code>{text(firstScope.locator || workspace.git_branch || item.branch)}</code>{scopes(item).length > 1 ? <small className="table-detail">+{scopes(item).length - 1} scopes</small> : null}</DataTableCell>
                <DataTableCell><StatusChip tone={statusTone(status)}>{status}</StatusChip></DataTableCell>
                <DataTableCell>{relativeDate(item.session_last_seen_at || item.heartbeat_at || item.last_seen_at)}</DataTableCell>
                <DataTableCell><Button size="sm" variant="ghost" onClick={() => setSelected(item)}>Detalle</Button></DataTableCell>
              </DataTableRow>
            );
          })}
        </DataTableBody>
      </DataTable>
      <IntentDetail item={selected} open={Boolean(selected)} onOpenChange={(open) => { if (!open) setSelected(null); }} />
    </>
  );
}

function IntentDetail({ item, open, onOpenChange }: { item: WorkItem | null; open: boolean; onOpenChange: (open: boolean) => void }) {
  const intent = record(item?.intent);
  const workspace = record(item?.workspace);
  const claims = item ? scopes(item) : [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader><p className="pact-kicker">INTENT · {shortID(intent.id)}</p><DialogTitle>{text(intent.title || item?.objective, "Detalle del intent")}</DialogTitle></DialogHeader>
        <DialogBody className="intent-detail">
          <dl className="fact-grid">
            <div><dt>Responsable</dt><dd>{text(item?.responsible_name || item?.actor_name)}</dd></div>
            <div><dt>Estado</dt><dd>{text(intent.status || item?.status)}</dd></div>
            <div><dt>Rama</dt><dd><code>{text(workspace.git_branch || item?.branch)}</code></dd></div>
            <div><dt>Base</dt><dd><code>{shortID(intent.base_revision || item?.base_revision, 16)}</code></dd></div>
          </dl>
          <section><h3>Resumen</h3><p>{text(intent.summary || intent.goal || item?.summary, "No hay resumen disponible.")}</p></section>
          <section><h3>Scopes reclamados</h3>{claims.length ? <ul className="scope-list">{claims.map((claim, index) => { const resource = record(claim.resource); return <li key={index}><code>{text(resource.locator)}</code><StatusChip tone={claim.mode === "shared" ? "info" : "neutral"}>{text(claim.mode, "exclusive")}</StatusChip></li>; })}</ul> : <p>No hay scopes declarados.</p>}</section>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
