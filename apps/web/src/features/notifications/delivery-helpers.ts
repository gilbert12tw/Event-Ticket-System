import type { NotificationDelivery } from "@/lib/api";

export function canRetryDelivery(status: string): boolean {
  return (
    status === "failed" || status === "dead_letter" || status === "pending"
  );
}

export function retryButtonLabel(row: NotificationDelivery, retrying: boolean) {
  if (retrying) return "重試中";
  if (!canRetryDelivery(row.status)) return "不可重試";
  return `重試第 ${row.attempts + 1} 次`;
}

export function retryDisabledReason(status: string) {
  if (status === "sent") return "已送達，不可重試";
  if (status === "suppressed") return "已抑制，不可重試";
  return "目前狀態不可重試";
}
