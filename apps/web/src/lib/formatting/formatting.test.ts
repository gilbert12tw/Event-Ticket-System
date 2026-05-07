import { describe, expect, it } from "vitest";
import { bookingActionLabel, eventStatusTone, normalizeAuditFilters, registrationTone } from ".";
import type { EventSummary } from "@/lib/api";

describe("formatting helpers", () => {
  it("normalizes audit filter date fields for API calls", () => {
    const filters = normalizeAuditFilters({
      actor_id: "hr-1",
      from: "2026-05-06T08:00",
      to: "2026-05-06T09:00",
      limit: ""
    });

    expect(filters.actor_id).toBe("hr-1");
    expect(filters.from).toContain("2026-05-06T");
    expect(filters.to).toContain("2026-05-06T");
    expect(filters.limit).toBe("50");
  });

  it("maps event, registration, and booking state to stable UI vocabulary", () => {
    expect(eventStatusTone("published")).toBe("ok");
    expect(registrationTone("waitlisted")).toBe("warn");
    expect(bookingActionLabel({ current_user_status: "", remaining_capacity: 0 } as EventSummary)).toBe("加入候補");
  });
});
