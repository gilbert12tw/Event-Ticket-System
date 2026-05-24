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

type PathMatcher = string | RegExp;

type MockRoute = readonly [
  method: string,
  matcher: PathMatcher,
  data: () => unknown,
];

const eventBookingPath = /^\/api\/v1\/events\/[^/]+\/bookings$/;
const eventDetailPath = /^\/api\/v1\/events\/[^/]+$/;
const eventEligibilityPath = /^\/api\/v1\/events\/[^/]+\/eligibility$/;
const adminEventDetailPath = /^\/api\/v1\/admin\/events\/[^/]+$/;
const adminRegistrationPath =
  /^\/api\/v1\/admin\/events\/[^/]+\/registrations$/;
const adminRegistrationCancelPath =
  /^\/api\/v1\/admin\/events\/[^/]+\/registrations\/[^/]+\/cancel$/;
const adminEventEligibilityPath =
  /^\/api\/v1\/admin\/events\/[^/]+\/eligibility$/;
const ticketPath = /^\/api\/v1\/tickets\/[^/]+$/;

function pathMatches(matcher: PathMatcher, pathName: string) {
  return typeof matcher === "string"
    ? matcher === pathName
    : matcher.test(pathName);
}

function eventIDFromPath(pathName: string) {
  return decodeURIComponent(pathName.split("/").pop() || "");
}

function registration(status = "confirmed") {
  return {
    registration_id: "reg-001",
    event_id: "evt-cets-001",
    employee_id: "E1001",
    status,
    idempotency_key: "book-evt-cets-001-E1001",
    created_at: "2026-01-02T09:00:00Z",
  };
}

function registrationRow() {
  return {
    ...registration(),
    employee_name: "陳雅莉",
    family_count: 0,
    ticket: sampleTickets[0],
  };
}

function defaultBookingResponse() {
  return {
    registration: registration(),
    ticket: sampleTickets[0],
    remaining_capacity: 227,
    message: "booking confirmed",
  };
}

function defaultCancelResponse() {
  return {
    registration: {
      ...registration("cancelled"),
      cancel_idempotency_key: "cancel-reg-001-E1001",
      cancel_reason: "employee cancellation",
      cancelled_at: "2026-01-04T09:00:00Z",
    },
    ticket: { ...sampleTickets[0], status: "revoked" },
    remaining_capacity: 228,
    message: "registration cancelled",
  };
}

