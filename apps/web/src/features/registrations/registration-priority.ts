import type { RegistrationDetail } from "@/lib/api";

export function sortRegistrationRowsByAttention(rows: RegistrationDetail[]) {
  return [...rows].sort((left, right) => {
    const priorityDiff =
      registrationAttentionPriority(left) -
      registrationAttentionPriority(right);
    if (priorityDiff !== 0) return priorityDiff;
    return parseTime(right.created_at) - parseTime(left.created_at);
  });
}

export function registrationAttentionCount(rows: RegistrationDetail[]) {
  return rows.filter((row) => registrationAttentionPriority(row) < 4).length;
}

export function initialRegistrationEventID() {
  return new URLSearchParams(globalThis.location.search).get("event_id") ?? "";
}

function registrationAttentionPriority(row: RegistrationDetail) {
  if (row.status === "waitlisted") return 0;
  if (row.ticket?.status === "active") return 1;
  if (row.ticket?.status === "revoked") return 2;
  if (row.status === "cancelled") return 3;
  return 4;
}

function parseTime(value?: string) {
  const time = new Date(value ?? "").getTime();
  return Number.isNaN(time) ? 0 : time;
}
