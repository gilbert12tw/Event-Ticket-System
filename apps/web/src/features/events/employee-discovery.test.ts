import { describe, expect, it } from "vitest";
import type { EventSummary, Ticket } from "@/lib/api";
import { eventFixture } from "@/test/event-fixtures";
import {
  defaultEmployeeDiscoveryState,
  employeeDiscoveryCityOptions,
  employeeEventsPath,
  filterEmployeeDiscoveryEvents,
  parseEmployeeDiscoveryQuery,
} from "./employee-discovery";

const now = new Date("2026-06-04T10:00:00+08:00");

function event(overrides: Partial<EventSummary>): EventSummary {
  return eventFixture({
    registration_close: "2026-06-10T12:00:00+08:00",
    registration_start: "2026-06-01T09:00:00+08:00",
    starts_at: "2026-06-04T13:00:00+08:00",
    ...overrides,
  });
}

function ticket(eventID: string): Ticket {
  return {
    employee_id: "E1001",
    event_id: eventID,
    event_location: "Taipei HQ",
    event_starts_at: "2026-06-04T13:00:00+08:00",
    event_title: "已報名活動",
    expires_at: "2026-06-04T18:00:00+08:00",
    issued_at: "2026-06-01T10:00:00+08:00",
    non_transferable: true,
    registration_id: "R-registered",
    signed_token: "signed-token",
    status: "active",
    ticket_id: "T-registered",
  };
}

function ids(events: EventSummary[]) {
  return events.map((item) => item.event_id);
}

describe("employee discovery filters", () => {
  it("reads discovery filters from URL and writes them without dropping view/date", () => {
    const discovery = parseEmployeeDiscoveryQuery(
      "?view=month&date=2026-06-04&q=%20yoga%20&capacity=limited&city=Hsinchu&status=registered",
    );

    expect(discovery).toEqual({
      q: "yoga",
      capacity: "limited",
      city: "Hsinchu",
      status: "registered",
    });
    expect(employeeEventsPath("month", "2026-06-04", discovery)).toBe(
      "/user/events?view=month&date=2026-06-04&q=yoga&capacity=limited&city=Hsinchu&status=registered",
    );
    expect(
      employeeEventsPath("week", "2026-06-04", defaultEmployeeDiscoveryState),
    ).toBe("/user/events?view=week&date=2026-06-04");
  });

  it("matches keyword search across title, description, location, city/site, category, and tags", () => {
    const events = [
      event({ event_id: "title", title: "Yoga Lab" }),
      event({ description: "Leadership circle", event_id: "description" }),
      event({ event_id: "location", location: "Auditorium A" }),
      event({ event_city: "Hsinchu", event_id: "city" }),
      event({ event_id: "site", event_site: "Nanda Campus" }),
      event({ category: "Wellbeing", event_id: "category" }),
      event({ event_id: "tag", tags: ["family-day"] }),
    ];

    const search = (q: string) =>
      ids(
        filterEmployeeDiscoveryEvents(
          events,
          [],
          { ...defaultEmployeeDiscoveryState, q },
          now,
        ),
      );

    expect(search("yoga")).toEqual(["title"]);
    expect(search("leadership")).toEqual(["description"]);
    expect(search("auditorium")).toEqual(["location"]);
    expect(search("hsinchu")).toEqual(["city"]);
    expect(search("nanda")).toEqual(["site"]);
    expect(search("wellbeing")).toEqual(["category"]);
    expect(search("family-day")).toEqual(["tag"]);
  });

  it("filters capacity, city, and employee-facing booking status", () => {
    const events = [
      event({ event_id: "limited", event_city: "Taipei" }),
      event({
        capacity: null,
        capacity_type: "unlimited",
        event_city: "Taipei",
        event_id: "unlimited",
        remaining_capacity: null,
      }),
      event({
        current_user_status: "confirmed",
        event_city: "Hsinchu",
        event_id: "registered",
      }),
      event({
        current_user_status: "waitlisted",
        event_city: "Hsinchu",
        event_id: "waitlisted",
      }),
      event({
        event_city: "Hsinchu",
        event_id: "waitlist-available",
        remaining_capacity: 0,
      }),
    ];

    expect(
      ids(
        filterEmployeeDiscoveryEvents(
          events,
          [],
          { ...defaultEmployeeDiscoveryState, capacity: "unlimited" },
          now,
        ),
      ),
    ).toEqual(["unlimited"]);
    expect(
      ids(
        filterEmployeeDiscoveryEvents(
          events,
          [],
          { ...defaultEmployeeDiscoveryState, city: "Taipei" },
          now,
        ),
      ),
    ).toEqual(["limited", "unlimited"]);
    expect(
      ids(
        filterEmployeeDiscoveryEvents(
          events,
          [ticket("registered")],
          { ...defaultEmployeeDiscoveryState, status: "registered" },
          now,
        ),
      ),
    ).toEqual(["registered"]);
    expect(
      ids(
        filterEmployeeDiscoveryEvents(
          events,
          [],
          { ...defaultEmployeeDiscoveryState, status: "waitlisted" },
          now,
        ),
      ),
    ).toEqual(["waitlisted", "waitlist-available"]);
  });

  it("treats an empty keyword as a reset and keeps selected city recoverable", () => {
    const events = [
      event({ event_city: "Taipei", event_id: "taipei" }),
      event({ event_city: "Hsinchu", event_id: "hsinchu" }),
    ];

    expect(
      ids(
        filterEmployeeDiscoveryEvents(
          events,
          [],
          { ...defaultEmployeeDiscoveryState, q: "   " },
          now,
        ),
      ),
    ).toEqual(["taipei", "hsinchu"]);
    expect(employeeDiscoveryCityOptions(events, "Tainan")).toContainEqual({
      value: "Tainan",
      label: "Tainan",
    });
  });
});
