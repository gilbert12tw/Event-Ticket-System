import type { AuditLog } from "@/lib/api";

export function sortAuditRowsByAttention(rows: AuditLog[]) {
  return [...rows].sort((left, right) => {
    const priorityDiff =
      auditAttentionPriority(left) - auditAttentionPriority(right);
    if (priorityDiff !== 0) return priorityDiff;
    return parseTime(right.created_at) - parseTime(left.created_at);
  });
}

export function auditTargetPath(
  row: Pick<AuditLog, "entity_id" | "entity_type">,
) {
  const entityID = encodeURIComponent(row.entity_id);
  if (row.entity_type === "event") return `/admin/events/${entityID}/edit`;
  if (row.entity_type === "registration") return "/admin/registrations";
  if (row.entity_type === "ticket" || row.entity_type === "checkin") {
    return "/admin/checkin";
  }
  return "";
}

function auditAttentionPriority(row: AuditLog) {
  if (row.action.includes("conflict")) return 0;
  if (/(failed|rejected|revoked|cancelled)/i.test(row.action)) return 1;
  if (row.entity_type === "ticket" || row.entity_type === "checkin") return 2;
  return 3;
}

function parseTime(value?: string) {
  const time = new Date(value ?? "").getTime();
  return Number.isNaN(time) ? 0 : time;
}
