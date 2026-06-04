import { describe, expect, it } from "vitest";
import type { ReportRow } from "@/lib/api";
import { sortReportsByAttention } from "./report-priority";

describe("report priority", () => {
  it("puts current, waitlist, and low-attendance reports first", () => {
    const rows = sortReportsByAttention(
      [
        report({ event_id: "ordinary", starts_at: "2026-06-01T00:00:00Z" }),
        report({
          event_id: "low-attendance",
          confirmed_count: 10,
          checkin_count: 1,
          starts_at: "2026-06-02T00:00:00Z",
        }),
        report({
          event_id: "waitlist",
          waitlist_count: 2,
          starts_at: "2026-06-03T00:00:00Z",
        }),
        report({ event_id: "current", starts_at: "2026-05-20T10:00:00Z" }),
      ],
      new Date("2026-05-20T12:00:00Z"),
    );

    expect(rows.map((row) => row.event_id)).toEqual([
      "current",
      "waitlist",
      "low-attendance",
      "ordinary",
    ]);
  });
});

function report(overrides: Partial<ReportRow> = {}): ReportRow {
  return {
    event_id: "evt-1",
    title: "活動",
    capacity_type: "limited",
    capacity: 10,
    confirmed_count: 2,
    waitlist_count: 0,
    employee_count: 2,
    family_count: 0,
    total_attendee_count: 2,
    ticket_count: 2,
    checkin_count: 2,
    remaining_capacity: 8,
    city_distribution: {},
    starts_at: "2026-06-01T00:00:00Z",
    ...overrides,
  };
}
