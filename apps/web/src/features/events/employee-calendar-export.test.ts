import { describe, expect, it } from "vitest";
import type { Ticket } from "@/lib/api";
import { eventFixture } from "@/test/event-fixtures";
import {
  canAddToCalendar,
  employeeCalendarExport,
} from "./employee-calendar-export";

function ticket(status = "active"): Ticket {
  return {
    employee_id: "E1001",
    event_id: "evt-secret-raw",
    issued_at: "2026-06-01T10:00:00+08:00",
    registration_id: "reg-secret-raw",
    signed_token: "signed-secret-token",
    status,
    ticket_id: "ticket-secret-raw",
  };
}

describe("employee calendar export", () => {
  it("builds a portable ICS event without raw internal identifiers", () => {
    const artifact = employeeCalendarExport(
      eventFixture({
        description: "下午茶, 分享;跨部門\\交流\n請準時到場",
        event_id: "evt-secret-raw",
        location: "台北總部 12F; Lounge",
        starts_at: "2026-06-04T13:42:00+08:00",
        title: "今天員工交流午茶",
      }),
      new Date("2026-06-01T00:00:00+08:00"),
    );

    expect(artifact.mimeType).toBe("text/calendar;charset=utf-8");
    expect(artifact.filename).toBe("2026-06-04-今天員工交流午茶.ics");
    expect(artifact.content).toContain("BEGIN:VCALENDAR\r\n");
    expect(artifact.content).toContain("VERSION:2.0\r\n");
    expect(artifact.content).toContain("BEGIN:VEVENT\r\n");
    expect(artifact.content).toContain("DTSTART:20260604T054200Z\r\n");
    expect(artifact.content).toContain("DTEND:20260604T074200Z\r\n");
    expect(artifact.content).toContain("DTSTAMP:20260531T160000Z\r\n");
    expect(artifact.content).toContain("SUMMARY:今天員工交流午茶\r\n");
    expect(artifact.content).toContain(
      "DESCRIPTION:下午茶\\, 分享\\;跨部門\\\\交流\\n請準時到場\r\n",
    );
    expect(artifact.content).toContain("LOCATION:台北總部 12F\\; Lounge\r\n");
    expect(artifact.content).not.toContain("evt-secret-raw");
    expect(artifact.content).not.toContain("E1001");
    expect(artifact.content).not.toContain("ticket-secret-raw");
    expect(artifact.content).not.toContain("reg-secret-raw");
    expect(artifact.content).not.toContain("signed-secret-token");
  });

  it("folds long ICS lines with CRLF continuation", () => {
    const artifact = employeeCalendarExport(
      eventFixture({
        event_id: "evt-long-title",
        title:
          "Quarterly engineering community lunch with platform architecture and product planning sharing session",
      }),
      new Date("2026-06-01T00:00:00Z"),
    );

    expect(artifact.content).toContain("\r\n ");
  });

  it("only allows confirmed or active-ticket events to be exported", () => {
    expect(
      canAddToCalendar(
        eventFixture({ current_user_status: "confirmed" }),
        undefined,
      ),
    ).toBe(true);
    expect(canAddToCalendar(eventFixture(), ticket("active"))).toBe(true);
    expect(
      canAddToCalendar(
        eventFixture({ current_user_status: "waitlisted" }),
        undefined,
      ),
    ).toBe(false);
    expect(canAddToCalendar(eventFixture(), ticket("revoked"))).toBe(false);
    expect(
      canAddToCalendar(
        eventFixture({ current_user_status: "cancelled" }),
        undefined,
      ),
    ).toBe(false);
  });
});
