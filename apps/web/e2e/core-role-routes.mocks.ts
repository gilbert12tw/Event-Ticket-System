import { expect, type Page } from "@playwright/test";
import {
  auditRows,
  claimsFromSession,
  envelope,
  impactReviews,
  mockProfiles,
  notificationDeliveries,
  reportRows,
  type RouteMockOptions,
  sampleEvent,
  sampleTickets,
  sessions,
  type Session,
} from "./core-role-routes.fixtures";

export async function ensureSessionRoutes(
  page: Page,
  principalID: keyof typeof sessions,
  options: RouteMockOptions = {},
) {
  const session = { ...sessions[principalID] } as Session;
  let currentSession = session;
  const eventRows = options.events ?? [sampleEvent];
  const fallbackEventDetail =
    options.eventDetail ?? eventRows[0] ?? sampleEvent;

  if (options.mockProfiles) {
    await page.addInitScript((token) => {
      (
        globalThis as typeof globalThis & {
          __CETS_PROVIDER_TOKEN__?: string;
        }
      ).__CETS_PROVIDER_TOKEN__ = token;
    }, `mock-provider-${principalID}`);
  }

  await page.route("**/*", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const pathName = url.pathname;
    const method = request.method().toUpperCase();
    const isMockedPath =
      pathName.startsWith("/api/") ||
      pathName === "/healthz" ||
      pathName === "/readyz";
    if (!isMockedPath) {
      return route.continue();
    }

    let body: Record<string, unknown> = {};
    try {
      if (request.postData()) {
        body = JSON.parse(request.postData() || "{}") as Record<
          string,
          unknown
        >;
      }
    } catch {
      body = {};
    }

    const hasCallerEmployeeID =
      url.searchParams.has("employee_id") ||
      Object.prototype.hasOwnProperty.call(body, "employee_id");
    const ownDataRequest =
      (pathName === "/api/v1/events" && method === "GET") ||
      (/^\/api\/v1\/events\/[^/]+$/.test(pathName) && method === "GET") ||
      (/^\/api\/v1\/events\/[^/]+\/eligibility$/.test(pathName) &&
        method === "GET") ||
      (/^\/api\/v1\/events\/[^/]+\/bookings$/.test(pathName) &&
        method === "POST") ||
      (pathName === "/api/v1/me/tickets" && method === "GET");
    if (ownDataRequest && hasCallerEmployeeID) {
      return route.fulfill({
        status: 400,
        json: envelope(
          null,
          false,
          "employee_id is derived from provider claims",
        ),
      });
    }

    if (pathName === "/api/v1/auth/mock-provider-token" && method === "POST") {
      const requestedID = body.profile_id as keyof typeof sessions;
      if (requestedID && sessions[requestedID])
        currentSession = { ...sessions[requestedID] } as Session;
      return route.fulfill({
        json: envelope({
          provider_token: `mock-provider-${currentSession.actor.id}`,
          expires_at: currentSession.expires_at,
          claims: claimsFromSession(currentSession),
        }),
      });
    }

    if (pathName === "/api/v1/auth/me" && method === "GET") {
      const authorization = request.headers().authorization || "";
      if (
        options.mockProfiles &&
        !authorization.startsWith("Bearer mock-provider-")
      ) {
        return route.fulfill({
          status: 401,
          json: envelope(null, false, "authentication required"),
        });
      }
      return route.fulfill({
        json: envelope(claimsFromSession(currentSession)),
      });
    }

    if (pathName === "/api/v1/auth/bootstrap" && method === "GET") {
      return route.fulfill({
        json: envelope({
          mock_profiles_enabled: Boolean(options.mockProfiles),
          mock_profiles: mockProfiles(),
          debug_chrome_enabled: true,
        }),
      });
    }

    if (pathName === "/healthz" || pathName === "/readyz") {
      return route.fulfill({ json: envelope({ status: "ok" }) });
    }

    if (pathName === "/api/v1/events" && method === "GET") {
      return route.fulfill({ json: envelope(eventRows) });
    }

    if (/^\/api\/v1\/events\/[^/]+$/.test(pathName) && method === "GET") {
      const eventID = decodeURIComponent(pathName.split("/").pop() || "");
      return route.fulfill({
        json: envelope(
          eventRows.find((event) => event.event_id === eventID) ??
            fallbackEventDetail,
        ),
      });
    }

    if (
      /^\/api\/v1\/events\/[^/]+\/bookings$/.test(pathName) &&
      method === "POST"
    ) {
      return route.fulfill({
        json: envelope(
          options.bookingResponse ?? {
            registration: {
              registration_id: "reg-001",
              event_id: "evt-cets-001",
              employee_id: "E1001",
              status: "confirmed",
              idempotency_key: "book-evt-cets-001-E1001",
              created_at: "2026-01-02T09:00:00Z",
            },
            ticket: sampleTickets[0],
            remaining_capacity: 227,
            message: "booking confirmed",
          },
        ),
      });
    }

    if (pathName === "/api/v1/admin/events" && method === "GET") {
      return route.fulfill({ json: envelope(eventRows) });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+$/.test(pathName) &&
      method === "GET"
    ) {
      return route.fulfill({ json: envelope(fallbackEventDetail) });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+\/registrations$/.test(pathName) &&
      method === "GET"
    ) {
      return route.fulfill({
        json: envelope([
          {
            registration_id: "reg-001",
            event_id: "evt-cets-001",
            employee_id: "E1001",
            employee_name: "陳雅莉",
            status: "confirmed",
            idempotency_key: "book-evt-cets-001-E1001",
            family_count: 0,
            created_at: "2026-01-02T09:00:00Z",
            ticket: sampleTickets[0],
          },
        ]),
      });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+\/eligibility\/preview$/.test(
        pathName,
      ) &&
      method === "POST"
    ) {
      return route.fulfill({
        json: envelope({
          event_id: "evt-cets-001",
          match_count: body?.rule ? 2 : 0,
          zero_match: false,
        }),
      });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+\/eligibility$/.test(pathName) &&
      method === "PUT"
    ) {
      return route.fulfill({
        json: envelope({
          event_id: "evt-cets-001",
          match_count: 2,
          zero_match: false,
        }),
      });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+\/lottery-runs$/.test(pathName) &&
      method === "POST"
    ) {
      return route.fulfill({
        json: envelope({
          run_id: "lottery-001",
          event_id: "evt-cets-001",
          seed: String(body.seed || "seed"),
          status: "completed",
          winner_count: 1,
          created_by: currentSession.actor.id,
          created_at: "2026-01-04T10:00:00Z",
        }),
      });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+\/waitlist\/promote$/.test(pathName) &&
      method === "POST"
    ) {
      return route.fulfill({
        json: envelope({ message: "promoted", remaining_capacity: 229 }),
      });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+\/state$/.test(pathName) &&
      method === "POST"
    ) {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (
      pathName === "/api/v1/admin/notifications/deliveries" &&
      method === "GET"
    ) {
      return route.fulfill({ json: envelope(notificationDeliveries) });
    }

    if (pathName === "/api/v1/admin/reports" && method === "GET") {
      return route.fulfill({ json: envelope(reportRows) });
    }

    if (
      pathName === "/api/v1/admin/eligibility-impact-reviews" &&
      method === "GET"
    ) {
      return route.fulfill({ json: envelope(impactReviews) });
    }

    if (
      /^\/api\/v1\/admin\/notifications\/deliveries\/[^/]+\/retry$/.test(
        pathName,
      ) &&
      method === "POST"
    ) {
      return route.fulfill({ json: envelope(notificationDeliveries[0]) });
    }

    if (pathName === "/api/v1/checkins" && method === "POST") {
      return route.fulfill({
        json: envelope({
          checkin_id: "checkin-001",
          ticket_id: "ticket-001",
          event_id: "evt-cets-001",
          event_title: "Corporate Family Day",
          employee_id: "E1001",
          status: "accepted",
          reason_code: "accepted",
          scanned_at: "2026-01-10T10:10:00Z",
          duplicate: false,
          holder: {
            display_name: "陳雅莉",
            department: "Engineering",
            city: "Taipei",
          },
          family_count: 0,
        }),
      });
    }

    if (
      /^\/api\/v1\/checkins\/events\/[^/]+\/offline-package/.test(pathName) &&
      method === "GET"
    ) {
      return route.fulfill({
        json: envelope({
          batch_id: "batch-001",
          event_id: "evt-cets-001",
          device_id: "gate-offline-1",
          valid_until: "2026-12-31T23:59:59Z",
          package_signature: "sig-001",
          ticket_count: 1,
          tickets: [
            {
              ticket_id: "ticket-001",
              employee_id: "E1001",
              token_hash: "hash-001",
              holder: {
                display_name: "陳雅莉",
                department: "Engineering",
                city: "Taipei",
              },
              family_count: 0,
            },
          ],
        }),
      });
    }

    if (pathName === "/api/v1/checkins/offline-sync" && method === "POST") {
      return route.fulfill({
        json: envelope({
          batch_id: "batch-001",
          accepted: 0,
          duplicate: 0,
          conflict: 0,
          results: [],
        }),
      });
    }

    if (pathName === "/api/v1/me/tickets" && method === "GET") {
      return route.fulfill({ json: envelope(sampleTickets) });
    }

    if (/^\/api\/v1\/tickets\/[^/]+$/.test(pathName) && method === "GET") {
      const ticketID = decodeURIComponent(pathName.split("/").pop() || "");
      const ticket = sampleTickets.find((item) => item.ticket_id === ticketID);
      if (!ticket) {
        return route.fulfill({
          status: 404,
          contentType: "application/json",
          body: JSON.stringify(envelope(null, false, "ticket not found")),
        });
      }
      return route.fulfill({ json: envelope(ticket) });
    }

    if (
      /^\/api\/v1\/me\/registrations\/[^/]+\/cancel$/.test(pathName) &&
      method === "POST"
    ) {
      return route.fulfill({
        json: envelope(
          options.cancelResponse ?? {
            registration: {
              registration_id: "reg-001",
              event_id: "evt-cets-001",
              employee_id: "E1001",
              status: "cancelled",
              idempotency_key: "book-evt-cets-001-E1001",
              cancel_idempotency_key: "cancel-reg-001-E1001",
              cancel_reason: "employee cancellation",
              cancelled_at: "2026-01-04T09:00:00Z",
              created_at: "2026-01-02T09:00:00Z",
            },
            ticket: { ...sampleTickets[0], status: "revoked" },
            remaining_capacity: 228,
            message: "registration cancelled",
          },
        ),
      });
    }

    if (pathName === "/api/v1/admin/reports/exports" && method === "POST") {
      return route.fulfill({
        json: envelope({
          export_id: "export-001",
          requested_by: "admin-1",
          report_type: "events",
          status: "queued",
          object_key: "reports/export-001.zip",
          created_at: "2026-01-06T10:00:00Z",
        }),
      });
    }

    if (pathName.startsWith("/api/v1/admin/audit-logs") && method === "GET") {
      return route.fulfill({ json: envelope(auditRows) });
    }

    if (pathName === "/api/v1/notifications/preferences" && method === "GET") {
      return route.fulfill({
        json: envelope({
          employee_id: "E1001",
          email_enabled: true,
          in_app_enabled: true,
          opted_out_categories: [],
          updated_at: "2026-01-01T00:00:00Z",
        }),
      });
    }

    if (pathName === "/api/v1/notifications/preferences" && method === "PUT") {
      return route.fulfill({
        json: envelope({
          employee_id: "E1001",
          email_enabled: true,
          in_app_enabled: true,
          opted_out_categories: [],
          updated_at: "2026-01-01T00:00:00Z",
        }),
      });
    }

    if (pathName === "/api/v1/admin/seed-demo" && method === "POST") {
      return route.fulfill({ json: envelope({ status: "seeded" }) });
    }

    if (pathName === "/api/v1/admin/events" && method === "POST") {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (pathName.startsWith("/api/v1/admin/events/") && method === "PATCH") {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+\/duplicate$/.test(pathName) &&
      method === "POST"
    ) {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (
      /^\/api\/v1\/admin\/events\/[^/]+$/.test(pathName) &&
      method === "DELETE"
    ) {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    return route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify(envelope(null, false, "not mocked")),
    });
  });
}

export async function loginAs(
  page: Page,
  principalID: string,
  options: { mockProfiles?: boolean } = {},
) {
  await page.goto("/");
  if (options.mockProfiles) {
    await page.getByLabel("本機身分清單").waitFor({ state: "visible" });
    await page.getByRole("button", { name: new RegExp(principalID) }).click();
    await expect(page.getByLabel("目前登入身份").first()).toBeVisible();
  }
  await expect(page.locator("main")).toBeVisible();
}
