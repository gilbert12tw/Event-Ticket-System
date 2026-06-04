import { describe, expect, it } from "vitest";
import type { Ticket } from "@/lib/api";
import {
  employeeTicketSurfaceState,
  groupEmployeeTicketsByDate,
  shouldShowCompanionCount,
  ticketCalendarExport,
} from "./employee-ticket-surface";

function ticket(overrides: Partial<Ticket> = {}): Ticket {
  return {
    employee_id: "E1001",
    event_id: "evt-secret-raw",
    event_location: "台北總部 12F Lounge",
    event_starts_at: "2026-06-04T13:42:00+08:00",
    event_title: "今天員工交流午茶",
    issued_at: "2026-06-01T10:00:00+08:00",
    non_transferable: true,
    registration_id: "reg-secret-raw",
    signed_token: "signed-secret-token",
    status: "active",
    ticket_id: "ticket-secret-raw",
    ...overrides,
  };
}

describe("employee ticket surface", () => {
  const now = new Date("2026-06-04T14:00:00+08:00");

  it("summarizes active entry tickets for QR-first display", () => {
    const state = employeeTicketSurfaceState(ticket(), now);

    expect(state.kind).toBe("entry-ready");
    expect(state.canShowQr).toBe(true);
    expect(state.canAddToCalendar).toBe(true);
    expect(state.copy).toBe("入口出示 QR code 即可。");
  });

  it("allows calendar export for future active tickets without showing QR", () => {
    const state = employeeTicketSurfaceState(
      ticket({ event_starts_at: "2026-06-06T13:42:00+08:00" }),
      now,
    );

    expect(state.kind).toBe("upcoming");
    expect(state.canShowQr).toBe(false);
    expect(state.canAddToCalendar).toBe(true);
  });

  it("groups tickets by event date with entry tickets before future and history", () => {
    const groups = groupEmployeeTicketsByDate(
      [
        ticket({
          event_starts_at: "2026-06-01T09:00:00+08:00",
          status: "redeemed",
          ticket_id: "redeemed",
        }),
        ticket({
          event_starts_at: "2026-06-06T09:00:00+08:00",
          ticket_id: "future",
        }),
        ticket({ ticket_id: "current" }),
      ],
      now,
    );

    expect(groups.map((group) => group.dateKey)).toEqual([
      "2026-06-04",
      "2026-06-06",
      "2026-06-01",
    ]);
    expect(groups.map((group) => group.tickets[0]?.ticket_id)).toEqual([
      "current",
      "future",
      "redeemed",
    ]);
  });

  it("hides companion count unless it changes entry planning", () => {
    expect(shouldShowCompanionCount(ticket({ family_count: 0 }))).toBe(false);
    expect(shouldShowCompanionCount(ticket({ family_count: 2 }))).toBe(true);
  });

  it("exports ticket calendar files without raw ticket or employee identifiers", () => {
    const artifact = ticketCalendarExport(ticket());

    expect(artifact.content).toContain("BEGIN:VCALENDAR\r\n");
    expect(artifact.content).toContain("SUMMARY:今天員工交流午茶\r\n");
    expect(artifact.content).not.toContain("evt-secret-raw");
    expect(artifact.content).not.toContain("ticket-secret-raw");
    expect(artifact.content).not.toContain("reg-secret-raw");
    expect(artifact.content).not.toContain("E1001");
    expect(artifact.content).not.toContain("signed-secret-token");
  });
});
