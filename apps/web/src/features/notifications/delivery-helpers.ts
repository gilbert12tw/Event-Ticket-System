import type { NotificationDelivery } from "@/lib/api";

export function canRetryDelivery(row: NotificationDelivery): boolean {
  return (
    row.channel === "email" &&
    (row.status === "failed" || row.status === "dead_letter")
  );
}

export function retryButtonLabel(row: NotificationDelivery, retrying: boolean) {
  if (retrying) return "重試中";
  if (!canRetryDelivery(row)) return "不可重試";
  return `重試第 ${row.attempts + 1} 次`;
}

export function retryDisabledReason(row: NotificationDelivery) {
  if (row.channel !== "email") return "僅電子郵件投遞可重試";
  if (row.status === "sent") return "已送達，不可重試";
  if (row.status === "suppressed") return "已抑制，不可重試";
  if (row.status === "pending") return "待處理中，不可重試";
  return "目前狀態不可重試";
}

export function sortDeliveriesByAttention(rows: NotificationDelivery[]) {
  return [...rows].sort((left, right) => {
    const priorityDiff =
      deliveryAttentionPriority(left) - deliveryAttentionPriority(right);
    if (priorityDiff !== 0) return priorityDiff;
    return parseTime(right.updated_at) - parseTime(left.updated_at);
  });
}

function deliveryAttentionPriority(row: NotificationDelivery) {
  if (canRetryDelivery(row)) return 0;
  if (row.status === "failed" || row.status === "dead_letter") return 1;
  if (row.status === "pending") return 2;
  if (row.status === "suppressed") return 3;
  return 4;
}

function parseTime(value?: string) {
  const time = new Date(value ?? "").getTime();
  return Number.isNaN(time) ? 0 : time;
}
