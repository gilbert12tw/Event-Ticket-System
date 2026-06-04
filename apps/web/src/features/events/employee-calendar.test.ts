import { describe, expect, it } from "vitest";
import type { Ticket } from "@/lib/api";
import { eventFixture } from "@/test/event-fixtures";
import {
  employeeEventDisplayState,
  groupEventsByCalendarDay,
  groupEventsByMonth,
  localDateKey,
  selectEmployeeAgenda,
  shouldShowCapacityHint,
} from "./employee-calendar";
import {
  calendarRangeForView,
  groupEmployeeCalendarEventsByDay,
  parseEmployeeCalendarQuery,
  registrationDeadlineView,
  selectEmployeeCalendarEvents,
  shiftCalendarDate,
} from "./employee-calendar-planner";

const now = new Date("2026-06-04T10:00:00+08:00");

function event(overrides: Parameters<typeof eventFixture>[0] = {}) {
  return eventFixture({
    registration_start: "2026-05-01T00:00:00+08:00",
    registration_close: "2026-06-30T23:59:00+08:00",
    starts_at: "2026-06-04T14:00:00+08:00",
    ...overrides,
  });
}

function ticket(eventID: string, status = "active"): Ticket {
  return {
    employee_id: "E1001",
    event_id: eventID,
    issued_at: "2026-06-01T10:00:00+08:00",
    registration_id: `reg-${eventID}`,
    status,
    ticket_id: `ticket-${eventID}`,
  };
}

