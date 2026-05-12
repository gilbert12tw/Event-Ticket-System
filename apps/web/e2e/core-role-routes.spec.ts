import { expect, test, type Page } from "@playwright/test";

const sessions = {
  "E1001": { actor: { id: "E1001", role: "employee" as const }, expires_at: "2099-12-31T23:59:59Z" },
  "admin-1": { actor: { id: "admin-1", role: "activity_admin" as const }, expires_at: "2099-12-31T23:59:59Z" },
  "staff-1": { actor: { id: "staff-1", role: "checkin_staff" as const }, expires_at: "2099-12-31T23:59:59Z" },
  "hr-1": { actor: { id: "hr-1", role: "hr_admin" as const }, expires_at: "2099-12-31T23:59:59Z" },
  "system-1": { actor: { id: "system-1", role: "system_admin" as const }, expires_at: "2099-12-31T23:59:59Z" }
};

type Session = (typeof sessions)[keyof typeof sessions];

const sampleEvent = {
  event_id: "evt-cets-001",
  title: "Phase 1 企業午餐日",
  description: "內部 demo 活動",
  location: "台北總部多功能廳",
  starts_at: "2026-01-10T10:00:00Z",
  registration_start: "2026-01-01T10:00:00Z",
  registration_close: "2026-01-09T23:00:00Z",
  capacity: 240,
  status: "published",
  allocation_mode: "first_come_first_served",
  created_by: "admin-1",
  created_at: "2026-01-01T08:00:00Z",
  updated_at: "2026-01-01T08:00:00Z",
  rule: {
    department: "Engineering",
    site: "Taipei",
    min_grade: 5,
    employment_status: "active"
  },
  eligible: true,
  eligibility_reason: "符合資格",
  confirmed_count: 12,
  waitlist_count: 0,
  remaining_capacity: 228,
  current_user_status: "confirmed",
  current_user_ticket: {
    ticket_id: "ticket-001",
    registration_id: "reg-001",
    event_id: "evt-cets-001",
    employee_id: "E1001",
    status: "active",
    issued_at: "2026-01-02T09:00:00Z",
    qr_payload: "mocked-qr-token",
    signed_token: "mocked-token"
  }
};

const sampleTickets = [
  {
    ticket_id: "ticket-001",
    registration_id: "reg-001",
    event_id: "evt-cets-001",
    employee_id: "E1001",
    status: "active",
    issued_at: "2026-01-02T09:00:00Z",
    event_title: "Phase 1 企業午餐日",
    event_location: "台北總部多功能廳",
    event_starts_at: "2026-01-10T10:00:00Z",
    employee_name: "Ariel Chen"
  }
];

const reportRows = [
  {
    event_id: "evt-cets-001",
    title: "Phase 1 企業午餐日",
    capacity: 240,
    confirmed_count: 12,
    waitlist_count: 0,
    ticket_count: 12,
    checkin_count: 8,
    remaining_capacity: 228,
    starts_at: "2026-01-10T10:00:00Z"
  }
];

const notificationDeliveries = [
  {
    delivery_id: "delivery-001",
    outbox_id: "outbox-001",
    employee_id: "E1001",
    channel: "in-app",
    status: "sent",
    attempts: 1,
    last_error: "",
    created_at: "2026-01-05T08:00:00Z",
    updated_at: "2026-01-05T08:01:00Z"
  }
];

const auditRows = [
  {
    audit_id: "audit-001",
    actor_id: "admin-1",
    role: "activity_admin",
    action: "event.created",
    entity_type: "event",
    entity_id: "evt-cets-001",
    metadata: "source=ui",
    created_at: "2026-01-01T08:30:00Z"
  }
];

const impactReviews = [
  {
    review_id: "review-001",
    event_id: "evt-cets-001",
    employee_id: "E1001",
    ticket_id: "ticket-001",
    status: "open",
    reason: "示範資料",
    created_at: "2026-01-03T10:00:00Z"
  }
];

