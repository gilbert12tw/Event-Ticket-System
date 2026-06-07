import { expect, type APIRequestContext, type Page } from "@playwright/test";
import {
  admin,
  api,
  employee,
  expectNoHorizontalOverflow,
  expectRoute,
  mainHeading,
  secondEmployee,
  type EventSummary,
  type NotificationDelivery,
  type RegistrationDetail,
  type Ticket,
} from "./live-flow-helpers";
import {
  expectPosterAPI,
  expectPosterRendered,
  posterUploadFile,
  type PosterFixture,
} from "./poster-fixtures";

export async function createEventWithPosterThroughUi(
  page: Page,
  request: APIRequestContext,
  title: string,
  poster: PosterFixture,
) {
  await expectRoute(page, "/admin/events", "活動設定");
  await page.getByRole("tab", { name: "建立活動" }).click();
  const form = eventForm(page, "建立活動");

  await form.getByRole("button", { name: "載入起始人資" }).click();
  await expect(page.getByText("起始資料已建立。")).toBeVisible();
  await form.getByLabel("活動名稱").fill(title);
  await form
    .getByLabel("描述")
    .fill(
      "Live E2E validates UI event creation, poster upload, booking, and waitlist.",
    );
  await form.getByLabel("容量").fill("1");
  await form.getByLabel("活動海報").setInputFiles(posterUploadFile(poster));
  await expect(form.getByText(poster.name)).toBeVisible();
  await form.getByRole("button", { name: "建立並發布" }).click();
  await expect(page.getByText("海報已上傳")).toBeVisible();

  const event = await findAdminEventByTitle(request, title);
  await expectPosterAPI(request, admin, event.event_id, poster);
  await openAdminEventEdit(page, event.title);
  await expectPosterRendered(
    page.locator(".admin-poster-frame").first(),
    poster,
    "admin edit after create",
  );
  return event;
}

export async function replacePosterAndUpdateEventThroughUi(
  page: Page,
  request: APIRequestContext,
  event: EventSummary,
  updatedTitle: string,
  poster: PosterFixture,
) {
  await openAdminEventEdit(page, event.title);
  const form = eventForm(page, "編輯活動");
  await form.getByLabel("活動海報").setInputFiles(posterUploadFile(poster));
  await expect(page.getByText("已上傳此海報。")).toBeVisible();
  await expectPosterAPI(request, admin, event.event_id, poster);
  await expectPosterRendered(
    page.locator(".admin-poster-frame").first(),
    poster,
    "admin edit after replacement upload",
  );

  await form.getByLabel("活動名稱").fill(updatedTitle);
  await form
    .getByLabel("描述")
    .fill("Updated by live E2E after replacing the event poster.");
  await form.getByRole("button", { name: "儲存活動" }).click();
  await expect(page.getByText("活動已更新並寫入稽核紀錄。")).toBeVisible();

  const updated = await findAdminEventByTitle(request, updatedTitle);
  await openAdminEventEdit(page, updated.title);
  await expectPosterRendered(
    page.locator(".admin-poster-frame").first(),
    poster,
    "admin edit after event update",
  );
  return updated;
}

