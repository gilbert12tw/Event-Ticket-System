import type { ReportRow } from "@/lib/api";

const currentWindowMs = 24 * 60 * 60 * 1000;

export function sortReportsByAttention(
  rows: ReportRow[],
  now: Date = new Date(),
) {
  return [...rows].sort((left, right) => {
    const priorityDiff =
      reportAttentionPriority(left, now) - reportAttentionPriority(right, now);
    if (priorityDiff !== 0) return priorityDiff;
    return parseTime(right.starts_at) - parseTime(left.starts_at);
  });
}

function reportAttentionPriority(row: ReportRow, now: Date) {
  if (isCurrentReport(row, now)) return 0;
  if (row.waitlist_count > 0) return 1;
  if (
    row.confirmed_count > 0 &&
    row.checkin_count / row.confirmed_count < 0.5
  ) {
    return 2;
  }
  if (row.remaining_capacity === 0) return 3;
  return 4;
}

function isCurrentReport(row: Pick<ReportRow, "starts_at">, now: Date) {
  const startsAt = parseTime(row.starts_at);
  const nowTime = now.getTime();
  return startsAt <= nowTime && nowTime < startsAt + currentWindowMs;
}

function parseTime(value?: string) {
  const time = new Date(value ?? "").getTime();
  return Number.isNaN(time) ? 0 : time;
}
