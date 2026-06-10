import { expect, test } from "@playwright/test";
import { execFile } from "node:child_process";
import { randomUUID } from "node:crypto";
import { dirname, resolve } from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import {
  collectBrowserErrors,
  employee,
  expectForbiddenRoute,
  expectRoute,
  hr,
  loginThroughUi,
  secondEmployee,
  signedTokenFor,
  signedTokenForEventTitle,
  staff,
  waitForNotificationDelivery,
} from "./support/live-flow-helpers";
import {
  createEventWithPosterThroughUi,
  expectAdminRegistrationCounts,
  expectEmployeeBooksEventWithPoster,
  expectEmployeeNotifications,
  expectEmployeeTicketSurfacesShowPoster,
  expectEmployeeWaitlistsWithPoster,
  expectHrRouteSmoke,
  expectNotificationDeliveryRoute,
  expectOfflineCheckinRoute,
  expectOnlineCheckinAndDuplicate,
  expectReportsExportAndAuditFilters,
  expectRoleRouteSmoke,
  expectSystemRouteSmoke,
  replacePosterAndUpdateEventThroughUi,
} from "./support/phase1-live-flow-assertions";
import { createPosterFixture } from "./support/poster-fixtures";

const execFileAsync = promisify(execFile);
const currentDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(currentDir, "../../..");

test.describe.serial("第一階段實際產品流程", () => {
  test.beforeEach(async () => {
    await resetDemoDB();
  });

  test("透過真實介面驗證海報、活動建立、搶票與候補", async ({
    page,
    request,
  }) => {
    const browserErrors = collectBrowserErrors(page);
    const suffix = `${Date.now()}-${randomUUID()}`;
    const initialTitle = `實際介面驗證活動 ${suffix}`;
    const updatedTitle = `實際介面驗證活動更新 ${suffix}`;
    const posterA = createPosterFixture(
      `poster-a-${suffix}.png`,
      [42, 105, 236],
    );
    const posterB = createPosterFixture(
      `poster-b-${suffix}.png`,
      [18, 184, 104],
    );

    await loginThroughUi(page, request, "admin-1", "活動設定");
    const createdEvent = await createEventWithPosterThroughUi(
      page,
      request,
      initialTitle,
      posterA,
    );
    const event = await replacePosterAndUpdateEventThroughUi(
      page,
      request,
      createdEvent,
      updatedTitle,
      posterB,
    );
    await expectRoleRouteSmoke(page);

    await loginThroughUi(page, request, employee.id, "活動探索");
    await expectEmployeeBooksEventWithPoster(page, event, posterB);
    await expectEmployeeTicketSurfacesShowPoster(page, request, event, posterB);
    await expectEmployeeNotifications(page, event);
    await signedTokenFor(request, employee, event.event_id);
    const demoCheckin = await signedTokenForEventTitle(
      request,
      employee,
      "Demo Check-in Today",
    );

    await loginThroughUi(page, request, secondEmployee.id, "活動探索");
    await expectEmployeeWaitlistsWithPoster(page, event, posterB);
    await expectEmployeeNotifications(page, event);

    await loginThroughUi(page, request, "admin-1", "活動設定");
    await expectAdminRegistrationCounts(page, request, event);

    await loginThroughUi(page, request, staff.id, "現場驗票");
    await expectOnlineCheckinAndDuplicate(
      page,
      demoCheckin.event,
      demoCheckin.token,
    );
    await expectOfflineCheckinRoute(page);
    await expectForbiddenRoute(page, "/admin/events");

    await loginThroughUi(page, request, hr.id, "人資報表");
    await expectReportsExportAndAuditFilters(page, event);
    await expectHrRouteSmoke(page);
    const delivery = await waitForNotificationDelivery(page, request);

    await loginThroughUi(page, request, "system-1", "人資報表");
    await expectNotificationDeliveryRoute(page, delivery);
    await expectRoute(page, "/admin/audit", "稽核查詢");
    await expectSystemRouteSmoke(page);
    await expectForbiddenRoute(page, "/admin/events");

    expect(browserErrors).toEqual([]);
  });
});

async function resetDemoDB() {
  await execFileAsync(
    resolve(repoRoot, "infra/k8s/baremetal/scripts/72-reset-demo-db.sh"),
    [],
    {
      cwd: repoRoot,
      env: {
        ...process.env,
        APPLY: "true",
      },
      maxBuffer: 1024 * 1024,
    },
  );
}
