import { describe, expect, it } from "vitest";
import {
  adminRoutes,
  canAccessRoute,
  currentRoute,
  defaultAdminRouteForRole,
  defaultRouteForRole,
  roleRouteAccess,
  routeAliases,
  routeKeyForPath,
  routePath,
  routes,
  ticketDetailPath,
  userRoutes,
} from "./routes";
import routeData from "./routes-data.json";

const expectedUserRouteKeys = [
  "user-events",
  "user-event-detail",
  "user-tickets",
  "user-notifications",
] as const;
const expectedAdminRouteKeys = [
  "admin-events",
  "admin-registrations",
  "admin-notifications",
  "admin-checkin",
  "admin-offline-checkin",
  "admin-reports",
  "admin-hr-settings",
  "admin-audit",
  "admin-demo",
] as const;
const expectedRouteKeys = new Set([
  ...expectedUserRouteKeys,
  ...expectedAdminRouteKeys,
]);
const iconNames = new Set([
  "activity",
  "audit",
  "bell",
  "calendar",
  "chart",
  "play",
  "scan",
  "send",
  "settings",
  "ticket",
  "users",
  "wifiOff",
]);
const allowedMobileExtraKeys = new Set(["mobileOverflow", "mobilePrimary"]);
const expectedRouteAliases = {
  "/": "user-events",
  "/employee/events": "user-events",
  "/employee/tickets": "user-tickets",
  "/admin/events/new": "admin-events",
  "/checkin": "admin-checkin",
  "/admin/offline-checkin": "admin-offline-checkin",
  "/admin/checkin/offline": "admin-offline-checkin",
  "/hr/reports": "admin-reports",
  "/admin/hr-sync": "admin-hr-settings",
  "/admin/settings": "admin-hr-settings",
  "/demo": "admin-demo",
  "/admin/demo": "admin-demo",
} as const;
const expectedRoleRouteAccess = {
  employee: [
    "user-events",
    "user-event-detail",
    "user-tickets",
    "user-notifications",
  ],
  activity_admin: [
    "admin-events",
    "admin-registrations",
    "admin-notifications",
    "admin-demo",
  ],
  checkin_staff: ["admin-checkin", "admin-offline-checkin"],
  hr_admin: ["admin-reports", "admin-hr-settings", "admin-audit"],
  system_admin: [
    "admin-reports",
    "admin-hr-settings",
    "admin-audit",
    "admin-notifications",
  ],
} as const;

describe("route guards", () => {
  it("keeps employee routes separate from admin routes", () => {
    expect(userRoutes.map((route) => route.key)).toContain("user-events");
    expect(adminRoutes.map((route) => route.key)).toContain("admin-events");
    expect(canAccessRoute("user-events", "employee")).toBe(true);
    expect(canAccessRoute("admin-events", "employee")).toBe(false);
  });

  it("keeps route data inside the known route contract", () => {
    expect(routeData.userRouteSeeds.map((seed) => seed[0])).toEqual(
      expectedUserRouteKeys,
    );
    expect(routeData.adminRouteSeeds.map((seed) => seed[0])).toEqual(
      expectedAdminRouteKeys,
    );
    expect(routes.map((route) => route.key)).toEqual([
      ...expectedUserRouteKeys,
      ...expectedAdminRouteKeys,
    ]);

    for (const seed of [
      ...routeData.userRouteSeeds,
      ...routeData.adminRouteSeeds,
    ]) {
      expect([8, 9]).toContain(seed.length);
      expect(expectedRouteKeys.has(seed[0])).toBe(true);
      expect(iconNames.has(seed[6])).toBe(true);
      expect(Array.isArray(seed[7])).toBe(true);
      for (const [key, value] of Object.entries(seed[8] ?? {})) {
        expect(allowedMobileExtraKeys.has(key)).toBe(true);
        expect(typeof value).toBe("boolean");
      }
    }
    expect(routeAliases).toEqual(expectedRouteAliases);
    expect(roleRouteAccess).toEqual(expectedRoleRouteAccess);
  });

  it("resolves default routes and paths per role", () => {
    expect(defaultRouteForRole("checkin_staff")).toBe("admin-checkin");
    expect(defaultAdminRouteForRole("hr_admin")).toBe("admin-reports");
    expect(defaultAdminRouteForRole("system_admin")).toBe("admin-reports");
    expect(canAccessRoute("admin-audit", "system_admin")).toBe(true);
    expect(canAccessRoute("admin-events", "system_admin")).toBe(false);
    expect(routePath("admin-demo")).toBe("/admin/flow-check");
  });

  it("keeps each role inside the expected route matrix", () => {
    expect(canAccessRoute("user-tickets", "employee")).toBe(true);
    expect(canAccessRoute("admin-events", "employee")).toBe(false);
    expect(canAccessRoute("admin-events", "activity_admin")).toBe(true);
    expect(canAccessRoute("admin-checkin", "activity_admin")).toBe(false);
    expect(canAccessRoute("admin-checkin", "checkin_staff")).toBe(true);
    expect(canAccessRoute("admin-reports", "checkin_staff")).toBe(false);
    expect(canAccessRoute("admin-reports", "hr_admin")).toBe(true);
    expect(canAccessRoute("admin-events", "hr_admin")).toBe(false);
    expect(canAccessRoute("admin-notifications", "system_admin")).toBe(true);
    expect(canAccessRoute("admin-events", "system_admin")).toBe(false);
  });

  it("maps production deep links onto existing flat route keys", () => {
    expect(routeKeyForPath("/user/events/evt_1")).toBe("user-event-detail");
    expect(routeKeyForPath("/admin/events/new")).toBe("admin-events");
    expect(routeKeyForPath("/admin/events/evt_1/edit")).toBe("admin-events");
    expect(routeKeyForPath("/admin/events/evt_1/eligibility")).toBe(
      "admin-events",
    );
    expect(routeKeyForPath("/admin/events/evt_1/registrations")).toBe(
      "admin-registrations",
    );
    expect(routeKeyForPath("/admin/checkin/offline")).toBe(
      "admin-offline-checkin",
    );
    expect(routeKeyForPath("/admin/offline-checkin")).toBe(
      "admin-offline-checkin",
    );
    expect(routeKeyForPath("/admin/hr-sync")).toBe("admin-hr-settings");
    expect(routeKeyForPath("/admin/settings")).toBe("admin-hr-settings");
  });

  it("uses canonical /admin/checkin/offline for offline check-in route path", () => {
    expect(routePath("admin-offline-checkin")).toBe("/admin/checkin/offline");
  });

  it("builds ticket detail query paths without adding a separate route key", () => {
    expect(ticketDetailPath("ticket/1")).toBe(
      "/user/tickets?ticket_id=ticket%2F1",
    );
    expect(routeKeyForPath("/user/tickets")).toBe("user-tickets");
  });

  it("preserves the event id from dynamic deep links for existing pages", () => {
    window.history.replaceState({}, "", "/user/events/evt_1");

    expect(currentRoute()).toBe("user-event-detail");
    expect(new URLSearchParams(window.location.search).get("event_id")).toBe(
      "evt_1",
    );
  });
});
