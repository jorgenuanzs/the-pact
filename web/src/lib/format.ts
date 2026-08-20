export const WORKSPACE_COLORS = [
  { value: "#c9ee4d", name: "Lima", key: "lime" },
  { value: "#2ab4e8", name: "Cian", key: "cyan" },
  { value: "#3c6ee0", name: "Cobalto", key: "cobalt" },
  { value: "#ee8a3c", name: "Naranja", key: "orange" },
  { value: "#d2683d", name: "Terracota", key: "terracotta" },
  { value: "#9b5fc0", name: "Malva", key: "mauve" },
] as const;

export function workspaceColorKey(color?: string): string {
  return WORKSPACE_COLORS.find((item) => item.value === color?.toLowerCase())?.key || "lime";
}

export function initials(value?: string): string {
  const parts = String(value || "?").trim().split(/\s+/).filter(Boolean);
  return parts.slice(0, 2).map((part) => part[0]?.toUpperCase()).join("") || "?";
}

export function slugify(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 63)
    .replace(/-+$/g, "");
}

export function shortID(value?: unknown, length = 8): string {
  const text = String(value ?? "");
  return text ? text.slice(0, length) : "—";
}

export function text(value?: unknown, fallback = "—"): string {
  if (value === null || value === undefined || value === "") return fallback;
  return String(value);
}

export function number(value?: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function formatDate(value?: unknown): string {
  if (!value) return "—";
  const date = new Date(String(value));
  if (Number.isNaN(date.getTime())) return text(value);
  return new Intl.DateTimeFormat("es", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

export function relativeDate(value?: unknown): string {
  if (!value) return "—";
  const timestamp = new Date(String(value)).getTime();
  if (!Number.isFinite(timestamp)) return text(value);
  const seconds = Math.round((timestamp - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat("es", { numeric: "auto" });
  if (Math.abs(seconds) < 60) return formatter.format(seconds, "second");
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return formatter.format(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, "hour");
  return formatter.format(Math.round(hours / 24), "day");
}

export function roleLabel(role?: string): string {
  return ({ owner: "Propietario", admin: "Administrador", member: "Miembro", observer: "Observador" } as Record<string, string>)[role || ""] || text(role);
}

export function canManage(role?: string): boolean {
  return role === "owner" || role === "admin";
}
