import { type APIRequestContext, expect, type Page, test } from "@playwright/test";

type Role = "employee" | "activity_admin" | "checkin_staff" | "hr_admin" | "system_admin";
type Actor = { id: string; role: Role };
type Envelope<T> = { success: boolean; data: T; error: string | null };
type MockProviderToken = { provider_token: string; expires_at: string };
type EventSummary = { event_id: string; title: string };
type Ticket = {
  ticket_id: string;
  event_id: string;
  employee_id: string;
  status: string;
  signed_token?: string;
  event_title?: string;
};
type NotificationDelivery = { delivery_id: string; status: string };

const admin: Actor = { id: "admin-1", role: "activity_admin" };
const employee: Actor = { id: "E1001", role: "employee" };
const secondEmployee: Actor = { id: "E1002", role: "employee" };
const staff: Actor = { id: "staff-1", role: "checkin_staff" };
const hr: Actor = { id: "hr-1", role: "hr_admin" };
const providerTokens = new Map<string, string>();

test.describe.serial("Phase 1 live production workflow", () => {
  test("exercises Phase 1 through real UI workflows without route mocking", async ({ page, request }) => {
    const browserErrors = collectBrowserErrors(page);
    const suffix = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const event = await createLiveEvent(request, suffix);

    await loginThroughUi(page, request, employee.id, "員工入口");
    await expectEmployeeCanBrowseAndBook(page, event);
    await expectEmployeeDetailRoute(page, event);
    await expectEmployeeTicketAndNotifications(page, event);
    const onlineToken = await signedTokenFor(request, employee, event.event_id);

    await loginThroughUi(page, request, secondEmployee.id, "員工入口");
    await expectEmployeeCanBrowseAndBook(page, event);
    const offlineToken = await signedTokenFor(request, secondEmployee, event.event_id);

    await loginThroughUi(page, request, staff.id, "驗票員入口");
    await expectOnlineCheckinAndDuplicate(page, onlineToken);
    await expectOfflineSyncWithNonEmptyScan(page, event, offlineToken);
    await expectForbiddenRoute(page, "/admin/events");

    await loginThroughUi(page, request, hr.id, "活動主辦入口");
    await expectReportsExportAndAuditFilters(page, event);
    const delivery = await waitForNotificationDelivery(page, request);
    await expectNotificationDeliveryRoute(page, delivery);

    await loginThroughUi(page, request, "system-1", "HR 報表入口");
    await expectRoute(page, "/admin/audit", "稽核入口");
    await expectForbiddenRoute(page, "/admin/events");

    expect(browserErrors).toEqual([]);
  });
});

