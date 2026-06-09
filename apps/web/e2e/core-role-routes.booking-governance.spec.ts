import { expect, test } from "@playwright/test";
import { expectNoHorizontalOverflow } from "./core-role-routes.assertions";
import {
  sampleEvent,
  sampleTickets,
  type EventFixture,
} from "./core-role-routes.fixtures";
import { ensureSessionRoutes, loginAs } from "./core-role-routes.mocks";

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
  await page.goto("/user/events", {
    waitUntil: "domcontentloaded",
  });
  await page.goto("/user/events/detail?event_id=evt-cets-001", {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL(
    /\/user\/events\/detail\?event_id=evt-cets-001$/,
  );

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

test("admin governance actions keep selected reason feedback", async ({
  page,
}) => {
  const requestBodies = {
    cancel: [] as Record<string, unknown>[],
    revoke: [] as Record<string, unknown>[],
  };
  await ensureSessionRoutes(page, "admin-1");
  page.on("request", (request) => {
    const pathName = new URL(request.url()).pathname;
    const body = JSON.parse(request.postData() || "{}") as Record<
      string,
      unknown
    >;
    if (
      /\/api\/v1\/admin\/events\/[^/]+\/registrations\/[^/]+\/cancel$/.test(
        pathName,
      )
    ) {
      requestBodies.cancel.push(body);
    }
    if (/\/api\/v1\/admin\/tickets\/[^/]+\/revoke$/.test(pathName)) {
      requestBodies.revoke.push(body);
    }
  });
  await loginAs(page, "admin-1");
  await page.goto("/admin/registrations", { waitUntil: "domcontentloaded" });

  await page.getByRole("button", { name: "取消" }).click();
  await page.getByRole("combobox", { name: "處置原因" }).click();
  await page.getByRole("option", { name: "主管要求" }).click();
  await page.getByRole("button", { name: "確認取消報名" }).click();
  await expect(page.getByText("報名已取消。")).toBeVisible();
  expect(requestBodies.cancel).toEqual([
    { reason: "manager request", idempotency_key: "cancel-reg-001" },
  ]);

  await page.getByRole("tab", { name: "票券狀態" }).click();
  await page.getByRole("button", { name: "撤銷票券" }).click();
  await page.getByRole("combobox", { name: "處置原因" }).click();
  await page.getByRole("option", { name: "安全審核" }).click();
  await page.getByRole("button", { name: "確認撤銷" }).click();
  await expect(page.getByText("票券已撤銷。")).toBeVisible();
  expect(requestBodies.revoke).toEqual([{ reason: "security review" }]);
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
