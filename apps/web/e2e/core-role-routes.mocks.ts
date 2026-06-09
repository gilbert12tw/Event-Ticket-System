import { expect, type Page, type Request, type Route } from "@playwright/test";
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
type MockRouteContext = {
  body: Record<string, unknown>;
  currentSession: Session;
  eventRows: (typeof sampleEvent)[];
  fallbackEventDetail: typeof sampleEvent;
  options: RouteMockOptions;
  pathName: string;
};

const withMethod =
  (method: string) =>
  (matcher: PathMatcher, data: () => unknown): MockRoute => [
    method,
    matcher,
    data,
  ];
const del = withMethod("DELETE");
const get = withMethod("GET");
const patch = withMethod("PATCH");
const post = withMethod("POST");
const put = withMethod("PUT");
const tinyPosterPng = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
  "base64",
);

const eventBookingPath = /^\/api\/v1\/events\/[^/]+\/bookings$/;
const eventDetailPath = /^\/api\/v1\/events\/[^/]+$/;
const eventEligibilityPath = /^\/api\/v1\/events\/[^/]+\/eligibility$/;
const eventPosterPath = /^\/api\/v1\/events\/[^/]+\/poster$/;
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

function isMockedRoutePath(pathName: string) {
  return (
    pathName.startsWith("/api/") ||
    pathName === "/healthz" ||
    pathName === "/readyz"
  );
}

