import type { AuditLog, AuditLogFilters } from "@/lib/api";

const filterKeys = [
  "actor_id",
  "role",
  "action",
  "entity_type",
  "entity_id",
  "from",
  "to",
  "limit",
  "cursor",
] as const satisfies readonly (keyof AuditLogFilters)[];

export type AuditUrlState = {
  filters: AuditLogFilters;
  selectedID: string;
};

export function readAuditUrlState(): AuditUrlState {
  const params = new URLSearchParams(window.location.search);
  const filters: AuditLogFilters = { limit: params.get("limit") || "50" };
  for (const key of filterKeys) {
    const value = params.get(key);
    if (value) filters[key] = value;
  }
  return {
    filters,
    selectedID: params.get("audit_id") || "",
  };
}

export function replaceAuditUrl(
  filters: AuditLogFilters,
  preset: string,
  selectedID: string,
) {
  const params = new URLSearchParams();
  params.set("tab", preset);
  for (const key of filterKeys) {
    const value = filters[key];
    if (value && String(value).trim()) params.set(key, String(value).trim());
  }
  if (selectedID) params.set("audit_id", selectedID);
  const query = params.toString();
  window.history.replaceState(
    {},
    "",
    `${window.location.pathname}${query ? `?${query}` : ""}${window.location.hash}`,
  );
}

export function auditCursorFromRow(row: AuditLog) {
  return `${row.created_at}|${row.audit_id}`;
}