const roleCases = [
  {
    name: "employee",
    principalID: "E1001",
    routes: [
      { path: "/user/events", heading: "員工入口" },
      { path: "/user/events/evt-cets-001", heading: "單一活動詳情" },
      { path: "/user/tickets", heading: "票券入口" },
      { path: "/user/notifications", heading: "通知中心" }
    ]
  },
  {
    name: "activity_admin",
    principalID: "admin-1",
    routes: [
      { path: "/admin/events", heading: "活動主辦入口" },
      { path: "/admin/registrations", heading: "報名治理入口" },
      { path: "/admin/notifications", heading: "Delivery log" }
    ]
  },
  {
    name: "checkin_staff",
    principalID: "staff-1",
    routes: [
      { path: "/admin/checkin", heading: "驗票員入口" },
      { path: "/admin/checkin/offline", heading: "離線名單" }
    ]
  },
  {
    name: "hr_admin",
    principalID: "hr-1",
    routes: [
      { path: "/admin/reports", heading: "HR 報表入口" },
      { path: "/admin/hr-settings", heading: "HR 同步設定" },
      { path: "/admin/audit", heading: "稽核入口" }
    ]
  },
  {
    name: "system_admin",
    principalID: "system-1",
    routes: [
      { path: "/admin/reports", heading: "HR 報表入口" },
      { path: "/admin/hr-settings", heading: "HR 同步設定" },
      { path: "/admin/audit", heading: "稽核入口" },
      { path: "/admin/notifications", heading: "Delivery log" }
    ]
  }
];

function envelope<T>(data: T, status = true, error: string | null = null) {
  return {
    success: status,
    data,
    error
  };
}

function claimsFromSession(session: Session) {
  return {
    employee_id: session.actor.id,
    display_name: session.actor.id,
    role_claims: [session.actor.role],
    mapped_roles: [session.actor.role],
    department: "Engineering",
    site: "Taipei",
    city: "Taipei",
    claims_status: "complete"
  };
}

function mockProfiles() {
  return Object.values(sessions).map((session) => ({
    profile_id: session.actor.id,
    display_name: session.actor.id,
    role_claims: [session.actor.role],
    mapped_roles: [session.actor.role],
    department: session.actor.role === "employee" ? "Engineering" : "Operations",
    site: "Taipei",
    city: "Taipei"
  }));
}

