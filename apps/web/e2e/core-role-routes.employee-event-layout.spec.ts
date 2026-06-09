import { expect, test, type Locator, type Page } from "@playwright/test";
import { expectNoHorizontalOverflow } from "./core-role-routes.assertions";
import { sampleEvent, type EventFixture } from "./core-role-routes.fixtures";
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

async function requiredBox(locator: Locator, label: string) {
  const box = await locator.boundingBox();
  if (!box) throw new Error(`${label} layout bounds are unavailable`);
  return box;
}

test("employee event list cards align poster and footer rows", async ({
  page,
}) => {
  const shortEvent: EventFixture = {
    ...sampleEvent,
    current_user_status: undefined,
    current_user_ticket: undefined,
    description: "短講與午餐交流。",
    ends_at: "2026-06-04T12:00:00+08:00",
    event_id: "evt-card-align-short",
    registration_close: "2099-01-09T23:00:00Z",
    starts_at: "2026-06-04T10:00:00+08:00",
    title: "午餐交流",
  };
  const longEvent: EventFixture = {
    ...shortEvent,
    capacity: 1,
    confirmed_count: 1,
    description:
      "這是一場跨部門工作坊，包含報到說明、主持人開場、分組討論與後續行動整理，用來驗證清單卡片在較長內容下仍保持同列對齊。",
    event_id: "evt-card-align-long",
    remaining_capacity: 0,
    title: "跨部門流程改善與活動營運協作工作坊",
    waitlist_count: 3,
  };
  const registeredEvent: EventFixture = {
    ...shortEvent,
    current_user_status: "waitlisted",
    event_id: "evt-card-align-waitlist",
    title: "候補流程說明會",
    waitlist_count: 8,
  };

  await openRoute(
    page,
    "E1001",
    "/user/events?mode=list&view=week&date=2026-06-04",
    {
      events: [shortEvent, longEvent, registeredEvent],
    },
  );

  const cards = page.locator(".employee-event-card");
  await expect(cards).toHaveCount(3);
  await expectNoHorizontalOverflow(page);

  const firstCard = await requiredBox(cards.nth(0), "first event card");
  const secondCard = await requiredBox(cards.nth(1), "second event card");

  if ((page.viewportSize()?.width ?? 0) <= 900) {
    expect(
      firstCard.y + firstCard.height,
      "mobile cards stack without overlap",
    ).toBeLessThanOrEqual(secondCard.y);
    expect(
      Math.abs(firstCard.x - secondCard.x),
      "mobile cards share the same column",
    ).toBeLessThanOrEqual(2);
    return;
  }

  const firstRowIndexes: number[] = [];
  for (const index of [0, 1, 2]) {
    const box = await cards.nth(index).boundingBox();
    if (box && Math.abs(box.y - firstCard.y) <= 2) {
      firstRowIndexes.push(index);
    }
  }
  if (firstRowIndexes.length < 2) {
    expect(
      firstCard.y + firstCard.height,
      "narrow desktop cards stack without overlap",
    ).toBeLessThanOrEqual(secondCard.y);
    return;
  }
  const rowIndexes = firstRowIndexes;

  const rowCardBoxes = await Promise.all(
    rowIndexes.map((index) =>
      requiredBox(cards.nth(index), `event card ${index + 1}`),
    ),
  );
  const rowPosterBoxes = await Promise.all(
    rowIndexes.map((index) =>
      requiredBox(
        cards.nth(index).locator(".employee-event-poster"),
        `event card ${index + 1} poster`,
      ),
    ),
  );
  const rowActionBoxes = await Promise.all(
    rowIndexes.map((index) =>
      requiredBox(
        cards.nth(index).locator(".employee-event-primary-action"),
        `event card ${index + 1} primary action`,
      ),
    ),
  );

  for (let index = 1; index < rowIndexes.length; index += 1) {
    expect(
      Math.abs(rowCardBoxes[index].y - rowCardBoxes[0].y),
      "cards in the same desktop row share a top edge",
    ).toBeLessThanOrEqual(2);
    expect(
      Math.abs(rowPosterBoxes[index].y - rowPosterBoxes[0].y),
      "posters in the same desktop row share a top edge",
    ).toBeLessThanOrEqual(2);
    expect(
      Math.abs(
        rowActionBoxes[index].y +
          rowActionBoxes[index].height -
          (rowActionBoxes[0].y + rowActionBoxes[0].height),
      ),
      "primary actions in the same desktop row share a baseline",
    ).toBeLessThanOrEqual(2);
  }
});