function collectBrowserErrors(page: Page) {
  const errors: string[] = [];
  page.on("console", (message) => {
    const text = message.text();
    if (message.type() === "error" && !/status of (401|403|409)/.test(text)) errors.push(text);
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return errors;
}

async function createLiveEvent(request: APIRequestContext, suffix: string) {
  await api<{ status: string }>(request, admin, "POST", "/api/v1/admin/seed-demo", {});
  return api<EventSummary>(request, admin, "POST", "/api/v1/admin/events", {
    title: `Live Phase 1 UI Gate ${suffix}`,
    description: "Compose-backed Playwright UI workflow event",
    location: "Taipei HQ",
    starts_at: futureISO(72),
    registration_start: futureISO(-1),
    registration_close: futureISO(48),
    capacity: 2,
    status: "published",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active"
    }
  });
}

async function loginThroughUi(page: Page, request: APIRequestContext, principalID: string, landingHeading: string) {
  const providerToken = await providerTokenFor(request, principalID);
  await page.addInitScript((token) => {
    (globalThis as typeof globalThis & { __CETS_PROVIDER_TOKEN__?: string }).__CETS_PROVIDER_TOKEN__ = token;
  }, providerToken);
  await page.goto("/", { waitUntil: "networkidle" });
  const loginHeading = page.getByRole("heading", { name: "選擇一個模擬 provider profile" });
  if (await loginHeading.isVisible({ timeout: 1_000 }).catch(() => false)) {
    await page.getByRole("button", { name: new RegExp(principalID) }).click();
  } else if (!(await page.getByRole("heading", { name: landingHeading }).isVisible({ timeout: 1_000 }).catch(() => false))) {
    await expect(page.getByRole("button", { name: "切換 Profile" }).first()).toBeVisible();
    await page.getByRole("button", { name: "切換 Profile" }).first().click();
    await expect(loginHeading).toBeVisible();
    await page.getByRole("button", { name: new RegExp(principalID) }).click();
  }
  await expect(page.getByRole("heading", { name: landingHeading })).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectEmployeeCanBrowseAndBook(page: Page, event: EventSummary) {
  await expectRoute(page, "/user/events", "員工入口");
  const card = page.locator("article.event-card").filter({ hasText: event.title });
  await expect(card).toBeVisible();
  await expect(card.getByText("符合資格")).toBeVisible();
  await card.getByRole("button", { name: "報名" }).click();
  await expect(card.locator('[data-slot="badge"]').filter({ hasText: /^confirmed$/ })).toBeVisible();
  await expect(card.getByRole("button", { name: "已報名" })).toBeDisabled();
  await expectNoHorizontalOverflow(page);
}

async function expectEmployeeDetailRoute(page: Page, event: EventSummary) {
  await expectRoute(page, `/user/events/${encodeURIComponent(event.event_id)}`, "單一活動詳情");
  await expect(page.getByRole("heading", { name: event.title })).toBeVisible();
  await expect(page.locator(".meta-list div").filter({ hasText: "Event ID" }).locator("dd")).toHaveText(event.event_id);
}

async function expectEmployeeTicketAndNotifications(page: Page, event: EventSummary) {
  await expectRoute(page, "/user/tickets", "票券入口");
  await expect(page.locator(".ticket-row").filter({ hasText: event.title })).toBeVisible();
  await expect(page.getByText("Signed token 已保留給驗票流程")).toBeVisible();
  await expect(page.getByText("QR").first()).toBeVisible();

  await expectRoute(page, "/user/notifications", "通知中心");
  await expect(page.getByText(event.title).first()).toBeVisible();
  await expect(page.getByText("Registration").first()).toBeVisible();
  await expect(page.getByText("Ticket").first()).toBeVisible();
}

async function expectOnlineCheckinAndDuplicate(page: Page, token: string) {
  await expectRoute(page, "/admin/checkin", "驗票員入口");
  await page.getByLabel("Signed token").fill(token);
  await page.getByLabel("裝置 ID").fill("playwright-live-online");
  await page.getByRole("button", { name: "送出驗票" }).click();
  await expect(page.getByRole("heading", { name: "驗票成功" })).toBeVisible();

  await page.getByRole("button", { name: "送出驗票" }).click();
  await expect(page.getByRole("heading", { name: "重複掃描被拒絕" })).toBeVisible();
  await expect(page.getByText("duplicate")).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectOfflineSyncWithNonEmptyScan(page: Page, event: EventSummary, token: string) {
  await expectRoute(page, "/admin/checkin/offline", "離線名單");
  await page.getByLabel("活動").selectOption({ label: event.title });
  await page.getByLabel("裝置 ID").fill("playwright-live-offline");
  await page.getByRole("button", { name: "下載離線名單" }).click();
  await expect(page.getByText(/已下載 batch .*共 [1-9][0-9]* 張票券/)).toBeVisible();
  await expect(page.locator("table").filter({ hasText: secondEmployee.id })).toBeVisible();

  await page.getByLabel("scan batch").fill(token);
  await page.getByRole("button", { name: "同步名單" }).click();
  await expect(page.getByText("已同步 1 筆掃描")).toBeVisible();
  await expect(page.locator(".kpi").filter({ hasText: "accepted" }).getByText("1")).toBeVisible();
  await expect(page.getByText("accepted").first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectReportsExportAndAuditFilters(page: Page, event: EventSummary) {
  await expectRoute(page, "/admin/reports", "HR 報表入口");
  await expect(page.getByText(event.title).first()).toBeVisible();
  await page.getByPlaceholder("搜尋活動或 event id").fill(event.title);
  await expect(page.getByText(event.title).first()).toBeVisible();
  await page.getByRole("button", { name: "匯出 CSV" }).click();
  await expect(page.getByText(/Export exp_/)).toBeVisible();
  await expect(page.getByText(/exports\/exp_.*\.csv/)).toBeVisible();

  await expectRoute(page, "/admin/audit", "稽核入口");
  await page.getByLabel("Action").fill("report.export.requested");
  await page.getByLabel("Role").selectOption("hr_admin");
  await page.getByRole("button", { name: "套用 server filters" }).click();
  const auditRow = page.getByRole("row", { name: /report\.export\.requested.*hr_admin/ }).first();
  await expect(auditRow).toBeVisible();
  await auditRow.getByRole("button", { name: "檢視" }).click();
  await expect(page.getByText("report_type").first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectNotificationDeliveryRoute(page: Page, delivery: NotificationDelivery) {
  await expectRoute(page, "/admin/notifications", "Delivery log");
  await page.getByRole("combobox", { exact: true, name: "狀態" }).selectOption("all");
  await page.getByRole("button", { name: "重新整理" }).click();
  const deliveryRow = page.locator("tr").filter({ hasText: delivery.delivery_id }).first();
  await expect(deliveryRow).toBeVisible();
  await expect(deliveryRow.getByText(delivery.status)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Delivery log" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectForbiddenRoute(page: Page, path: string) {
  await page.goto(path, { waitUntil: "networkidle" });
  await expect(page.getByRole("heading", { name: "權限不足" })).toBeVisible();
  await expect(page.getByText("目前登入角色無法進入")).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectRoute(page: Page, path: string, heading: string) {
  await page.goto(path, { waitUntil: "networkidle" });
  await expect(page.locator("main").getByRole("heading", { name: heading }).first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => Math.ceil(document.documentElement.scrollWidth - window.innerWidth));
  expect(overflow, "no horizontal overflow").toBeLessThanOrEqual(1);
}

async function signedTokenFor(request: APIRequestContext, actor: Actor, eventID: string) {
  const tickets = await api<Ticket[]>(request, actor, "GET", "/api/v1/me/tickets");
  const ticket = tickets.find((candidate) => candidate.event_id === eventID);
  expect(ticket?.signed_token, `ticket token for ${actor.id} on ${eventID}`).toBeTruthy();
  return ticket?.signed_token || "";
}

async function waitForNotificationDelivery(page: Page, request: APIRequestContext) {
  for (let attempt = 0; attempt < 30; attempt++) {
    const deliveries = await api<NotificationDelivery[]>(request, hr, "GET", "/api/v1/admin/notifications/deliveries");
    const visible = deliveries.find((delivery) => delivery.status !== "failed" && delivery.status !== "dead_letter") || deliveries[0];
    if (visible) return visible;
    await page.waitForTimeout(1_000);
  }
  throw new Error("notification delivery worker did not create a visible delivery");
}

async function api<T>(request: APIRequestContext, actor: Actor, method: string, path: string, body?: unknown): Promise<T> {
  const response = await rawApi(request, actor, method, path, body);
  const text = await response.text();
  expect(response.ok(), `${method} ${path} failed: ${text}`).toBe(true);
  const payload = JSON.parse(text) as Envelope<T>;
  expect(payload.success, `${method} ${path} envelope error: ${payload.error}`).toBe(true);
  return payload.data;
}

async function rawApi(request: APIRequestContext, actor: Actor, method: string, path: string, body?: unknown) {
  const providerToken = await providerTokenFor(request, actor.id);
  return request.fetch(path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${providerToken}`
    },
    data: body
  });
}

async function providerTokenFor(request: APIRequestContext, profileID: string) {
  const cached = providerTokens.get(profileID);
  if (cached) return cached;

  const response = await request.post("/api/v1/auth/mock-provider-token", {
    headers: {
      "Content-Type": "application/json"
    },
    data: {
      profile_id: profileID
    }
  });
  const text = await response.text();
  expect(response.ok(), `POST /api/v1/auth/mock-provider-token failed for ${profileID}: ${text}`).toBe(true);
  const payload = JSON.parse(text) as Envelope<MockProviderToken>;
  expect(payload.success, `mock provider token envelope failed for ${profileID}: ${payload.error}`).toBe(true);
  expect(payload.data.provider_token, `mock provider token missing for ${profileID}`).toBeTruthy();
  providerTokens.set(profileID, payload.data.provider_token);
  return payload.data.provider_token;
}

function futureISO(hours: number) {
  return new Date(Date.now() + hours * 60 * 60 * 1000).toISOString();
}
