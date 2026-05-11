import { describe, expect, it } from "vitest";
import { adminRoutes, canAccessRoute, currentRoute, defaultAdminRouteForRole, defaultRouteForRole, routeKeyForPath, routePath, userRoutes } from "./routes";

describe("route guards", () => {
  it("keeps employee routes separate from admin routes", () => {
    expect(userRoutes.map((route) => route.key)).toContain("user-events");
    expect(adminRoutes.map((route) => route.key)).toContain("admin-events");
    expect(canAccessRoute("user-events", "employee")).toBe(true);
    expect(canAccessRoute("admin-events", "employee")).toBe(false);
  });

  it("resolves default routes and paths per role", () => {
    expect(defaultRouteForRole("checkin_staff")).toBe("admin-checkin");
    expect(defaultAdminRouteForRole("hr_admin")).toBe("admin-events");
    expect(defaultAdminRouteForRole("system_admin")).toBe("admin-reports");
    expect(canAccessRoute("admin-audit", "system_admin")).toBe(true);
    expect(canAccessRoute("admin-events", "system_admin")).toBe(false);
    expect(canAccessRoute("api-docs", "checkin_staff")).toBe(true);
    expect(routePath("admin-demo")).toBe("/admin/demo");
    expect(routePath("api-docs")).toBe("/api-docs");
  });

  it("maps production deep links onto existing flat route keys", () => {
    expect(routeKeyForPath("/user/events/evt_1")).toBe("user-event-detail");
    expect(routeKeyForPath("/admin/events/new")).toBe("admin-events");
    expect(routeKeyForPath("/admin/events/evt_1/edit")).toBe("admin-events");
    expect(routeKeyForPath("/admin/events/evt_1/eligibility")).toBe("admin-events");
    expect(routeKeyForPath("/admin/events/evt_1/registrations")).toBe("admin-registrations");
    expect(routeKeyForPath("/admin/checkin/offline")).toBe("admin-offline-checkin");
    expect(routeKeyForPath("/admin/offline-checkin")).toBe("admin-offline-checkin");
    expect(routeKeyForPath("/admin/hr-sync")).toBe("admin-hr-settings");
    expect(routeKeyForPath("/admin/settings")).toBe("admin-hr-settings");
    expect(routeKeyForPath("/admin/api-docs")).toBe("api-docs");
  });

  it("uses canonical /admin/checkin/offline for offline check-in route path", () => {
    expect(routePath("admin-offline-checkin")).toBe("/admin/checkin/offline");
  });

  it("preserves the event id from dynamic deep links for existing pages", () => {
    window.history.replaceState({}, "", "/user/events/evt_1");

    expect(currentRoute()).toBe("user-event-detail");
    expect(new URLSearchParams(window.location.search).get("event_id")).toBe("evt_1");
  });
});
