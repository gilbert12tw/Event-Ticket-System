import { describe, expect, it } from "vitest";
import type { RegistrationDetail } from "@/lib/api";
import {
  initialRegistrationEventID,
  registrationAttentionCount,
  sortRegistrationRowsByAttention,
} from "./registration-priority";

describe("registration governance priority", () => {
  it("puts waitlist and ticket governance rows before passive rows", () => {
    const rows = sortRegistrationRowsByAttention([
      registration({ registration_id: "passive", status: "confirmed" }),
      registration({ registration_id: "cancelled", status: "cancelled" }),
      registration({
        registration_id: "ticket",
        status: "confirmed",
        ticket: {
          ticket_id: "tkt-1",
          registration_id: "ticket",
          event_id: "evt-1",
          employee_id: "E1001",
          status: "active",
          issued_at: "2026-01-01T00:00:00Z",
          non_transferable: true,
        },
      }),
      registration({ registration_id: "waitlist", status: "waitlisted" }),
    ]);

    expect(rows.map((row) => row.registration_id)).toEqual([
      "waitlist",
      "ticket",
      "cancelled",
      "passive",
    ]);
    expect(registrationAttentionCount(rows)).toBe(3);
  });

  it("reads the initial event id from direct governance links", () => {
    window.history.replaceState({}, "", "/admin/registrations?event_id=evt-42");

    expect(initialRegistrationEventID()).toBe("evt-42");
  });
});

function registration(
  overrides: Partial<RegistrationDetail> = {},
): RegistrationDetail {
  return {
    registration_id: "reg-1",
    event_id: "evt-1",
    employee_id: "E1001",
    status: "confirmed",
    idempotency_key: "book-1",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}
