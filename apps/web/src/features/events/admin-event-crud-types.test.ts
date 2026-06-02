import { afterEach, describe, expect, it, vi } from "vitest";
import type { EmployeeProfile, EventSummary } from "@/lib/api";
import { defaultEventForm } from "@/lib/formatting";
import {
  applyEventTemplate,
  applySchedulePreset,
  matchingEmployeesForEvent,
} from "./admin-event-crud-types";

function employee(overrides: Partial<EmployeeProfile> = {}): EmployeeProfile {
  return {
    employee_id: "E1001",
    full_name: "Ariel Chen",
    department: "Engineering",
    site: "Taipei",
    city: "Taipei",
    job_grade: 6,
    employment_status: "active",
    ...overrides,
  };
}

function event(overrides: Partial<EventSummary> = {}): EventSummary {
  return {
    event_id: "evt-1",
    title: "活動",
    description: "公司活動",
    location: "Taipei HQ",
    event_city: "Taipei",
    event_site: "Taipei",
    starts_at: "2026-06-10T10:00:00Z",
    registration_start: "2026-06-01T10:00:00Z",
    registration_close: "2026-06-05T10:00:00Z",
    capacity_type: "limited",
    capacity: 10,
    allows_family: false,
    allocation_mode: "fcfs",
    status: "published",
    created_by: "admin-1",
    created_at: "2026-05-31T08:00:00Z",
    updated_at: "2026-05-31T08:00:00Z",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    },
    confirmed_count: 0,
    waitlist_count: 0,
    remaining_capacity: 10,
    current_user_status: "",
    no_show_cooldown: { active: false },
    ...overrides,
  };
}

function expectedLocalInput(systemTime: string, hours: number) {
  const date = new Date(new Date(systemTime).getTime() + hours * 60 * 60_000);
  date.setMinutes(date.getMinutes() - date.getTimezoneOffset());
  return date.toISOString().slice(0, 16);
}

describe("admin event CRUD helpers", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("applies event templates without dropping unrelated form fields", () => {
    const base = { ...defaultEventForm(), entry_method: "manual" };

    expect(applyEventTemplate(base, "")).toEqual(
      expect.objectContaining({
        _selectedTemplate: "",
        entry_method: "manual",
      }),
    );
    expect(applyEventTemplate(base, "family")).toEqual(
      expect.objectContaining({
        title: "台北家庭電影夜",
        capacity_type: "unlimited",
        capacity: "",
        department: "*",
        entry_method: "manual",
        _selectedTemplate: "family",
      }),
    );
    expect(applyEventTemplate(base, "company")).toEqual(
      expect.objectContaining({
        title: "全公司交流活動",
        status: "draft",
        site: "*",
        _selectedTemplate: "company",
      }),
    );
  });

  it("applies schedule presets and tracks the selected schedule", () => {
    vi.useFakeTimers();
    const systemTime = "2026-05-31T08:00:00Z";
    vi.setSystemTime(new Date(systemTime));
    const base = defaultEventForm();

    expect(base._selectedSchedule).toBe("custom");
    expect(applySchedulePreset(base, "custom")).toEqual(
      expect.objectContaining({
        registration_start: base.registration_start,
        registration_close: base.registration_close,
        starts_at: base.starts_at,
        _selectedSchedule: "custom",
      }),
    );
    expect(applySchedulePreset(base, "open-now")).toEqual(
      expect.objectContaining({
        registration_start: expectedLocalInput(systemTime, 0),
        registration_close: expectedLocalInput(systemTime, 48),
        starts_at: expectedLocalInput(systemTime, 72),
        _selectedSchedule: "open-now",
      }),
    );
    expect(applySchedulePreset(base, "one-week")).toEqual(
      expect.objectContaining({
        registration_start: expectedLocalInput(systemTime, 0),
        registration_close: expectedLocalInput(systemTime, 24 * 7),
        starts_at: expectedLocalInput(systemTime, 24 * 10),
        _selectedSchedule: "one-week",
      }),
    );
    expect(applySchedulePreset(base, "next-week")).toEqual(
      expect.objectContaining({
        registration_start: expectedLocalInput(systemTime, 24 * 7),
        registration_close: expectedLocalInput(systemTime, 24 * 11),
        starts_at: expectedLocalInput(systemTime, 24 * 14),
        _selectedSchedule: "next-week",
      }),
    );
  });

  it("matches employees against the selected event rule only", () => {
    const rows = [
      employee({ employee_id: "E1001" }),
      employee({ employee_id: "E2002", department: "Finance" }),
      employee({ employee_id: "E3003", job_grade: 4 }),
      employee({ employee_id: "E4004", employment_status: "contractor" }),
    ];

    expect(matchingEmployeesForEvent(rows, undefined)).toEqual([]);
    expect(matchingEmployeesForEvent(rows, event())).toEqual([rows[0]]);
    expect(
      matchingEmployeesForEvent(
        rows,
        event({
          rule: {
            department: "*",
            site: "Taipei",
            min_grade: 4,
            employment_status: "*",
          },
        }),
      ).map((row) => row.employee_id),
    ).toEqual(["E1001", "E2002", "E3003", "E4004"]);
  });
});
