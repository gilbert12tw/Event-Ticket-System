import {
  type APIRequestContext,
  expect,
  type Page,
  test,
} from "@playwright/test";
import { randomUUID } from "node:crypto";
import {
  admin,
  api,
  chooseComboboxOption,
  collectBrowserErrors,
  employee,
  expectForbiddenRoute,
  expectNoHorizontalOverflow,
  expectRoute,
  futureISO,
  hr,
  loginThroughUi,
  mainHeading,
  secondEmployee,
  signedTokenFor,
  staff,
  type EventSummary,
  type NotificationDelivery,
  type Ticket,
  waitForNotificationDelivery,
} from "./support/live-flow-helpers";

test.describe.serial("第一階段實際產品流程", () => {
  test("透過真實介面流程驗證第一階段，不使用路由模擬", async ({
    page,
    request,
  }) => {
    const browserErrors = collectBrowserErrors(page);
    const suffix = `${Date.now()}-${randomUUID()}`;
    const event = await createLiveEvent(request, suffix);

    await loginThroughUi(page, request, employee.id, "活動探索");
    await expectEmployeeCanBrowseAndBook(page, event);
    await expectEmployeeDetailRoute(page, event);
    await expectEmployeeTicketAndNotifications(page, request, event);
    const onlineToken = await signedTokenFor(request, employee, event.event_id);

    await loginThroughUi(page, request, secondEmployee.id, "活動探索");
    await expectEmployeeCanBrowseAndBook(page, event);
    const offlineToken = await signedTokenFor(
      request,
      secondEmployee,
      event.event_id,
    );

    await loginThroughUi(page, request, staff.id, "現場驗票");
    await expectOnlineCheckinAndDuplicate(page, event, onlineToken);
    await expectOfflineSyncWithNonEmptyScan(page, event, offlineToken);
    await expectForbiddenRoute(page, "/admin/events");

    await loginThroughUi(page, request, hr.id, "人資報表");
    await expectReportsExportAndAuditFilters(page, event);
    const delivery = await waitForNotificationDelivery(page, request);

    await loginThroughUi(page, request, "system-1", "人資報表");
    await expectNotificationDeliveryRoute(page, delivery);
    await expectRoute(page, "/admin/audit", "稽核查詢");
    await expectForbiddenRoute(page, "/admin/events");

    expect(browserErrors).toEqual([]);
  });
});

async function createLiveEvent(request: APIRequestContext, suffix: string) {
  await api<{ status: string }>(
    request,
    admin,
    "POST",
    "/api/v1/admin/seed-demo",
    {},
  );
  return api<EventSummary>(request, admin, "POST", "/api/v1/admin/events", {
    title: `實際介面驗證活動 ${suffix}`,
    description: "容器環境中的介面流程驗證活動",
    location: "台北總部",
    starts_at: futureISO(-1),
    registration_start: futureISO(-2),
    registration_close: futureISO(48),
    capacity: 2,
    status: "published",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active",
    },
  });
}

