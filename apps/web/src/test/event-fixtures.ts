import type { AuthMeClaims, EventSummary } from "@/lib/api";

export const claims: AuthMeClaims = {
  employee_id: "E1001",
  display_name: "Ariel Chen",
  role_claims: ["employee"],
  mapped_roles: ["employee"],
  department: "Engineering",
  site: "Taipei",
  city: "Taipei",
  grade: 6,
  employment_status: "active",
  claims_status: "complete",
};

export function eventFixture(
  overrides: Partial<EventSummary> = {},
): EventSummary {
  return {
    event_id: "evt-1",
    title: "活動",
    description: "公司活動",
    location: "Taipei HQ",
    event_city: "Taipei",
    event_site: "Taipei",
    starts_at: "2027-01-01T10:00:00Z",
    ends_at: "2027-01-01T12:00:00Z",
    registration_start: "2026-05-01T10:00:00Z",
    registration_close: "2026-12-31T10:00:00Z",
    capacity_type: "limited",
    capacity: 10,
    allows_family: false,
    status: "published",
    allocation_mode: "fcfs",
    created_by: "admin-1",
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    },
    eligible: true,
    eligibility_reason: "eligible",
    confirmed_count: 1,
    waitlist_count: 0,
    remaining_capacity: 9,
    current_user_status: "",
    no_show_cooldown: { active: false },
    ...overrides,
  };
}
