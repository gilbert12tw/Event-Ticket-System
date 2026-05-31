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
  const params = new URLSearchParams(globalThis.location.search);
  const filters: AuditLogFilters = { limit: params.get("limit") ?? "50" };
  for (const key of filterKeys) {
    const value = params.get(key);
    if (value?.trim()) {
      filters[key] = value.trim();
    }
  }
  return {
    filters,
    selectedID: params.get("audit_id") ?? "",
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
    if (typeof value === "string" && value.trim())
      params.set(key, value.trim());
  }
  if (selectedID) params.set("audit_id", selectedID);
  const query = params.toString();
  const querySuffix = query ? `?${query}` : "";
  globalThis.history.replaceState(
    {},
    "",
    `${globalThis.location.pathname}${querySuffix}${globalThis.location.hash}`,
  );
}

export function auditCursorFromRow(row: AuditLog) {
  return `${row.created_at}|${row.audit_id}`;
}