function notificationPreferences() {
  return {
    employee_id: "E1001",
    email_enabled: true,
    in_app_enabled: true,
    opted_out_categories: [],
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function holder() {
  return {
    display_name: "陳雅莉",
    department: "Engineering",
    city: "Taipei",
  };
}

function acceptedCheckin() {
  return {
    checkin_id: "checkin-001",
    ticket_id: "ticket-001",
    event_id: "evt-cets-001",
    event_title: "Corporate Family Day",
    employee_id: "E1001",
    status: "accepted",
    reason_code: "accepted",
    scanned_at: "2026-01-10T10:10:00Z",
    duplicate: false,
    holder: holder(),
    family_count: 0,
  };
}

function eligibilityResult(matchCount: number) {
  return {
    event_id: "evt-cets-001",
    match_count: matchCount,
    zero_match: false,
  };
}

function offlinePackage() {
  return {
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
        holder: holder(),
        family_count: 0,
      },
    ],
  };
}

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
    const ok = (data: unknown) => route.fulfill({ json: envelope(data) });
    const failure = (status: number, message: string) =>
      route.fulfill({ status, json: envelope(null, false, message) });
    const routeData = (routes: MockRoute[]) => {
      const match = routes.find(
        ([routeMethod, matcher]) =>
          routeMethod === method && pathMatches(matcher, pathName),
      );
      return match ? ok(match[2]()) : undefined;
    };

    const hasCallerEmployeeID =
      url.searchParams.has("employee_id") ||
      Object.prototype.hasOwnProperty.call(body, "employee_id");
    const ownDataRequest =
      (pathName === "/api/v1/events" && method === "GET") ||
      (eventDetailPath.test(pathName) && method === "GET") ||
      (eventEligibilityPath.test(pathName) && method === "GET") ||
      (eventBookingPath.test(pathName) && method === "POST") ||
      (pathName === "/api/v1/me/tickets" && method === "GET");
    if (ownDataRequest && hasCallerEmployeeID) {
      return failure(400, "employee_id is derived from provider claims");
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
        return failure(401, "authentication required");
      }
      return ok(claimsFromSession(currentSession));
    }

    if (pathName === "/api/v1/auth/bootstrap" && method === "GET") {
      return ok({
        mock_profiles_enabled: Boolean(options.mockProfiles),
        mock_profiles: mockProfiles(),
        debug_chrome_enabled: true,
      });
    }

    if (ticketPath.test(pathName) && method === "GET") {
      const ticketID = eventIDFromPath(pathName);
      const ticket = sampleTickets.find((item) => item.ticket_id === ticketID);
      if (!ticket) {
        return failure(404, "ticket not found");
      }
      return ok(ticket);
    }

    const simpleResponse = routeData([
      ["GET", "/healthz", () => ({ status: "ok" })],
      ["GET", "/readyz", () => ({ status: "ok" })],
      ["GET", "/api/v1/events", () => eventRows],
      [
        "GET",
        eventDetailPath,
        () =>
          eventRows.find(
            (event) => event.event_id === eventIDFromPath(pathName),
          ) ?? fallbackEventDetail,
      ],
      [
        "POST",
        eventBookingPath,
        () => options.bookingResponse ?? defaultBookingResponse(),
      ],
      ["GET", "/api/v1/admin/events", () => eventRows],
      ["GET", adminEventDetailPath, () => fallbackEventDetail],
      ["POST", /^\/api\/v1\/admin\/events\/[^/]+\/state$/, () => sampleEvent],
      ["GET", adminRegistrationPath, () => [registrationRow()]],
      [
        "POST",
        adminRegistrationCancelPath,
        () => options.cancelResponse ?? defaultCancelResponse(),
      ],
      [
        "POST",
        /^\/api\/v1\/admin\/tickets\/[^/]+\/revoke$/,
        () => ({ ...sampleTickets[0], status: "revoked" }),
      ],
      [
        "POST",
        /^\/api\/v1\/admin\/events\/[^/]+\/eligibility\/preview$/,
        () => eligibilityResult(body.rule ? 2 : 0),
      ],
      ["PUT", adminEventEligibilityPath, () => eligibilityResult(2)],
      [
        "POST",
        /^\/api\/v1\/admin\/events\/[^/]+\/lottery-runs$/,
        () => ({
          run_id: "lottery-001",
          event_id: "evt-cets-001",
          seed: String(body.seed || "seed"),
          status: "completed",
          winner_count: 1,
          created_by: currentSession.actor.id,
          created_at: "2026-01-04T10:00:00Z",
        }),
      ],
      [
        "POST",
        /^\/api\/v1\/admin\/events\/[^/]+\/waitlist\/promote$/,
        () => ({ message: "promoted", remaining_capacity: 229 }),
      ],
      [
        "POST",
        /^\/api\/v1\/admin\/notifications\/deliveries\/[^/]+\/retry$/,
        () => notificationDeliveries[0],
      ],
      [
        "GET",
        "/api/v1/admin/notifications/deliveries",
        () => notificationDeliveries,
      ],
      ["GET", "/api/v1/admin/reports", () => reportRows],
      ["GET", "/api/v1/admin/eligibility-impact-reviews", () => impactReviews],
      ["POST", "/api/v1/checkins", acceptedCheckin],
      [
        "GET",
        /^\/api\/v1\/checkins\/events\/[^/]+\/offline-package/,
        offlinePackage,
      ],
      [
        "POST",
        "/api/v1/checkins/offline-sync",
        () => ({
          batch_id: "batch-001",
          accepted: 0,
          duplicate: 0,
          conflict: 0,
          results: [],
        }),
      ],
      ["GET", "/api/v1/me/tickets", () => sampleTickets],
      [
        "POST",
        "/api/v1/admin/reports/exports",
        () => ({
          export_id: "export-001",
          requested_by: "admin-1",
          report_type: "events",
          status: "queued",
          object_key: "reports/export-001.zip",
          created_at: "2026-01-06T10:00:00Z",
        }),
      ],
      ["GET", /^\/api\/v1\/admin\/audit-logs/, () => auditRows],
      ["GET", "/api/v1/notifications/preferences", notificationPreferences],
      ["PUT", "/api/v1/notifications/preferences", notificationPreferences],
      [
        "POST",
        /^\/api\/v1\/me\/registrations\/[^/]+\/cancel$/,
        () => options.cancelResponse ?? defaultCancelResponse(),
      ],
      ["POST", "/api/v1/admin/seed-demo", () => ({ status: "seeded" })],
      ["POST", "/api/v1/admin/events", () => sampleEvent],
      ["PATCH", /^\/api\/v1\/admin\/events\//, () => sampleEvent],
      [
        "POST",
        /^\/api\/v1\/admin\/events\/[^/]+\/duplicate$/,
        () => sampleEvent,
      ],
      ["DELETE", /^\/api\/v1\/admin\/events\/[^/]+$/, () => sampleEvent],
    ]);
    if (simpleResponse) {
      return simpleResponse;
    }

    return failure(404, "not mocked");
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