async function expectEmployeeCanBrowseAndBook(page: Page, event: EventSummary) {
  await expectRoute(page, "/user/events", "活動探索");
  const card = page
    .locator("article.event-list-item")
    .filter({ hasText: event.title });
  await expect(card).toBeVisible();
  await expect(card.getByText("符合資格")).toBeVisible();
  await card
    .getByRole("link", { name: /立即報名|加入候補|設定同行人數/ })
    .click();
  await expect(mainHeading(page, "活動詳情")).toBeVisible();
  await expect(mainHeading(page, event.title)).toBeVisible();
  await page.getByRole("button", { name: /立即報名|加入候補/ }).click();
  await expect(page.getByText(/報名成功，票券已核發|已加入候補/)).toBeVisible();
  await expectRoute(page, "/user/events?tab=registered", "活動探索");
  const registeredCard = page
    .locator("article.event-list-item")
    .filter({ hasText: event.title });
  await expect(registeredCard).toBeVisible();
  await expect(
    registeredCard.locator('[data-slot="badge"]').filter({ hasText: "已報名" }),
  ).toBeVisible();
  await expect(
    registeredCard.getByRole("link", { name: "查看票券" }),
  ).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectEmployeeDetailRoute(page: Page, event: EventSummary) {
  await expectRoute(
    page,
    `/user/events/detail?event_id=${encodeURIComponent(event.event_id)}`,
    "活動詳情",
  );
  await expect(mainHeading(page, event.title)).toBeVisible();
  await expect(
    page
      .locator(".meta-list div")
      .filter({ hasText: "活動編號" })
      .locator("dd"),
  ).toHaveText(event.event_id);
}

async function expectEmployeeTicketAndNotifications(
  page: Page,
  request: APIRequestContext,
  event: EventSummary,
) {
  await expectRoute(page, "/user/tickets", "我的票券");
  await expect(
    page.locator(".ticket-row").filter({ hasText: event.title }),
  ).toBeVisible();
  await expect(page.getByLabel("票券二維碼")).toHaveCount(0);

  const tickets = await api<Ticket[]>(
    request,
    employee,
    "GET",
    "/api/v1/me/tickets",
  );
  const ticket = tickets.find(
    (candidate) => candidate.event_id === event.event_id,
  );
  expect(ticket?.ticket_id, `ticket for ${event.event_id}`).toBeTruthy();
  const signedToken = ticket?.signed_token || "";
  expect(signedToken, `ticket token for ${event.event_id}`).toBeTruthy();
  await page.locator(".ticket-row").filter({ hasText: event.title }).click();
  await expect(page).toHaveURL(
    new RegExp(
      String.raw`/user/tickets\?ticket_id=${encodeURIComponent(ticket?.ticket_id || "")}$`,
    ),
  );
  await expect(page.getByLabel("票券二維碼")).toBeVisible();
  await expect(page.getByText(signedToken)).toHaveCount(0);

  await expectRoute(page, "/user/notifications", "通知中心");
  await expectNotificationEntry(page, event.title, "報名");
  await expectNotificationEntry(page, event.title, "票券");
}

async function expectOnlineCheckinAndDuplicate(
  page: Page,
  event: EventSummary,
  token: string,
) {
  await expectRoute(page, "/admin/checkin", "現場驗票");
  await chooseComboboxOption(page, "驗票活動", event.title);
  await page.getByLabel("掃描或貼上票券").fill(token);
  await chooseComboboxOption(page, "驗票裝置", "自訂裝置");
  await page.getByLabel("自訂裝置代號").fill("playwright-live-online");
  await page.getByRole("button", { name: "送出驗票" }).click();
  await expect(mainHeading(page, "驗票成功")).toBeVisible();

  await page.getByRole("button", { name: "送出驗票" }).click();
  await expect(mainHeading(page, "重複掃描被拒絕")).toBeVisible();
  await expect(page.getByText("重複掃描").first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectOfflineSyncWithNonEmptyScan(
  page: Page,
  event: EventSummary,
  token: string,
) {
  await expectRoute(page, "/admin/checkin/offline", "離線驗票同步");
  await chooseComboboxOption(page, "活動", event.title);
  await chooseComboboxOption(page, "驗票裝置", "自訂裝置");
  await page.getByLabel("自訂裝置代號").fill("playwright-live-offline");
  await page.getByRole("button", { name: "下載離線名單" }).click();
  await expect(
    page.locator("table").filter({ hasText: secondEmployee.id }),
  ).toBeVisible();

  await page.getByLabel("掃描批次").fill(token);
  await page.getByRole("button", { name: "同步名單" }).click();
  await expect(page.getByRole("tab", { name: "3 同步結果" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(
    page.locator(".kpi").filter({ hasText: "成功" }).getByText("1"),
  ).toBeVisible();
  await expect(
    page
      .locator("tr")
      .filter({ hasText: secondEmployee.id })
      .filter({ hasText: "驗票成功" }),
  ).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectReportsExportAndAuditFilters(
  page: Page,
  event: EventSummary,
) {
  await expectRoute(page, "/admin/reports", "人資報表");
  await expectReportEntry(page, event.title);
  await page.getByPlaceholder("搜尋活動或活動編號").fill(event.title);
  await expectReportEntry(page, event.title);
  await page.getByRole("button", { name: "匯出完整參與報表" }).click();
  await page.getByRole("tab", { name: "匯出狀態" }).click();
  await expect(
    page.locator(".export-detail dd").filter({ hasText: /^exp_/ }),
  ).toBeVisible();
  await expect(page.getByText(/exports\/exp_.*\.csv/)).toBeVisible();

  await expectRoute(page, "/admin/audit", "稽核查詢");
  await page.getByLabel("操作").click();
  await page.getByRole("option", { name: "報表匯出" }).click();
  await page.getByLabel("角色").click();
  await page.getByRole("option", { name: "人資管理員" }).click();
  await page.getByRole("button", { name: "套用篩選" }).click();
  const auditRow = page
    .locator("main tr.interactive-row")
    .filter({ hasText: "報表匯出請求" })
    .filter({ hasText: "人資管理員" })
    .first();
  if (await auditRow.isVisible({ timeout: 1_000 }).catch(() => false)) {
    await auditRow.click();
  } else {
    const auditCard = page
      .locator(".audit-mobile-card")
      .filter({ hasText: "報表匯出請求" })
      .filter({ hasText: "人資管理員" })
      .first();
    await expect(auditCard).toBeVisible();
    await auditCard.getByRole("button", { name: "檢視稽核明細" }).click();
  }
  await expect(page.getByText("report_type").first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectReportEntry(page: Page, eventTitle: string) {
  const mobileCard = page
    .locator("main .mobile-summary-card")
    .filter({ hasText: eventTitle })
    .first();
  if (await mobileCard.isVisible({ timeout: 500 }).catch(() => false)) {
    await expect(mobileCard).toBeVisible();
    return;
  }
  await expect(
    page.locator("main tr").filter({ hasText: eventTitle }).first(),
  ).toBeVisible();
}

async function expectNotificationDeliveryRoute(
  page: Page,
  delivery: NotificationDelivery,
) {
  await expectRoute(page, "/admin/notifications", "通知投遞");
  await page
    .getByRole("group", { name: "投遞狀態" })
    .getByRole("button", { name: /^全部/ })
    .click();
  await page.getByRole("button", { name: "重新整理" }).click();
  await expectDeliveryEntry(page, delivery.delivery_id, "已送達");
  await expect(page.getByRole("heading", { name: "投遞紀錄" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

async function expectDeliveryEntry(
  page: Page,
  deliveryID: string,
  statusLabel: string,
) {
  const mobileCard = page
    .locator("main .mobile-summary-card")
    .filter({ hasText: deliveryID })
    .first();
  if (await mobileCard.isVisible({ timeout: 500 }).catch(() => false)) {
    await expect(mobileCard).toBeVisible();
    await expect(
      mobileCard.getByText(statusLabel, { exact: true }),
    ).toBeVisible();
    return;
  }
  const deliveryRow = page
    .locator("tr")
    .filter({ hasText: deliveryID })
    .first();
  await expect(deliveryRow).toBeVisible();
  await expect(
    deliveryRow.getByText(statusLabel, { exact: true }),
  ).toBeVisible();
}

async function expectNotificationEntry(
  page: Page,
  eventTitle: string,
  kind: string,
) {
  const mobileCard = page
    .locator("main .mobile-summary-card")
    .filter({ hasText: eventTitle })
    .filter({ hasText: kind })
    .first();
  if (await mobileCard.isVisible({ timeout: 500 }).catch(() => false)) {
    await expect(mobileCard).toBeVisible();
    return;
  }
  await expect(
    page
      .locator("main tr")
      .filter({ hasText: eventTitle })
      .filter({ hasText: kind })
      .first(),
  ).toBeVisible();
}
