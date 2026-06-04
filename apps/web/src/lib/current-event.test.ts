import { describe, expect, it } from "vitest";
import type { EventSummary } from "@/lib/api";
import {
  isCurrentEvent,
  selectCurrentEvent,
  selectCurrentEventID,
  sortEventsByManagementPriority,
} from "./current-event";

describe("current event selection", () => {
  const now = new Date("2026-05-20T12:00:00Z");

  it("treats the 24 hours after starts_at as the current event window", () => {
    expect(
      isCurrentEvent(event({ starts_at: "2026-05-20T10:00:00Z" }), now),
    ).toBe(true);
    expect(
      isCurrentEvent(event({ starts_at: "2026-05-21T10:00:00Z" }), now),
    ).toBe(false);
    expect(
      isCurrentEvent(event({ starts_at: "2026-05-19T10:00:00Z" }), now),
    ).toBe(false);
  });

  it("selects the most recently started current event", () => {
    const selected = selectCurrentEvent(
      [
        event({ event_id: "older", starts_at: "2026-05-20T09:00:00Z" }),
        event({ event_id: "latest", starts_at: "2026-05-20T11:00:00Z" }),
        event({ event_id: "future", starts_at: "2026-05-21T11:00:00Z" }),
      ],
      now,
    );

    expect(selected?.event_id).toBe("latest");
  });

  it("falls back to the existing selection or first event when none are current", () => {
    const rows = [
      event({ event_id: "past", starts_at: "2026-05-01T10:00:00Z" }),
      event({ event_id: "future", starts_at: "2026-06-01T10:00:00Z" }),
    ];

    expect(selectCurrentEventID(rows, "future", now)).toBe("future");
    expect(selectCurrentEventID(rows, "missing", now)).toBe("past");
  });

  it("sorts admin activity management by current and actionable events", () => {
    const rows = sortEventsByManagementPriority(
      [
        event({
          event_id: "archived",
          starts_at: "2026-06-01T10:00:00Z",
          status: "archived",
        }),
        event({
          event_id: "waitlist",
          starts_at: "2026-06-02T10:00:00Z",
          waitlist_count: 4,
        }),
        event({
          event_id: "current",
          starts_at: "2026-05-20T11:00:00Z",
        }),
        event({
          event_id: "draft",
          starts_at: "2026-06-03T10:00:00Z",
          status: "draft",
        }),
      ],
      now,
    );

    expect(rows.map((row) => row.event_id)).toEqual([
      "current",
      "waitlist",
      "draft",
      "archived",
    ]);
  });
});

function event(overrides: Partial<EventSummary> = {}): EventSummary {
  return {
    event_id: "evt-1",
    title: "Live Event",
    description: "",
    location: "Taipei HQ",
    starts_at: "2026-05-20T10:00:00Z",
    registration_start: "2026-05-01T10:00:00Z",
    registration_close: "2026-05-19T10:00:00Z",
    capacity_type: "limited",
    capacity: 40,
    allows_family: false,
    status: "published",
    allocation_mode: "fcfs",
    created_by: "admin-1",
    created_at: "2026-05-01T09:00:00Z",
    updated_at: "2026-05-01T09:00:00Z",
    rule: {
      department: "Engineering",
      site: "Taipei HQ",
      min_grade: 5,
      employment_status: "active",
    },
    confirmed_count: 1,
    waitlist_count: 0,
    remaining_capacity: 39,
    current_user_status: "",
    ...overrides,
  };
}