describe("employee calendar helpers", () => {
  it("groups the current work week and marks days with relevant events", () => {
    const days = groupEventsByCalendarDay(
      [
        event({ event_id: "monday", starts_at: "2026-06-01T10:00:00+08:00" }),
        event({
          current_user_status: "confirmed",
          event_id: "today",
          starts_at: "2026-06-04T14:00:00+08:00",
        }),
        event({
          eligible: false,
          event_id: "hidden",
          starts_at: "2026-06-04T16:00:00+08:00",
        }),
      ],
      now,
    );

    expect(days.map((day) => day.dateKey)).toEqual([
      "2026-06-01",
      "2026-06-02",
      "2026-06-03",
      "2026-06-04",
      "2026-06-05",
      "2026-06-06",
      "2026-06-07",
    ]);
    expect(days[3]).toMatchObject({
      dateKey: "2026-06-04",
      eventCount: 1,
      hasRegistration: true,
      isToday: true,
    });
  });

  it("selects agenda items for one date and prioritizes registered events", () => {
    const rows = selectEmployeeAgenda(
      [
        event({ event_id: "bookable", title: "Bookable" }),
        event({
          current_user_status: "waitlisted",
          event_id: "waitlisted",
          starts_at: "2026-06-04T18:00:00+08:00",
          title: "Waitlisted",
        }),
        event({
          event_id: "tomorrow",
          starts_at: "2026-06-05T10:00:00+08:00",
          title: "Tomorrow",
        }),
        event({ eligible: false, event_id: "hidden", title: "Hidden" }),
      ],
      "2026-06-04",
      [],
      now,
    );

    expect(rows.map((row) => row.event_id)).toEqual(["waitlisted", "bookable"]);
  });

  it("builds a month grid without counting hidden events", () => {
    const days = groupEventsByMonth(
      [
        event({ event_id: "visible", starts_at: "2026-06-18T10:00:00+08:00" }),
        event({
          eligible: false,
          event_id: "hidden",
          starts_at: "2026-06-18T12:00:00+08:00",
        }),
      ],
      now,
      now,
    );

    expect(days).toHaveLength(42);
    expect(days[0].dateKey).toBe("2026-06-01");
    expect(days.find((day) => day.dateKey === "2026-06-18")).toMatchObject({
      eventCount: 1,
    });
  });

  it("summarizes employee-facing event states without exposing internals", () => {
    expect(
      employeeEventDisplayState(
        event({ event_id: "limited", remaining_capacity: 3 }),
        undefined,
        now,
      ),
    ).toMatchObject({
      kind: "bookable",
      label: "剩 3 席",
      primaryLabel: "報名活動",
      showInMain: true,
    });
    expect(
      employeeEventDisplayState(
        event({
          current_user_ticket: ticket("ticketed"),
          event_id: "ticketed",
          starts_at: "2026-06-04T08:00:00+08:00",
        }),
        undefined,
        now,
      ),
    ).toMatchObject({ kind: "entry-ready", label: "可入場" });
    expect(
      employeeEventDisplayState(event({ eligible: false }), undefined, now),
    ).toMatchObject({ showInMain: false, label: "目前不能報名" });
  });

  it("only shows scarce capacity hints for limited events", () => {
    expect(
      shouldShowCapacityHint(
        event({ capacity_type: "limited", remaining_capacity: 5 }),
      ),
    ).toBe(true);
    expect(
      shouldShowCapacityHint(
        event({ capacity_type: "limited", remaining_capacity: 8 }),
      ),
    ).toBe(false);
    expect(
      shouldShowCapacityHint(
        event({ capacity_type: "unlimited", remaining_capacity: null }),
      ),
    ).toBe(false);
  });

  it("formats local calendar keys consistently", () => {
    expect(localDateKey(new Date("2026-06-04T01:30:00+08:00"))).toBe(
      "2026-06-04",
    );
  });

  it("builds day, week, and month ranges with Monday week starts", () => {
    const week = calendarRangeForView("week", now, now);
    const day = calendarRangeForView("day", now, now);
    const month = calendarRangeForView("month", now, now);

    expect(week.days.map((day) => day.dateKey)).toEqual([
      "2026-06-01",
      "2026-06-02",
      "2026-06-03",
      "2026-06-04",
      "2026-06-05",
      "2026-06-06",
      "2026-06-07",
    ]);
    expect(week.label).toBe("06/01 - 06/07");
    expect(week.days.find((day) => day.isToday)?.dateKey).toBe("2026-06-04");
    expect(day.days.map((row) => row.dateKey)).toEqual(["2026-06-04"]);
    expect(month.days).toHaveLength(42);
    expect(month.label).toBe("2026年6月");
  });

  it("parses calendar query with safe defaults", () => {
    expect(
      parseEmployeeCalendarQuery("?view=month&date=2026-06-12", now),
    ).toMatchObject({
      dateKey: "2026-06-12",
      view: "month",
    });
    expect(
      parseEmployeeCalendarQuery("?view=year&date=2026-99-12", now),
    ).toMatchObject({
      dateKey: "2026-06-04",
      view: "week",
    });
  });

  it("shifts calendar anchors by the active view", () => {
    expect(localDateKey(shiftCalendarDate("day", now, 1))).toBe("2026-06-05");
    expect(localDateKey(shiftCalendarDate("week", now, -1))).toBe("2026-05-28");
    expect(localDateKey(shiftCalendarDate("month", now, 1))).toBe("2026-07-01");
  });

  it("summarizes registration deadlines for employee decisions", () => {
    expect(
      registrationDeadlineView(
        event({
          registration_start: "2026-06-05T09:00:00+08:00",
          registration_close: "2026-06-10T18:00:00+08:00",
        }),
        now,
      ),
    ).toMatchObject({
      detail: "06/05 開放",
      kind: "pending",
      label: "尚未開放",
    });
    expect(
      registrationDeadlineView(
        event({ registration_close: "2026-06-04T18:00:00+08:00" }),
        now,
      ),
    ).toMatchObject({ kind: "today", label: "今天截止", tone: "warn" });
    expect(
      registrationDeadlineView(
        event({ registration_close: "2026-06-05T18:00:00+08:00" }),
        now,
      ),
    ).toMatchObject({ kind: "tomorrow", label: "明天截止" });
    expect(
      registrationDeadlineView(
        event({ registration_close: "2026-06-08T18:00:00+08:00" }),
        now,
      ),
    ).toMatchObject({ kind: "soon", label: "報名剩 4 天" });
    expect(
      registrationDeadlineView(
        event({ registration_close: "2026-06-20T18:00:00+08:00" }),
        now,
      ),
    ).toMatchObject({ kind: "open", label: "報名至 06/20" });
    expect(
      registrationDeadlineView(
        event({ registration_close: "2026-06-03T18:00:00+08:00" }),
        now,
      ),
    ).toMatchObject({ kind: "closed", label: "已截止" });
  });

  it("selects visible calendar events and groups them by rendered days", () => {
    const range = calendarRangeForView("week", now, now);
    const rows = selectEmployeeCalendarEvents(
      [
        event({ event_id: "bookable", title: "Bookable" }),
        event({
          current_user_status: "confirmed",
          event_id: "registered",
          title: "Registered",
        }),
        event({
          eligible: false,
          event_id: "hidden",
          title: "Hidden",
        }),
        event({
          event_id: "next-week",
          starts_at: "2026-06-08T10:00:00+08:00",
        }),
      ],
      [],
      range,
      now,
    );
    const groups = groupEmployeeCalendarEventsByDay(rows, range);

    expect(rows.map(({ event }) => event.event_id)).toEqual([
      "registered",
      "bookable",
    ]);
    expect(
      groups.find((group) => group.dateKey === "2026-06-04"),
    ).toMatchObject({
      eventCount: 2,
      hasRegistration: true,
    });
  });
});
