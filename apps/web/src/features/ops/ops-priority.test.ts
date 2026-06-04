import { describe, expect, it } from "vitest";
import type { OpsDashboard } from "@/lib/api";
import { prioritizeOpsDashboard } from "./ops-priority";

describe("ops dashboard prioritization", () => {
  it("places actionable and degraded operational rows first", () => {
    const dashboard = prioritizeOpsDashboard({
      capacity_pressure: {
        events: [
          {
            event_id: "roomy",
            capacity_type: "limited",
            remaining_capacity: 20,
            reservation_count: 1,
            rate_limit_drop_per_min: 0,
            idempotency_replay_per_min: 0,
          },
          {
            event_id: "tight",
            capacity_type: "limited",
            remaining_capacity: 1,
            reservation_count: 12,
            rate_limit_drop_per_min: 0,
            idempotency_replay_per_min: 0,
          },
        ],
      },
      queues: {
        queues: [
          {
            name: "notification",
            pending: 1,
            in_flight: 0,
            dead_letter: 0,
            p95_age_seconds: 30,
          },
          {
            name: "projection",
            pending: 0,
            in_flight: 0,
            dead_letter: 2,
            p95_age_seconds: 10,
          },
        ],
      },
      reports_freshness: {
        projections: [
          {
            name: "fresh",
            last_applied: "2026-01-01T00:00:00Z",
            lag_seconds: 3,
            degraded: false,
          },
          {
            name: "stale",
            last_applied: "2026-01-01T00:00:00Z",
            lag_seconds: 90,
            degraded: true,
          },
        ],
      },
      dead_letter_recent: [
        {
          delivery_id: "blocked",
          worker_kind: "notification",
          event_type: "notification.requested",
          status: "dead_letter",
          retry_count: 1,
          created_at: "2026-01-01T00:00:00Z",
          retry_eligible: false,
          dead_letter_eligible: true,
        },
        {
          delivery_id: "retryable",
          worker_kind: "notification",
          event_type: "notification.requested",
          status: "dead_letter",
          retry_count: 1,
          created_at: "2026-01-01T00:01:00Z",
          retry_eligible: true,
          dead_letter_eligible: true,
        },
      ],
      replay_recent: [],
    } satisfies OpsDashboard);

    expect(dashboard.capacity_pressure.events[0]?.event_id).toBe("tight");
    expect(dashboard.queues.queues[0]?.name).toBe("projection");
    expect(dashboard.reports_freshness.projections[0]?.name).toBe("stale");
    expect(dashboard.dead_letter_recent?.[0]?.delivery_id).toBe("retryable");
  });
});
