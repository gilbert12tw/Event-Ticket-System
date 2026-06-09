import { describe, expect, it, vi } from "vitest";
import type { Ticket } from "@/lib/api";
import {
  selectCurrentTicket,
  ticketEntryReadinessView,
} from "./ticket-readiness";

describe("ticket readiness", () => {
  it("uses the supplied clock to hide QR before the event starts", () => {
    const readiness = ticketEntryReadinessView(
      ticket({ event_starts_at: "2026-05-19T10:00:00Z" }),
      new Date("2026-05-19T09:59:59Z"),
    );

    expect(readiness.kind).toBe("not-open");
    expect(readiness.copy).toContain("活動尚未開始");
  });

  it("marks active tickets as expired after expires_at", () => {
    const readiness = ticketEntryReadinessView(
      ticket({ expires_at: "2026-05-20T10:00:00Z" }),
      new Date("2026-05-20T10:00:01Z"),
    );

    expect(readiness.kind).toBe("expired");
    expect(readiness.copy).toContain("有效期限");
  });

  it("does not let explicit expiry extend beyond the current event window", () => {
    const readiness = ticketEntryReadinessView(
      ticket({
        event_starts_at: "2026-05-20T10:00:00Z",
        expires_at: "2099-12-31T23:59:59Z",
      }),
      new Date("2026-05-21T10:00:01Z"),
    );

    expect(readiness.kind).toBe("expired");
  });

  it("keeps late-night tickets entry-ready after local midnight until expiry", () => {
    const readiness = ticketEntryReadinessView(
      ticket({
        event_starts_at: "2026-06-04T23:00:00+08:00",
        expires_at: "2026-06-05T23:00:00+08:00",
      }),
      new Date("2026-06-05T00:30:00+08:00"),
    );

    expect(readiness.kind).toBe("entry-ready");
  });

  it("selects the latest entry-ready ticket without exposing raw token text", () => {
    vi.setSystemTime(new Date("2026-05-20T12:00:00Z"));
    const selected = selectCurrentTicket([
      ticket({
        ticket_id: "future",
        event_starts_at: "2026-05-21T10:00:00Z",
      }),
      ticket({
        ticket_id: "older",
        event_starts_at: "2026-05-19T10:00:00Z",
      }),
      ticket({
        ticket_id: "current",
        event_starts_at: "2026-05-20T10:00:00Z",
      }),
      ticket({
        ticket_id: "expired",
        event_starts_at: "2026-05-20T10:00:00Z",
        expires_at: "2026-05-20T11:00:00Z",
      }),
    ]);

    expect(selected?.ticket_id).toBe("current");
    vi.useRealTimers();
  });
});

function ticket(overrides: Partial<Ticket> = {}): Ticket {
  return {
    ticket_id: "T-1",
    registration_id: "R-1",
    event_id: "EVT-1",
    employee_id: "E1001",
    status: "active",
    signed_token: "signed-token",
    issued_at: "2026-05-06T10:00:00Z",
    event_title: "台北家庭電影夜",
    event_location: "Taipei HQ",
    event_starts_at: "2026-05-20T10:00:00Z",
    expires_at: "2026-05-21T10:00:00Z",
    non_transferable: true,
    ...overrides,
  };
}
