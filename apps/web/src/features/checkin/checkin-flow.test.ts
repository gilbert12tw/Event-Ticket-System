import { describe, expect, it } from "vitest";
import type { EventSummary } from "@/lib/api";
import {
  isCheckinReady,
  selectedCheckinEventID,
  shouldSubmitDetectedToken,
} from "./checkin-flow";

describe("check-in flow helpers", () => {
  it("selects the current event without requiring staff to choose one", () => {
    const selected = selectedCheckinEventID(
      [
        event({ event_id: "old", starts_at: "2026-05-01T10:00:00Z" }),
        event({ event_id: "live", starts_at: "2026-05-20T10:00:00Z" }),
      ],
      "",
      new Date("2026-05-20T12:00:00Z"),
    );

    expect(selected).toBe("live");
  });

  it("keeps manual fallback readiness explicit", () => {
    expect(
      isCheckinReady({
        token: " signed-token ",
        deviceID: "gate-1",
        eventID: "evt-1",
      }),
    ).toBe(true);
    expect(
      isCheckinReady({ token: "signed-token", deviceID: "", eventID: "evt-1" }),
    ).toBe(false);
  });

  it("debounces duplicate scanner reads inside the repeat window", () => {
    expect(
      shouldSubmitDetectedToken({
        detectedToken: "qr-token",
        lastScan: null,
        nowMs: 2000,
      }),
    ).toBe(true);
    expect(
      shouldSubmitDetectedToken({
        detectedToken: "qr-token",
        lastScan: { token: "qr-token", scannedAtMs: 1000 },
        nowMs: 2000,
      }),
    ).toBe(false);
    expect(
      shouldSubmitDetectedToken({
        detectedToken: "qr-token",
        lastScan: { token: "qr-token", scannedAtMs: 1000 },
        nowMs: 2600,
      }),
    ).toBe(true);
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
