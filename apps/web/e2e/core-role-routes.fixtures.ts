function session<const Role extends string>(id: string, role: Role) {
  return {
    actor: { id, role },
    expires_at: "2099-12-31T23:59:59Z",
  };
}

export const sessions = {
  E1001: session("E1001", "employee"),
  "admin-1": session("admin-1", "activity_admin"),
  "staff-1": session("staff-1", "checkin_staff"),
  "hr-1": session("hr-1", "hr_admin"),
  "system-1": session("system-1", "system_admin"),
};

export type Session = (typeof sessions)[keyof typeof sessions];

export const sampleTickets = [
  {
    ticket_id: "ticket-001",
    registration_id: "reg-001",
    event_id: "evt-cets-001",
    employee_id: "E1001",
    status: "active",
    issued_at: "2026-01-02T09:00:00Z",
    event_title: "第一階段企業午餐日",
    event_location: "台北總部多功能廳",
    event_starts_at: "2026-01-10T10:00:00Z",
    employee_name: "陳雅莉",
    qr_payload: "mocked-qr-token",
    signed_token: "mocked-token",
  },
];

export const sampleEvent = {
  event_id: "evt-cets-001",
  title: "第一階段企業午餐日",
  description: "內部示範活動",
  location: "台北總部多功能廳",
  event_city: "Taipei",
  event_site: "Taipei",
  starts_at: "2026-01-10T10:00:00Z",
  registration_start: "2026-01-01T10:00:00Z",
  registration_close: "2026-01-09T23:00:00Z",
  capacity_type: "limited",
  capacity: 240,
  allows_family: false,
  status: "published",
  allocation_mode: "first_come_first_served",
  created_by: "admin-1",
  created_at: "2026-01-01T08:00:00Z",
  updated_at: "2026-01-01T08:00:00Z",
  rule: {
    department: "Engineering",
    site: "Taipei",
    min_grade: 5,
    employment_status: "active",
  },
  eligible: true,
  eligibility_reason: "符合資格",
  confirmed_count: 12,
  waitlist_count: 0,
  remaining_capacity: 228,
  current_user_status: "confirmed",
  current_user_ticket: sampleTickets[0],
};

export type EventFixture = Record<string, unknown> & { event_id: string };

export type RouteMockOptions = {
  bookingResponse?: unknown;
  cancelResponse?: unknown;
  eventDetail?: EventFixture;
  events?: EventFixture[];
  mockProfiles?: boolean;
};

export const reportRows = [
  {
    event_id: "evt-cets-001",
    title: "第一階段企業午餐日",
    capacity_type: "limited",
    capacity: 240,
    confirmed_count: 12,
    waitlist_count: 0,
    employee_count: 12,
    family_count: 0,
    total_attendee_count: 12,
    ticket_count: 12,
    checkin_count: 8,
    remaining_capacity: 228,
    city_distribution: { Taipei: 12 },
    starts_at: "2026-01-10T10:00:00Z",
  },
];

export const notificationDeliveries = [
  {
    delivery_id: "delivery-001",
    outbox_id: "outbox-001",
    employee_id: "E1001",
    channel: "in-app",
    status: "sent",
    attempts: 1,
    last_error: "",
    created_at: "2026-01-05T08:00:00Z",
    updated_at: "2026-01-05T08:01:00Z",
  },
];

export const auditRows = [
  {
    audit_id: "audit-001",
    actor_id: "admin-1",
    role: "activity_admin",
    action: "event.created",
    entity_type: "event",
    entity_id: "evt-cets-001",
    metadata: "source=ui",
    created_at: "2026-01-01T08:30:00Z",
  },
];

export const impactReviews = [
  {
    review_id: "review-001",
    event_id: "evt-cets-001",
    employee_id: "E1001",
    ticket_id: "ticket-001",
    status: "open",
    reason: "示範資料",
    created_at: "2026-01-03T10:00:00Z",
  },
];

type PrincipalID = keyof typeof sessions;
type RouteSeed = readonly [path: string, heading: string];

const roleCase = (
  name: string,
  principalID: PrincipalID,
  routes: readonly RouteSeed[],
) => ({
  name,
  principalID,
  routes: routes.map(([path, heading]) => ({ path, heading })),
});

const forbiddenRouteCase = (
  name: string,
  principalID: PrincipalID,
  path: string,
) => ({ name, principalID, path });

export const roleCases = [
  roleCase("employee", "E1001", [
    ["/user/events", "活動探索"],
    ["/user/events/evt-cets-001", "活動詳情"],
    ["/user/tickets", "我的票券"],
    ["/user/tickets?ticket_id=ticket-001", "我的票券"],
    ["/user/notifications", "通知中心"],
  ]),
  roleCase("activity_admin", "admin-1", [
    ["/admin/events", "活動設定"],
    ["/admin/registrations", "報名治理"],
    ["/admin/notifications", "通知投遞"],
  ]),
  roleCase("checkin_staff", "staff-1", [
    ["/admin/checkin", "現場驗票"],
    ["/admin/checkin/offline", "離線驗票同步"],
  ]),
  roleCase("hr_admin", "hr-1", [
    ["/admin/reports", "人資報表"],
    ["/admin/hr-settings", "人資同步設定"],
    ["/admin/audit", "稽核查詢"],
  ]),
  roleCase("system_admin", "system-1", [
    ["/admin/reports", "人資報表"],
    ["/admin/hr-settings", "人資同步設定"],
    ["/admin/audit", "稽核查詢"],
    ["/admin/notifications", "通知投遞"],
  ]),
];

export const forbiddenRouteCases = [
  forbiddenRouteCase(
    "employee cannot open admin events",
    "E1001",
    "/admin/events",
  ),
  forbiddenRouteCase(
    "activity admin cannot open check-in",
    "admin-1",
    "/admin/checkin",
  ),
  forbiddenRouteCase(
    "check-in staff cannot open reports",
    "staff-1",
    "/admin/reports",
  ),
  forbiddenRouteCase(
    "HR cannot open event operations",
    "hr-1",
    "/admin/events",
  ),
  forbiddenRouteCase(
    "system admin cannot open event operations",
    "system-1",
    "/admin/events",
  ),
] as const;

export function envelope<T>(
  data: T,
  status = true,
  error: string | null = null,
) {
  return {
    success: status,
    data,
    error,
  };
}

export function claimsFromSession(session: Session) {
  return {
    employee_id: session.actor.id,
    display_name: session.actor.id,
    role_claims: [session.actor.role],
    mapped_roles: [session.actor.role],
    department: "Engineering",
    site: "Taipei",
    city: "Taipei",
    grade: 6,
    employment_status: "active",
    claims_status: "complete",
  };
}

export function mockProfiles() {
  return Object.values(sessions).map((session) => ({
    profile_id: session.actor.id,
    display_name: session.actor.id,
    role_claims: [session.actor.role],
    mapped_roles: [session.actor.role],
    department:
      session.actor.role === "employee" ? "Engineering" : "Operations",
    site: "Taipei",
    city: "Taipei",
    grade: session.actor.role === "employee" ? 6 : 5,
    employment_status: "active",
  }));
}
