import { expect, test } from "@playwright/test";
import {
  expectNoHorizontalOverflow,
  expectNotificationControlsCompact,
  expectPrimaryCtaTreatment,
} from "./core-role-routes.assertions";
import {
  forbiddenRouteCases,
  roleCases,
  sampleEvent,
  sampleTickets,
  type EventFixture,
} from "./core-role-routes.fixtures";
import { ensureSessionRoutes, loginAs } from "./core-role-routes.mocks";

for (const roleCase of roleCases) {
  for (const route of roleCase.routes) {
    test.describe(`${roleCase.name}: ${route.path}`, () => {
      test(`can load route and stay horizontally contained`, async ({
        page,
      }) => {
        const consoleErrors: string[] = [];
        page.on("console", (message) => {
          if (message.type() === "error") consoleErrors.push(message.text());
        });
        await ensureSessionRoutes(page, roleCase.principalID);
        await loginAs(page, roleCase.principalID);
        await page.goto(route.path, { waitUntil: "domcontentloaded" });
        await expect(
          page
            .getByRole("heading", { name: route.heading, exact: true })
            .first(),
        ).toBeVisible();
        await expectNoHorizontalOverflow(page);
        await expect(
          page.locator(".desktop-sidebar .workspace-switch"),
        ).toHaveCount(0);
        if (route.path === "/admin/notifications") {
          await expectNotificationControlsCompact(page);
        }
        expect(consoleErrors).toEqual([]);
      });
    });
  }
}

