import { expect, test, type Page } from "@playwright/test";
import {
  expectElementAboveMobileTabbar,
  expectNoHorizontalOverflow,
  expectNotificationControlsCompact,
  expectPrimaryCtaTreatment,
} from "./core-role-routes.assertions";
import {
  forbiddenRouteCases,
  roleCases,
  sampleEvent,
  type EventFixture,
} from "./core-role-routes.fixtures";
import { ensureSessionRoutes, loginAs } from "./core-role-routes.mocks";

async function openRoute(
  page: Page,
  principalID: Parameters<typeof ensureSessionRoutes>[1],
  path: string,
  options?: Parameters<typeof ensureSessionRoutes>[2],
) {
  await ensureSessionRoutes(page, principalID, options);
  await loginAs(page, principalID);
  await page.goto(path, { waitUntil: "domcontentloaded" });
}

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
        await openRoute(page, roleCase.principalID, route.path);
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
  await openRoute(page, "staff-1", "/admin/events");
  await expect(page.getByRole("heading", { name: "權限不足" })).toBeVisible();
  await expect(page.getByText("目前登入角色無法進入")).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

for (const routeCase of forbiddenRouteCases) {
  test(`${routeCase.name}`, async ({ page }) => {
    await openRoute(page, routeCase.principalID, routeCase.path);
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
  await openRoute(page, "E1001", "/user/tickets");

  await expect(
    page.getByRole("heading", { level: 2, name: "我的票券" }),
  ).toBeVisible();
  await expect(page.getByLabel("目前可入場票券")).toBeVisible();
  await expect(page.getByLabel("票券二維碼").first()).toBeVisible();

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

  await openRoute(page, "E1001", "/user/events?view=week&date=2026-06-04", {
    eventDetail: bookableEvent,
    events: [bookableEvent, waitlistEvent],
  });

  await expectPrimaryCtaTreatment(
    page.getByRole("link", { name: "報名活動：可直接報名活動" }),
  );
  await expectPrimaryCtaTreatment(
    page.getByRole("link", { name: "查看詳情：候補活動" }),
  );

  await page.goto("/user/events/detail?event_id=evt-waitlist", {
    waitUntil: "domcontentloaded",
  });
  await expectPrimaryCtaTreatment(
    page.getByRole("button", { name: /加入候補/ }),
  );
  await expectNoHorizontalOverflow(page);
});

test("employee event discovery search stays compact and keyboard accessible", async ({
  page,
}) => {
  const lunchEvent: EventFixture = {
    ...sampleEvent,
    current_user_status: undefined,
    current_user_ticket: undefined,
    event_id: "evt-search-lunch",
    registration_close: "2099-01-09T23:00:00Z",
    title: "企業午餐交流",
  };
  const otherEvent: EventFixture = {
    ...lunchEvent,
    event_id: "evt-search-training",
    tags: ["training"],
    title: "技術訓練工作坊",
  };

  await openRoute(
    page,
    "E1001",
    "/user/events?mode=list&view=week&date=2026-06-04",
    {
      events: [lunchEvent, otherEvent],
    },
  );

  const search = page.getByRole("searchbox", { name: "搜尋活動" });
  await expect(search).toBeVisible();
  await expect(page.getByRole("tab", { name: "活動列表" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await search.focus();
  await expect(search).toBeFocused();
  await search.fill("午餐");

  await expect(page).toHaveURL(/q=%E5%8D%88%E9%A4%90/);
  await expect(page.getByText("符合 1 場活動")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "報名活動：企業午餐交流" }),
  ).toBeVisible();
  await expect(page.getByText("技術訓練工作坊")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /清除/ })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("employee calendar tab stays date-first without search controls", async ({
  page,
}) => {
  await openRoute(page, "E1001", "/user/events?view=week&date=2026-06-04");

  await expect(page.getByRole("tab", { name: "日曆" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByLabel("週行事曆")).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "搜尋活動" })).toHaveCount(
    0,
  );
  await expect(page.getByRole("button", { name: "更新" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("employee event agenda keeps a single desktop card at reusable width", async ({
  page,
}) => {
  if ((page.viewportSize()?.width ?? 0) <= 900) return;

  const singleEvent: EventFixture = {
    ...sampleEvent,
    event_id: "evt-single-card",
    title: "單一活動桌面比例檢查",
    starts_at: "2026-06-04T10:00:00+08:00",
    registration_close: "2099-01-09T23:00:00Z",
    current_user_status: undefined,
    current_user_ticket: undefined,
  };

  await openRoute(page, "E1001", "/user/events?view=day&date=2026-06-04", {
    events: [singleEvent],
  });

  const cardList = page.locator(".employee-event-card-list").first();
  const eventCard = page.locator(".employee-event-card").first();
  await expect(eventCard).toBeVisible();
  await expect(page.locator(".employee-event-card")).toHaveCount(1);

  const cardBox = await eventCard.boundingBox();
  const listBox = await cardList.boundingBox();
  if (!cardBox || !listBox) {
    throw new Error("employee event card layout bounds are unavailable");
  }
  expect(
    cardBox.width,
    "single event card keeps desktop card width",
  ).toBeLessThanOrEqual(380);
  expect(
    listBox.width - cardBox.width,
    "single event card does not stretch across the agenda",
  ).toBeGreaterThan(120);
  await expectNoHorizontalOverflow(page);
});

test("employee ticket detail missing state is recoverable", async ({
  page,
}) => {
  await openRoute(page, "E1001", "/user/tickets?ticket_id=missing");

  await expect(page.getByText("找不到票券。")).toBeVisible();
  await expect(page.getByLabel("票券二維碼")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "返回我的票券" })).toBeVisible();
  await expect(
    page.getByRole("link", { exact: true, name: "回我的票券" }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "瀏覽活動" })).toBeVisible();
  await expect(page.getByRole("button", { name: "重新整理" })).toBeVisible();
  await expectNoHorizontalOverflow(page);
});

test("mobile check-in result stays clear of bottom navigation", async ({
  page,
}) => {
  if ((page.viewportSize()?.width ?? 0) > 900) return;

  await openRoute(page, "staff-1", "/admin/checkin");
  await page.getByLabel("掃描或貼上票券簽章碼").fill("mocked-token");
  await page.getByRole("button", { name: "送出驗票" }).click();

  await expect(page.getByRole("heading", { name: "驗票成功" })).toBeVisible();
  await expectElementAboveMobileTabbar(
    page,
    ".checkin-result-panel.has-result",
  );
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
  await expect(
    page.locator(".content-grid").getByRole("heading", {
      name: "Demo 控制台",
    }),
  ).toBeVisible();
  await expect(
    page.locator(".demo-step-item").filter({ hasText: "載入 demo 員工" }),
  ).toBeVisible();
  await expect(page.locator(".demo-step-item")).toHaveCount(11);
  await expectNoHorizontalOverflow(page);
});