function parseRequestBody(request: Request) {
  try {
    const postData = request.postData();
    return postData ? (JSON.parse(postData) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function isOwnDataRequest(pathName: string, method: string) {
  return (
    (pathName === "/api/v1/events" && method === "GET") ||
    (eventDetailPath.test(pathName) && method === "GET") ||
    (eventEligibilityPath.test(pathName) && method === "GET") ||
    (eventBookingPath.test(pathName) && method === "POST") ||
    (pathName === "/api/v1/me/tickets" && method === "GET")
  );
}

function hasCallerEmployeeID(url: URL, body: Record<string, unknown>) {
  return (
    url.searchParams.has("employee_id") || Object.hasOwn(body, "employee_id")
  );
}

function lotterySeed(value: unknown) {
  if (typeof value === "string" && value) return value;
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "seed";
}

function createRouteResponder(route: Route, method: string, pathName: string) {
  const ok = (data: unknown) => route.fulfill({ json: envelope(data) });
  const failure = (status: number, message: string) =>
    route.fulfill({ status, json: envelope(null, false, message) });
  const routeData = (routes: readonly MockRoute[]) => {
    const match = routes.find(
      ([routeMethod, matcher]) =>
        routeMethod === method && pathMatches(matcher, pathName),
    );
    return match ? ok(match[2]()) : undefined;
  };
  return { failure, ok, routeData };
}

function isMockProviderTokenRequest(pathName: string, method: string) {
  return pathName === "/api/v1/auth/mock-provider-token" && method === "POST";
}

function isAuthMeRequest(pathName: string, method: string) {
  return pathName === "/api/v1/auth/me" && method === "GET";
}

function isAuthBootstrapRequest(pathName: string, method: string) {
  return pathName === "/api/v1/auth/bootstrap" && method === "GET";
}

function sessionForRequestedProfile(
  body: Record<string, unknown>,
  currentSession: Session,
) {
  const requestedID = body.profile_id as keyof typeof sessions;
  return requestedID && sessions[requestedID]
    ? ({ ...sessions[requestedID] } as Session)
    : currentSession;
}

function providerTokenPayload(currentSession: Session) {
  return {
    provider_token: `mock-provider-${currentSession.actor.id}`,
    expires_at: currentSession.expires_at,
    claims: claimsFromSession(currentSession),
  };
}

function requiresMockAuthorization(
  request: Request,
  options: RouteMockOptions,
) {
  const authorization = request.headers().authorization || "";
  return (
    Boolean(options.mockProfiles) &&
    !authorization.startsWith("Bearer mock-provider-")
  );
}

function ticketFromPath(pathName: string) {
  const ticketID = eventIDFromPath(pathName);
  return sampleTickets.find((item) => item.ticket_id === ticketID);
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

function mockRoutesForContext(context: MockRouteContext): readonly MockRoute[] {
  const {
    body,
    currentSession,
    eventRows,
    fallbackEventDetail,
    options,
    pathName,
  } = context;
  return [
    get("/healthz", () => ({ status: "ok" })),
    get("/readyz", () => ({ status: "ok" })),
    get("/api/v1/events", () => eventRows),
    get(
      eventDetailPath,
      () =>
        eventRows.find(
          (event) => event.event_id === eventIDFromPath(pathName),
        ) ?? fallbackEventDetail,
    ),
    post(
      eventBookingPath,
      () => options.bookingResponse ?? defaultBookingResponse(),
    ),
    get("/api/v1/admin/events", () => eventRows),
    get("/api/v1/admin/hr/options", () => ({
      sites: [
        { value: "*", label: "所有廠區" },
        { value: "Taipei HQ", label: "Taipei HQ" },
        { value: "Tainan HQ", label: "Tainan HQ" },
      ],
    })),
    get(adminEventDetailPath, () => fallbackEventDetail),
    post(/^\/api\/v1\/admin\/events\/[^/]+\/state$/, () => sampleEvent),
    get(adminRegistrationPath, () => [registrationRow()]),
    post(
      adminRegistrationCancelPath,
      () => options.cancelResponse ?? defaultCancelResponse(),
    ),
    post(/^\/api\/v1\/admin\/tickets\/[^/]+\/revoke$/, () => ({
      ...sampleTickets[0],
      status: "revoked",
    })),
    post(/^\/api\/v1\/admin\/events\/[^/]+\/eligibility\/preview$/, () =>
      eligibilityResult(body.rule ? 2 : 0),
    ),
    put(adminEventEligibilityPath, () => eligibilityResult(2)),
    post(/^\/api\/v1\/admin\/events\/[^/]+\/lottery-runs$/, () => ({
      run_id: "lottery-001",
      event_id: "evt-cets-001",
      seed: lotterySeed(body.seed),
      status: "completed",
      input_snapshot_at: "2026-01-04T10:00:00Z",
      algorithm_version: "deterministic-sha256-v1",
      candidate_count: 2,
      winner_count: 1,
      created_by: currentSession.actor.id,
      created_at: "2026-01-04T10:00:00Z",
    })),
    post(/^\/api\/v1\/admin\/events\/[^/]+\/waitlist\/promote$/, () => ({
      message: "promoted",
      remaining_capacity: 229,
    })),
    post(
      /^\/api\/v1\/admin\/notifications\/deliveries\/[^/]+\/retry$/,
      () => notificationDeliveries[0],
    ),
    get("/api/v1/admin/notifications/deliveries", () => notificationDeliveries),
    get("/api/v1/admin/reports", () => reportRows),
    get("/api/v1/admin/eligibility-impact-reviews", () => impactReviews),
    post("/api/v1/checkins", acceptedCheckin),
    get(/^\/api\/v1\/checkins\/events\/[^/]+\/offline-package/, offlinePackage),
    post("/api/v1/checkins/offline-sync", () => ({
      batch_id: "batch-001",
      accepted: 0,
      duplicate: 0,
      conflict: 0,
      results: [],
    })),
    get("/api/v1/me/tickets", () => sampleTickets),
    post("/api/v1/admin/reports/exports", () => ({
      export_id: "export-001",
      requested_by: "admin-1",
      report_type: "events",
      status: "queued",
      object_key: "reports/export-001.zip",
      created_at: "2026-01-06T10:00:00Z",
    })),
    get(/^\/api\/v1\/admin\/audit-logs/, () => auditRows),
    get("/api/v1/notifications/preferences", notificationPreferences),
    put("/api/v1/notifications/preferences", notificationPreferences),
    post(
      /^\/api\/v1\/me\/registrations\/[^/]+\/cancel$/,
      () => options.cancelResponse ?? defaultCancelResponse(),
    ),
    post("/api/v1/admin/seed-demo", () => ({ status: "seeded" })),
    post("/api/v1/admin/events", () => sampleEvent),
    patch(/^\/api\/v1\/admin\/events\//, () => sampleEvent),
    post(/^\/api\/v1\/admin\/events\/[^/]+\/duplicate$/, () => sampleEvent),
    del(/^\/api\/v1\/admin\/events\/[^/]+$/, () => sampleEvent),
  ];
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
    if (!isMockedRoutePath(pathName)) {
      return route.continue();
    }

    const body = parseRequestBody(request);
    const { failure, ok, routeData } = createRouteResponder(
      route,
      method,
      pathName,
    );

    if (isOwnDataRequest(pathName, method) && hasCallerEmployeeID(url, body)) {
      return failure(400, "employee_id is derived from provider claims");
    }

    if (isMockProviderTokenRequest(pathName, method)) {
      currentSession = sessionForRequestedProfile(body, currentSession);
      return ok(providerTokenPayload(currentSession));
    }

    if (isAuthMeRequest(pathName, method)) {
      if (requiresMockAuthorization(request, options)) {
        return failure(401, "authentication required");
      }
      return ok(claimsFromSession(currentSession));
    }

    if (isAuthBootstrapRequest(pathName, method)) {
      return ok({
        mock_profiles_enabled: Boolean(options.mockProfiles),
        mock_profiles: mockProfiles(),
        debug_chrome_enabled: true,
      });
    }

    if (ticketPath.test(pathName) && method === "GET") {
      const ticket = ticketFromPath(pathName);
      if (!ticket) {
        return failure(404, "ticket not found");
      }
      return ok(ticket);
    }

    if (eventPosterPath.test(pathName) && method === "GET") {
      return route.fulfill({
        status: 200,
        contentType: "image/png",
        body: tinyPosterPng,
      });
    }

    const simpleResponse = routeData(
      mockRoutesForContext({
        body,
        currentSession,
        eventRows,
        fallbackEventDetail,
        options,
        pathName,
      }),
    );
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