test("shows explicit unauthorized state for forbidden deep links", async ({
  page,
}) => {
  await ensureSessionRoutes(page, "staff-1");
  await loginAs(page, "staff-1");
  await page.goto("/admin/events", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("heading", { name: "權限不足" })).toBeVisible();
  await expect(page.getByText("目前登入角色無法進入")).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

for (const routeCase of forbiddenRouteCases) {
  test(`${routeCase.name}`, async ({ page }) => {
    await ensureSessionRoutes(page, routeCase.principalID);
    await loginAs(page, routeCase.principalID);
    await page.goto(routeCase.path, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { name: "權限不足" })).toBeVisible();
    await expect(
      page.locator(".desktop-sidebar .workspace-switch"),
    ).toHaveCount(0);
    await expectNoHorizontalOverflow(page);
  });
}

test("employee tickets open exact detail only after list click", async ({
  page,
}) => {
  await ensureSessionRoutes(page, "E1001");
  await loginAs(page, "E1001");
  await page.goto("/user/tickets", { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("heading", { name: "票券清單" })).toBeVisible();
  await expect(page.getByLabel("票券二維碼")).toHaveCount(0);

  await page.getByRole("link", { name: /第一階段企業午餐日/ }).click();
  await expect(page).toHaveURL(/\/user\/tickets\?ticket_id=ticket-001$/);
  await expect(page.getByRole("heading", { name: "票券詳細" })).toBeVisible();
  await expect(page.getByLabel("票券二維碼")).toBeVisible();
  await expect(page.getByText("mocked-token")).toHaveCount(0);
  await expect(page.getByText("mocked-qr-token")).toHaveCount(0);
  await expectNoHorizontalOverflow(page);
});

test("employee booking CTAs keep primary visual treatment", async ({
  page,
}) => {
  const bookableEvent: EventFixture = {
    ...sampleEvent,
    event_id: "evt-bookable",
    title: "可直接報名活動",
    current_user_status: undefined,
    current_user_ticket: undefined,
    registration_close: "2099-01-09T23:00:00Z",
    confirmed_count: 4,
    waitlist_count: 0,
    remaining_capacity: 8,
  };
  const waitlistEvent: EventFixture = {
    ...bookableEvent,
    event_id: "evt-waitlist",
    title: "候補活動",
    capacity: 4,
    confirmed_count: 4,
    waitlist_count: 2,
    remaining_capacity: 0,
  };

  await ensureSessionRoutes(page, "E1001", {
    eventDetail: bookableEvent,
    events: [bookableEvent, waitlistEvent],
  });
  await loginAs(page, "E1001");
  await page.goto("/user/events", { waitUntil: "domcontentloaded" });

  await expectPrimaryCtaTreatment(page.getByRole("link", { name: /立即報名/ }));
  await expectPrimaryCtaTreatment(page.getByRole("link", { name: /加入候補/ }));

  await page.goto("/user/events/detail?event_id=evt-waitlist", {
    waitUntil: "domcontentloaded",
  });
  await expectPrimaryCtaTreatment(
    page.getByRole("button", { name: /加入候補/ }),
  );
  await expectNoHorizontalOverflow(page);
});

test("employee ticket detail missing state is recoverable", async ({
  page,
}) => {
  await ensureSessionRoutes(page, "E1001");
  await loginAs(page, "E1001");
  await page.goto("/user/tickets?ticket_id=missing", {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByText("找不到票券。")).toBeVisible();
  await expect(page.getByLabel("票券二維碼")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "返回我的票券" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("employee cancellation requires confirmation before API call", async ({
  page,
}) => {
  let cancelRequests = 0;
  const cancellableEvent: EventFixture = {
    ...sampleEvent,
    registration_close: "2099-01-09T23:00:00Z",
  };
  await ensureSessionRoutes(page, "E1001", {
    eventDetail: cancellableEvent,
    events: [cancellableEvent],
  });
  page.on("request", (request) => {
    const pathName = new URL(request.url()).pathname;
    if (/\/api\/v1\/me\/registrations\/[^/]+\/cancel$/.test(pathName)) {
      cancelRequests += 1;
    }
  });
  await loginAs(page, "E1001");
  await page.goto("/user/events?tab=registered", {
    waitUntil: "domcontentloaded",
  });

  await page.getByRole("button", { name: "取消報名" }).click();
  const dialog = page.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("第一階段企業午餐日")).toBeVisible();
  await expect(dialog.getByText(/已核發票券會同步失效/)).toBeVisible();
  expect(cancelRequests).toBe(0);

  await page.getByRole("button", { name: "保留報名" }).click();
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  expect(cancelRequests).toBe(0);

  await page.getByRole("button", { name: "取消報名" }).click();
  await page.getByRole("button", { name: "確認取消報名" }).click();
  await expect(page.getByText("報名已取消").first()).toBeVisible();
  expect(cancelRequests).toBe(1);
  await expectNoHorizontalOverflow(page);
});

test("employee duplicate booking response keeps existing ticket handoff", async ({
  page,
}) => {
  let bookingRequests = 0;
  const availableEvent: EventFixture = {
    ...sampleEvent,
    current_user_status: undefined,
    current_user_ticket: undefined,
    registration_close: "2099-01-09T23:00:00Z",
    remaining_capacity: 3,
  };
  await ensureSessionRoutes(page, "E1001", {
    bookingResponse: {
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
      duplicate: true,
    },
    eventDetail: availableEvent,
    events: [availableEvent],
  });
  page.on("request", (request) => {
    const pathName = new URL(request.url()).pathname;
    if (/\/api\/v1\/events\/[^/]+\/bookings$/.test(pathName)) {
      bookingRequests += 1;
    }
  });
  await loginAs(page, "E1001");
  await page.goto("/user/events/detail?event_id=evt-cets-001", {
    waitUntil: "domcontentloaded",
  });

  await page.getByRole("button", { name: "立即報名" }).click();
  await expect(page.getByText("你已經報名此活動")).toBeVisible();
  await expect(page.getByText(/未建立新的報名/)).toBeVisible();
  expect(bookingRequests).toBe(1);

  await page.getByRole("link", { name: "查看票券" }).click();
  await expect(page).toHaveURL(/\/user\/tickets\?ticket_id=ticket-001$/);
  await expectNoHorizontalOverflow(page);
});

test("mobile route and utility sheets expose overflow navigation and debug tools", async ({
  page,
}) => {
  if ((page.viewportSize()?.width ?? 0) > 900) return;

  await ensureSessionRoutes(page, "hr-1", { mockProfiles: true });
  await page.goto("/admin/reports?debug=1", { waitUntil: "domcontentloaded" });
  await expect(
    page.getByRole("heading", { name: "人資報表", exact: true }).first(),
  ).toBeVisible();

  const moreButton = page.getByRole("button", { name: "更多頁面" });
  if ((await moreButton.count()) > 0) {
    await moreButton.click();
    await expect(page.getByRole("heading", { name: "更多頁面" })).toBeVisible();
    await expect(page.getByRole("link", { name: /稽核查詢/ })).toBeVisible();
    await page.keyboard.press("Escape");
  } else {
    await expect(page.getByRole("link", { name: /稽核/ })).toBeVisible();
  }
  await page.getByRole("button", { name: "開啟工具" }).click();
  await expect(page.getByRole("heading", { name: "工作區工具" })).toBeVisible();
  await expect(
    page.locator(".mobile-utility-sheet .api-panel.sheet"),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "切換身分" })).toBeVisible();
});

test("keeps the demo runbook available for mock provider profiles", async ({
  page,
}) => {
  await ensureSessionRoutes(page, "admin-1", { mockProfiles: true });
  await page.goto("/admin/flow-check?debug=1", {
    waitUntil: "domcontentloaded",
  });
  await expect(
    page.getByRole("heading", { name: "流程檢查", level: 1 }).first(),
  ).toBeVisible();
  await page
    .locator(".content-grid")
    .getByRole("button", { name: "執行流程檢查" })
    .click();
  await expect(
    page.getByRole("heading", { name: "流程檢查", level: 1 }).first(),
  ).toBeVisible();
  await expect(
    page
      .locator(".content-grid")
      .getByRole("button", { name: /執行流程檢查|執行中/ }),
  ).toBeVisible();
  await expectNoHorizontalOverflow(page);
});