export async function expectEmployeeBooksEventWithPoster(
  page: Page,
  event: EventSummary,
  poster: PosterFixture,
) {
  await expectRoute(page, "/user/events?mode=list", "活動探索");
  const card = employeeEventCard(page, event.title);
  await expect(card).toBeVisible();
  await expectPosterRendered(
    card.locator(".employee-event-poster").first(),
    poster,
    "employee event list before booking",
  );
  await card.getByRole("link", { name: new RegExp(event.title) }).click();

  await expect(mainHeading(page, "活動詳情")).toBeVisible();
  await expect(mainHeading(page, event.title)).toBeVisible();
  await expectPosterRendered(
    page.locator(".employee-event-detail-hero .employee-event-poster").first(),
    poster,
    "employee detail before booking",
  );
  await page.getByRole("button", { name: "立即報名" }).click();
  await expect(page.getByText("報名成功，票券已核發")).toBeVisible();
  await expect(page.getByRole("link", { name: "查看票券" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export async function expectEmployeeWaitlistsWithPoster(
  page: Page,
  event: EventSummary,
  poster: PosterFixture,
) {
  await expectRoute(page, "/user/events?mode=list", "活動探索");
  const card = employeeEventCard(page, event.title);
  await expect(card).toBeVisible();
  await expectPosterRendered(
    card.locator(".employee-event-poster").first(),
    poster,
    "employee event list before waitlist",
  );
  await card.getByRole("link", { name: new RegExp(event.title) }).click();

  await expect(mainHeading(page, "活動詳情")).toBeVisible();
  await expect(mainHeading(page, event.title)).toBeVisible();
  await expectPosterRendered(
    page.locator(".employee-event-detail-hero .employee-event-poster").first(),
    poster,
    "employee detail before waitlist",
  );
  await page.getByRole("button", { name: "加入候補" }).click();
  await expect(page.getByText("已加入候補")).toBeVisible();
  await expect(page.getByText("候補中").first()).toBeVisible();
  await expect(page.getByRole("link", { name: "查看票券" })).toHaveCount(0);
  await expectNoHorizontalOverflow(page);
}

export async function expectEmployeeTicketSurfacesShowPoster(
  page: Page,
  request: APIRequestContext,
  event: EventSummary,
  poster: PosterFixture,
) {
  await expectRoute(page, "/user/tickets", "我的票券");
  const ticket = await ticketForEvent(request, employee, event.event_id);
  const currentPass = page
    .locator(".employee-ticket-pass")
    .filter({ hasText: event.title })
    .first();
  if (await currentPass.isVisible({ timeout: 500 }).catch(() => false)) {
    await expectPosterRendered(
      currentPass.locator(".employee-event-poster").first(),
      poster,
      "employee ticket pass list",
    );
  }
  const ticketCard = page
    .locator(".employee-ticket-card")
    .filter({ hasText: event.title })
    .first();
  await expect(ticketCard).toBeVisible();
  await expectPosterRendered(
    ticketCard.locator(".employee-event-poster").first(),
    poster,
    "employee ticket timeline card",
  );
  await ticketCard.getByRole("link").click();
  await expect(page).toHaveURL(
    new RegExp(
      String.raw`/user/tickets\?ticket_id=${encodeURIComponent(ticket.ticket_id)}$`,
    ),
  );
  await expect(page.getByLabel("票券二維碼")).toBeVisible();
  await expect(page.getByText(ticket.signed_token || "")).toHaveCount(0);
  await expectPosterRendered(
    page.locator(".employee-ticket-pass .employee-event-poster").first(),
    poster,
    "employee ticket detail",
  );
}

export async function expectAdminRegistrationCounts(
  page: Page,
  request: APIRequestContext,
  event: EventSummary,
) {
  const rows = await api<RegistrationDetail[]>(
    request,
    admin,
    "GET",
    `/api/v1/admin/events/${encodeURIComponent(event.event_id)}/registrations`,
  );
  expect(rows.filter((row) => row.status === "confirmed")).toHaveLength(1);
  expect(rows.filter((row) => row.status === "waitlisted")).toHaveLength(1);

  await expectRoute(
    page,
    `/admin/registrations?event_id=${encodeURIComponent(event.event_id)}`,
    "報名治理",
  );
  await expect(page.getByText(event.title).first()).toBeVisible();
  await expect(page.getByText("已報名").first()).toBeVisible();
  await expect(page.getByText("候補").first()).toBeVisible();
  await page.getByRole("tab", { name: "候補名單" }).click();
  await expect(page.locator("main").getByText(secondEmployee.id)).toBeVisible();
  await page.getByRole("tab", { name: "報名名單" }).click();
  await expect(page.locator("main").getByText(employee.id)).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export async function expectEmployeeNotifications(
  page: Page,
  event: EventSummary,
) {
  await expectRoute(page, "/user/notifications", "通知中心");
  await expectNotificationEntry(page, event.title, "報名");
}

export async function expectOnlineCheckinAndDuplicate(
  page: Page,
  event: EventSummary,
  token: string,
) {
  await expectRoute(page, "/admin/checkin", "現場驗票");
  await expect(
    page.locator('[aria-label="目前驗票活動"]').getByText(event.title),
  ).toBeVisible();
  await page.getByLabel(/掃描或貼上票券簽章碼/).fill(token);
  await page.getByRole("button", { name: "送出驗票" }).click();
  await expect(mainHeading(page, "驗票成功")).toBeVisible();

  await page.getByRole("button", { name: "送出驗票" }).click();
  await expect(mainHeading(page, "重複掃描被拒絕")).toBeVisible();
  await expect(page.getByText("重複掃描").first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export async function expectOfflineCheckinRoute(page: Page) {
  await expectRoute(page, "/admin/checkin/offline", "離線驗票同步");
  await expect(
    page.getByRole("button", { name: "下載離線名單" }),
  ).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export async function expectReportsExportAndAuditFilters(
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
  await openAuditExportDetail(page);
  await expect(page.getByText("report_type").first()).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export async function expectNotificationDeliveryRoute(
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

export async function expectRoleRouteSmoke(page: Page) {
  await expectRoute(page, "/admin/events", "活動設定");
  await expectRoute(page, "/admin/registrations", "報名治理");
  await expectRoute(page, "/admin/notifications", "通知投遞");
  await expectRoute(page, "/admin/flow-check", "流程檢查");
}

export async function expectHrRouteSmoke(page: Page) {
  await expectRoute(page, "/admin/hr-settings", "人資同步設定");
  await expectOptionalRoute(page, "/admin/ops", "營運監控");
}

export async function expectSystemRouteSmoke(page: Page) {
  await expectOptionalRoute(page, "/admin/ops", "營運監控");
  await expectRoute(page, "/admin/hr-settings", "人資同步設定");
}

async function findAdminEventByTitle(
  request: APIRequestContext,
  title: string,
) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const events = await api<EventSummary[]>(
      request,
      admin,
      "GET",
      "/api/v1/admin/events",
    );
    const event = events.find((candidate) => candidate.title === title);
    if (event) return event;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`admin event not found: ${title}`);
}

async function openAdminEventEdit(page: Page, title: string) {
  await expectRoute(page, "/admin/events?tab=list", "活動設定");
  const row = page.locator("article.event-row").filter({ hasText: title });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "編輯" }).click();
  await expect(eventForm(page, "編輯活動")).toBeVisible();
}

function eventForm(page: Page, heading: string) {
  return page.locator("form").filter({ hasText: heading }).first();
}

function employeeEventCard(page: Page, title: string) {
  return page.locator("article.employee-event-card").filter({ hasText: title });
}

async function ticketForEvent(
  request: APIRequestContext,
  actor: typeof employee,
  eventID: string,
) {
  const tickets = await api<Ticket[]>(
    request,
    actor,
    "GET",
    "/api/v1/me/tickets",
  );
  const ticket = tickets.find((candidate) => candidate.event_id === eventID);
  expect(ticket?.ticket_id, `ticket for ${eventID}`).toBeTruthy();
  expect(ticket?.signed_token, `ticket token for ${eventID}`).toBeTruthy();
  return ticket as Ticket;
}

async function expectOptionalRoute(page: Page, path: string, heading: string) {
  await expectRoute(page, path, heading).catch(async () => {
    await expect(page.getByRole("heading", { name: "權限不足" })).toBeVisible();
    await expectNoHorizontalOverflow(page);
  });
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

async function openAuditExportDetail(page: Page) {
  const auditRow = page
    .locator("main tr.interactive-row")
    .filter({ hasText: "報表匯出請求" })
    .filter({ hasText: "人資管理員" })
    .first();
  if (await auditRow.isVisible({ timeout: 1_000 }).catch(() => false)) {
    await auditRow.click();
    return;
  }
  const auditCard = page
    .locator(".audit-mobile-card")
    .filter({ hasText: "報表匯出請求" })
    .filter({ hasText: "人資管理員" })
    .first();
  await expect(auditCard).toBeVisible();
  await auditCard.getByRole("button", { name: "檢視稽核明細" }).click();
}

async function expectDeliveryEntry(
  page: Page,
  deliveryID: string,
  statusLabel: string,
) {
  const mobileCard = page
    .locator("main .mobile-summary-card")
    .filter({ hasText: compactRecordID(deliveryID) })
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

function compactRecordID(value: string) {
  // Must match compactIdentifier() in features/notifications/pages.tsx so the
  // live assertion searches for the same truncated text the UI renders.
  return value.length <= 22 ? value : `${value.slice(0, 8)}…${value.slice(-6)}`;
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
