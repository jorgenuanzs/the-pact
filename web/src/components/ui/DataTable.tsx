import type {
  HTMLAttributes,
  TableHTMLAttributes,
  TdHTMLAttributes,
  ThHTMLAttributes,
} from "react";

import { cx } from "./utils";

interface DataTableProps extends TableHTMLAttributes<HTMLTableElement> {
  containerClassName?: string;
}

export function DataTable({ className, containerClassName, ...props }: DataTableProps) {
  return (
    <div className={cx("pact-data-table-scroll", containerClassName)} tabIndex={0}>
      <table className={cx("pact-data-table", className)} {...props} />
    </div>
  );
}

export function DataTableHead(props: HTMLAttributes<HTMLTableSectionElement>) {
  return <thead {...props} />;
}

export function DataTableBody(props: HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody {...props} />;
}

export interface DataTableRowProps extends HTMLAttributes<HTMLTableRowElement> {
  state?: "default" | "warning" | "danger" | "muted";
}

export function DataTableRow({ state = "default", className, ...props }: DataTableRowProps) {
  return <tr className={cx("pact-data-table-row", className)} data-state={state} {...props} />;
}

export function DataTableHeaderCell(props: ThHTMLAttributes<HTMLTableCellElement>) {
  return <th scope="col" {...props} />;
}

export function DataTableCell(props: TdHTMLAttributes<HTMLTableCellElement>) {
  return <td {...props} />;
}