async function ensureSessionRoutes(page: Page, principalID: keyof typeof sessions, options: { mockProfiles?: boolean } = {}) {
  const session = { ...sessions[principalID] } as Session;
  let currentSession = session;

  await page.route("**/*", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const pathName = url.pathname;
    const method = request.method().toUpperCase();
    const isMockedPath = pathName.startsWith("/api/") || pathName === "/healthz" || pathName === "/readyz";
    if (!isMockedPath) {
      return route.continue();
    }

    let body: Record<string, unknown> = {};
    try {
      if (request.postData()) {
        body = JSON.parse(request.postData() || "{}") as Record<string, unknown>;
      }
    } catch {
      body = {};
    }

    const hasCallerEmployeeID = url.searchParams.has("employee_id") || Object.prototype.hasOwnProperty.call(body, "employee_id");
    const ownDataRequest =
      (pathName === "/api/v1/events" && method === "GET") ||
      (/^\/api\/v1\/events\/[^/]+$/.test(pathName) && method === "GET") ||
      (/^\/api\/v1\/events\/[^/]+\/eligibility$/.test(pathName) && method === "GET") ||
      (/^\/api\/v1\/events\/[^/]+\/bookings$/.test(pathName) && method === "POST") ||
      (pathName === "/api/v1/me/tickets" && method === "GET");
    if (ownDataRequest && hasCallerEmployeeID) {
      return route.fulfill({ status: 400, json: envelope(null, false, "employee_id is derived from provider claims") });
    }

    if (pathName === "/api/v1/auth/mock-provider-token" && method === "POST") {
      const requestedID = body.profile_id as keyof typeof sessions;
      if (requestedID && sessions[requestedID]) currentSession = { ...sessions[requestedID] } as Session;
      return route.fulfill({
        json: envelope({
          provider_token: `mock-provider-${currentSession.actor.id}`,
          expires_at: currentSession.expires_at,
          claims: claimsFromSession(currentSession)
        })
      });
    }

    if (pathName === "/api/v1/auth/me" && method === "GET") {
      const authorization = request.headers().authorization || "";
      if (options.mockProfiles && !authorization.startsWith("Bearer mock-provider-")) {
        return route.fulfill({ status: 401, json: envelope(null, false, "authentication required") });
      }
      return route.fulfill({ json: envelope(claimsFromSession(currentSession)) });
    }

    if (pathName === "/api/v1/auth/bootstrap" && method === "GET") {
      return route.fulfill({ json: envelope({ mock_profiles_enabled: Boolean(options.mockProfiles), mock_profiles: mockProfiles() }) });
    }

    if (pathName === "/healthz" || pathName === "/readyz") {
      return route.fulfill({ json: envelope({ status: "ok" }) });
    }

    if (pathName === "/api/v1/events" && method === "GET") {
      return route.fulfill({ json: envelope([sampleEvent]) });
    }

    if (/^\/api\/v1\/events\/[^/]+$/.test(pathName) && method === "GET") {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (pathName === "/api/v1/admin/events" && method === "GET") {
      return route.fulfill({ json: envelope([sampleEvent]) });
    }

    if (/^\/api\/v1\/admin\/events\/[^/]+$/.test(pathName) && method === "GET") {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (/^\/api\/v1\/admin\/events\/[^/]+\/registrations$/.test(pathName) && method === "GET") {
      return route.fulfill({ json: envelope([]) });
    }

    if (/^\/api\/v1\/admin\/events\/[^/]+\/waitlist\/promote$/.test(pathName) && method === "POST") {
      return route.fulfill({ json: envelope({ message: "promoted", remaining_capacity: 229 }) });
    }

    if (/^\/api\/v1\/admin\/events\/[^/]+\/state$/.test(pathName) && method === "POST") {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (pathName === "/api/v1/admin/notifications/deliveries" && method === "GET") {
      return route.fulfill({ json: envelope(notificationDeliveries) });
    }

    if (pathName === "/api/v1/admin/reports" && method === "GET") {
      return route.fulfill({ json: envelope(reportRows) });
    }

    if (pathName === "/api/v1/admin/eligibility-impact-reviews" && method === "GET") {
      return route.fulfill({ json: envelope(impactReviews) });
    }

    if (/^\/api\/v1\/admin\/notifications\/deliveries\/[^/]+\/retry$/.test(pathName) && method === "POST") {
      return route.fulfill({ json: envelope(notificationDeliveries[0]) });
    }

    if (pathName === "/api/v1/checkins" && method === "POST") {
      return route.fulfill({ json: envelope({}) });
    }

    if (/^\/api\/v1\/checkins\/events\/[^/]+\/offline-package/.test(pathName) && method === "GET") {
      return route.fulfill({
        json: envelope({
          batch_id: "batch-001",
          event_id: "evt-cets-001",
          device_id: "gate-offline-1",
          valid_until: "2026-12-31T23:59:59Z",
          package_signature: "sig-001",
          ticket_count: 1,
          tickets: [{ ticket_id: "ticket-001", employee_id: "E1001", token_hash: "hash-001" }]
        })
      });
    }

    if (pathName === "/api/v1/checkins/offline-sync" && method === "POST") {
      return route.fulfill({ json: envelope({ batch_id: "batch-001", accepted: 0, duplicate: 0, conflict: 0, results: [] }) });
    }

    if (pathName === "/api/v1/me/tickets" && method === "GET") {
      return route.fulfill({ json: envelope(sampleTickets) });
    }

    if (pathName === "/api/v1/admin/reports/exports" && method === "POST") {
      return route.fulfill({
        json: envelope({
          export_id: "export-001",
          requested_by: "admin-1",
          report_type: "events",
          status: "queued",
          object_key: "reports/export-001.zip",
          created_at: "2026-01-06T10:00:00Z"
        })
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
          updated_at: "2026-01-01T00:00:00Z"
        })
      });
    }

    if (pathName === "/api/v1/notifications/preferences" && method === "PUT") {
      return route.fulfill({
        json: envelope({
          employee_id: "E1001",
          email_enabled: true,
          in_app_enabled: true,
          opted_out_categories: [],
          updated_at: "2026-01-01T00:00:00Z"
        })
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

    if (/^\/api\/v1\/admin\/events\/[^/]+\/duplicate$/.test(pathName) && method === "POST") {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    if (/^\/api\/v1\/admin\/events\/[^/]+$/.test(pathName) && method === "DELETE") {
      return route.fulfill({ json: envelope(sampleEvent) });
    }

    return route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify(envelope(null, false, "not mocked")) });
  });
}

async function loginAs(page: Page, principalID: string, options: { mockProfiles?: boolean } = {}) {
  await page.goto("/");
  if (options.mockProfiles) {
    await page.getByLabel("Mock provider profiles").waitFor({ state: "visible" });
    await page.getByRole("button", { name: new RegExp(principalID) }).click();
  }
  await expect(page.locator("main")).toBeVisible();
}

async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => {
    const doc = document.documentElement;
    const body = document.body;
    const contentWidth = Math.max(
      doc.scrollWidth,
      doc.offsetWidth,
      doc.clientWidth,
      body.scrollWidth,
      body.offsetWidth,
      body.clientWidth,
      window.innerWidth
    );
    return Math.ceil(contentWidth - window.innerWidth);
  });
  expect(overflow, "no horizontal overflow").toBeLessThanOrEqual(1);

  const contextLeaks = await page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLElement>(".workspace-context")).flatMap((context, index) => {
      const bounds = context.getBoundingClientRect();
      return Array.from(context.children)
        .filter((child): child is HTMLElement => child instanceof HTMLElement && child.offsetParent !== null)
        .filter((child) => {
          const childBounds = child.getBoundingClientRect();
          return childBounds.left < bounds.left - 1 || childBounds.right > bounds.right + 1;
        })
        .map((child) => ({ index, className: child.className, right: child.getBoundingClientRect().right, parentRight: bounds.right }));
    })
  );
  expect(contextLeaks, "workspace context children stay inside their panel").toEqual([]);

  const actionLeaks = await page.evaluate(() => {
    const selectors = [".toolbar", ".row-actions", ".status-selectors", ".form-actions", ".section-heading"];
    return Array.from(document.querySelectorAll<HTMLElement>(selectors.join(","))).flatMap((container, index) => {
      const style = window.getComputedStyle(container);
      if (container.offsetParent === null && style.position !== "fixed") return [];
      const bounds = container.getBoundingClientRect();
      if (bounds.width <= 0 || bounds.height <= 0) return [];
      return Array.from(container.children)
        .filter((child): child is HTMLElement => child instanceof HTMLElement && child.offsetParent !== null)
        .filter((child) => {
          const childBounds = child.getBoundingClientRect();
          return childBounds.left < bounds.left - 1 || childBounds.right > bounds.right + 1;
        })
        .map((child) => ({ index, selector: selectors.find((selector) => container.matches(selector)), className: child.className }));
    });
  });
  expect(actionLeaks, "toolbar and action children stay inside their containers").toEqual([]);

  const clippedButtons = await page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .filter((button) => button.offsetParent !== null)
      .filter((button) => button.scrollWidth > button.clientWidth + 1 || button.scrollHeight > button.clientHeight + 1)
      .map((button) => ({
        text: button.textContent?.trim() || button.getAttribute("aria-label") || "",
        className: button.className,
        scrollWidth: button.scrollWidth,
        clientWidth: button.clientWidth
      }))
  );
  expect(clippedButtons, "visible buttons do not clip their text").toEqual([]);

  const apiLayout = await page.evaluate(() => {
    const shell = document.querySelector<HTMLElement>(".app-shell");
    const api = document.querySelector<HTMLElement>(".api-panel");
    if (!shell || !api) return null;
    return {
      columns: window.getComputedStyle(shell).gridTemplateColumns.split(" ").filter(Boolean).length,
      position: window.getComputedStyle(api).position,
      state: api.dataset.state,
      width: window.innerWidth
    };
  });
  expect(apiLayout?.state, "API activity starts collapsed").toBe("collapsed");
  if (apiLayout && apiLayout.width > 900 && apiLayout.width < 1680) {
    expect(apiLayout.columns, "collapsed API activity does not reserve a third shell column").toBeLessThanOrEqual(2);
  }
  if (apiLayout && apiLayout.width > 1240 && apiLayout.width < 1680) {
    expect(apiLayout.position, "collapsed API activity floats outside the main grid").toBe("fixed");
  }
}

