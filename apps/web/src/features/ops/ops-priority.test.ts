import { describe, expect, it } from "vitest";
import type { OpsDashboard } from "@/lib/api";
import {
  prioritizeOpsDashboard,
  sortCapacityPressure,
  sortDeadLetters,
  sortQueues,
  sortReportFreshness,
} from "./ops-priority";

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

  it("sorts recent replays newest first and tolerates missing lists", () => {
    const replay = (id: string, createdAt: string | null) => ({
      delivery_id: id,
      worker_kind: "notification",
      event_type: "notification.requested",
      status: "replayed",
      retry_count: 0,
      created_at: createdAt as string,
      retry_eligible: false,
      dead_letter_eligible: false,
    });

    const dashboard = prioritizeOpsDashboard({
      capacity_pressure: { events: [] },
      queues: { queues: [] },
      reports_freshness: { projections: [] },
      replay_recent: [
        replay("older", "2026-01-01T00:00:00Z"),
        replay("unparsable", null),
        replay("newer", "2026-01-02T00:00:00Z"),
      ],
    } satisfies OpsDashboard);

    expect(dashboard.replay_recent?.map((r) => r.delivery_id)).toEqual([
      "newer",
      "older",
      "unparsable",
    ]);
    expect(dashboard.dead_letter_recent).toEqual([]);
  });
});

describe("sortCapacityPressure", () => {
  it("treats unlimited capacity as lowest pressure and breaks ties by reservations", () => {
    const row = (
      id: string,
      remaining: number | null,
      reservations: number,
    ) => ({
      event_id: id,
      capacity_type: "limited",
      remaining_capacity: remaining,
      reservation_count: reservations,
      rate_limit_drop_per_min: 0,
      idempotency_replay_per_min: 0,
    });

    const sorted = sortCapacityPressure([
      row("unlimited", null, 50),
      row("busy", 2, 9),
      row("calm", 2, 3),
    ]);

    expect(sorted.map((r) => r.event_id)).toEqual([
      "busy",
      "calm",
      "unlimited",
    ]);
  });
});

describe("sortQueues", () => {
  it("breaks dead-letter ties by pending then p95 age", () => {
    const queue = (
      name: string,
      pending: number,
      deadLetter: number,
      p95: number,
    ) => ({
      name,
      pending,
      in_flight: 0,
      dead_letter: deadLetter,
      p95_age_seconds: p95,
    });

    const sorted = sortQueues([
      queue("aging", 1, 1, 120),
      queue("young", 1, 1, 5),
      queue("backlogged", 9, 1, 5),
    ]);

    expect(sorted.map((q) => q.name)).toEqual(["backlogged", "aging", "young"]);
  });
});

describe("sortReportFreshness", () => {
  it("orders equally degraded projections by lag", () => {
    const projection = (name: string, lag: number) => ({
      name,
      last_applied: "2026-01-01T00:00:00Z",
      lag_seconds: lag,
      degraded: true,
    });

    const sorted = sortReportFreshness([
      projection("behind", 30),
      projection("way-behind", 300),
    ]);

    expect(sorted.map((p) => p.name)).toEqual(["way-behind", "behind"]);
  });
});

describe("sortDeadLetters", () => {
  it("breaks retry-eligibility ties by retry count then recency", () => {
    const deadLetter = (id: string, retries: number, createdAt: string) => ({
      delivery_id: id,
      worker_kind: "notification",
      event_type: "notification.requested",
      status: "dead_letter",
      retry_count: retries,
      created_at: createdAt,
      retry_eligible: true,
      dead_letter_eligible: true,
    });

    const sorted = sortDeadLetters([
      deadLetter("old-once", 1, "2026-01-01T00:00:00Z"),
      deadLetter("new-once", 1, "2026-01-03T00:00:00Z"),
      deadLetter("thrice", 3, "2026-01-02T00:00:00Z"),
    ]);

    expect(sorted.map((d) => d.delivery_id)).toEqual([
      "thrice",
      "new-once",
      "old-once",
    ]);
  });
});
