import { describe, expect, it } from "vitest";
import { defaultEditEventForm, defaultEventForm } from "@/lib/formatting";
import {
  createBody,
  eventSiteOptions,
  initialTab,
  updateBody,
  windowReady,
} from "./admin-page-helpers";

describe("admin page helpers", () => {
  it("builds create payloads without UI-only fields", () => {
    const body = createBody({
      ...defaultEventForm(),
      title: "  Team Lunch  ",
      department: "",
      site: "",
      employment_status: "",
      capacity_type: "unlimited",
      capacity: "99",
      tags: "food, taipei, food",
    });

    expect(body).toEqual(
      expect.objectContaining({
        title: "Team Lunch",
        ends_at: expect.any(String),
        capacity: null,
        allows_family: true,
        tags: ["food", "taipei"],
        rule: {
          department: "*",
          site: "*",
          min_grade: 5,
          employment_status: "active",
        },
      }),
    );
    expect(body).not.toHaveProperty("_selectedTemplate");
    expect(body).not.toHaveProperty("_selectedSchedule");
  });

  it("builds update payloads for limited-capacity events", () => {
    const body = updateBody({
      ...defaultEditEventForm(),
      title: "  Town Hall  ",
      description: "  Quarterly update  ",
      capacity_type: "limited",
      capacity: "32",
      tags: "company, update",
    });

    expect(body).toEqual(
      expect.objectContaining({
        title: "Town Hall",
        description: "Quarterly update",
        ends_at: expect.any(String),
        capacity: 32,
        allows_family: false,
        tags: ["company", "update"],
      }),
    );
  });

  it("derives admin tab state from route path", () => {
    expect(initialTab("/admin/events/new")).toBe("create");
    expect(initialTab("/admin/events/edit")).toBe("edit");
    expect(initialTab("/admin/events/eligibility")).toBe("eligibility");
    expect(initialTab("/admin/events")).toBe("list");
  });

  it("filters wildcard site options for event location choices", () => {
    expect(
      eventSiteOptions([
        { value: "*", label: "不限" },
        { value: "Taipei HQ", label: "台北總部" },
      ]),
    ).toEqual([
      { value: "", label: "未設定" },
      { value: "Taipei HQ", label: "台北總部" },
    ]);
  });

  it("validates registration windows before publish", () => {
    expect(
      windowReady(
        "2026-06-10T10:00:00Z",
        "2026-06-10T12:00:00Z",
        "2026-06-01T10:00:00Z",
        "2026-06-05T10:00:00Z",
      ),
    ).toBe(true);
    expect(
      windowReady(
        "2026-06-10T10:00:00Z",
        "2026-06-10T12:00:00Z",
        "2026-06-06T10:00:00Z",
        "2026-06-05T10:00:00Z",
      ),
    ).toBe(false);
    expect(
      windowReady(
        "2026-06-10T10:00:00Z",
        "2026-06-10T10:00:00Z",
        "2026-06-01T10:00:00Z",
        "2026-06-05T10:00:00Z",
      ),
    ).toBe(false);
  });
});