async function expectNotificationControlsCompact(page: Page) {
  const radioMetrics = await page.evaluate(() =>
    Array.from(document.querySelectorAll<HTMLInputElement>(".status-selectors input[type='radio']")).map((input) => {
      const bounds = input.getBoundingClientRect();
      return {
        width: Math.round(bounds.width),
        height: Math.round(bounds.height)
      };
    })
  );
  expect(radioMetrics.length, "notification delivery status radios are rendered").toBeGreaterThan(0);
  expect(radioMetrics, "notification delivery status radios stay compact").toEqual(
    expect.arrayContaining([expect.objectContaining({ width: expect.any(Number), height: expect.any(Number) })])
  );
  for (const metric of radioMetrics) {
    expect(metric.width, "status radio width").toBeLessThanOrEqual(20);
    expect(metric.height, "status radio height").toBeLessThanOrEqual(20);
  }
}

for (const roleCase of roleCases) {
  for (const route of roleCase.routes) {
    test.describe(`${roleCase.name}: ${route.path}`, () => {
      test(`can load route and stay horizontally contained`, async ({ page }) => {
        const consoleErrors: string[] = [];
        page.on("console", (message) => {
          if (message.type() === "error") consoleErrors.push(message.text());
        });
        await ensureSessionRoutes(page, roleCase.principalID);
        await loginAs(page, roleCase.principalID);
        await page.goto(route.path, { waitUntil: "domcontentloaded" });
        await expect(page.getByRole("heading", { name: route.heading, exact: true }).first()).toBeVisible();
        await expectNoHorizontalOverflow(page);
        if (route.path === "/admin/notifications") {
          await expectNotificationControlsCompact(page);
        }
        expect(consoleErrors).toEqual([]);
      });
    });
  }
}

test("shows explicit unauthorized state for forbidden deep links", async ({ page }) => {
  await ensureSessionRoutes(page, "staff-1");
  await loginAs(page, "staff-1");
  await page.goto("/admin/events", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "權限不足" })).toBeVisible();
  await expect(page.getByText("目前登入角色無法進入")).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("keeps the demo runbook available for mock provider profiles", async ({ page }) => {
  await ensureSessionRoutes(page, "admin-1", { mockProfiles: true });
  await loginAs(page, "admin-1", { mockProfiles: true });
  await page.getByRole("button", { name: "跑完整 Demo" }).click();
  await expect(page.getByRole("heading", { name: "Demo Runbook", level: 1 })).toBeVisible();
  await expect(page.getByRole("button", { name: /Run full demo|執行中/ })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});
