import type {
  CapacityPressureRow,
  NotificationDeliveryOpsRow,
  OpsDashboard,
  QueueStatusRow,
  ReportFreshnessProjection,
} from "@/lib/api";

export function prioritizeOpsDashboard(dashboard: OpsDashboard): OpsDashboard {
  return {
    ...dashboard,
    capacity_pressure: {
      events: sortCapacityPressure(dashboard.capacity_pressure.events),
    },
    queues: {
      queues: sortQueues(dashboard.queues.queues),
    },
    reports_freshness: {
      projections: sortReportFreshness(dashboard.reports_freshness.projections),
    },
    dead_letter_recent: sortDeadLetters(dashboard.dead_letter_recent ?? []),
    replay_recent: [...(dashboard.replay_recent ?? [])].sort(
      (left, right) => parseTime(right.created_at) - parseTime(left.created_at),
    ),
  };
}

export function sortCapacityPressure(rows: CapacityPressureRow[]) {
  return [...rows].sort((left, right) => {
    const leftRemaining = left.remaining_capacity ?? Number.MAX_SAFE_INTEGER;
    const rightRemaining = right.remaining_capacity ?? Number.MAX_SAFE_INTEGER;
    if (leftRemaining !== rightRemaining) return leftRemaining - rightRemaining;
    return right.reservation_count - left.reservation_count;
  });
}

export function sortQueues(rows: QueueStatusRow[]) {
  return [...rows].sort((left, right) => {
    const deadDiff = right.dead_letter - left.dead_letter;
    if (deadDiff !== 0) return deadDiff;
    const pendingDiff = right.pending - left.pending;
    if (pendingDiff !== 0) return pendingDiff;
    return right.p95_age_seconds - left.p95_age_seconds;
  });
}

export function sortReportFreshness(rows: ReportFreshnessProjection[]) {
  return [...rows].sort((left, right) => {
    if (left.degraded !== right.degraded) return left.degraded ? -1 : 1;
    return right.lag_seconds - left.lag_seconds;
  });
}

export function sortDeadLetters(rows: NotificationDeliveryOpsRow[]) {
  return [...rows].sort((left, right) => {
    if (left.retry_eligible !== right.retry_eligible) {
      return left.retry_eligible ? -1 : 1;
    }
    const retryDiff = right.retry_count - left.retry_count;
    if (retryDiff !== 0) return retryDiff;
    return parseTime(right.created_at) - parseTime(left.created_at);
  });
}

function parseTime(value?: string | null) {
  const time = new Date(value ?? "").getTime();
  return Number.isNaN(time) ? 0 : time;
}
