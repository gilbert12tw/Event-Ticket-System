import type { CapacityType } from "./contracts";

export type WorkerKind =
  | "notification"
  | "projection"
  | "compensation"
  | "export"
  | "unknown";

export type CapacityPressureRow = {
  event_id: string;
  capacity_type: CapacityType;
  remaining_capacity: number | null;
  reservation_count: number;
  rate_limit_drop_per_min: number | null;
  idempotency_replay_per_min: number | null;
};

export type CapacityPressure = {
  events: CapacityPressureRow[];
};

export type QueueStatusRow = {
  name: WorkerKind;
  pending: number;
  in_flight: number;
  dead_letter: number;
  p95_age_seconds: number;
  last_processed_at?: string | null;
};

export type QueueStatus = {
  queues: QueueStatusRow[];
};

export type ReportFreshnessProjection = {
  name: string;
  last_applied: string | null;
  lag_seconds: number;
  degraded: boolean;
  rebuild_in_progress?: boolean;
  rebuild_started_at?: string | null;
};

export type ReportFreshness = {
  projections: ReportFreshnessProjection[];
};

export type NotificationDeliveryOpsRow = {
  delivery_id: string;
  worker_kind: WorkerKind;
  event_type: string;
  status: string;
  retry_count: number;
  last_error?: string | null;
  recipient_redacted?: string | null;
  created_at: string;
  last_attempt_at?: string | null;
  dead_letter_at?: string | null;
  retry_eligible: boolean;
  dead_letter_eligible: boolean;
};

export type OutboxReplayRecentRow = {
  audit_id: string;
  actor_role: string;
  kind: WorkerKind;
  dry_run: boolean;
  affected_count: number;
  enqueued_count?: number | null;
  created_at: string;
};

export type OpsDashboard = {
  capacity_pressure: CapacityPressure;
  queues: QueueStatus;
  reports_freshness: ReportFreshness;
  dead_letter_recent?: NotificationDeliveryOpsRow[];
  replay_recent?: OutboxReplayRecentRow[];
};
