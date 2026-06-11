import { type APIRequestContext, expect, type Page } from "@playwright/test";

export type Role =
  | "employee"
  | "activity_admin"
  | "checkin_staff"
  | "hr_admin"
  | "system_admin";
export type Actor = { id: string; role: Role };
export type Envelope<T> = { success: boolean; data: T; error: string | null };
export type MockProviderToken = { provider_token: string; expires_at: string };
export type EventSummary = { event_id: string; title: string };
export type RegistrationDetail = {
  registration_id: string;
  event_id: string;
  employee_id: string;
  status: string;
  ticket?: Ticket | null;
};
export type Ticket = {
  ticket_id: string;
  event_id: string;
  employee_id: string;
  status: string;
  signed_token?: string;
  event_title?: string;
};
export type NotificationDelivery = { delivery_id: string; status: string };

export const admin: Actor = { id: "admin-1", role: "activity_admin" };
export const employee: Actor = { id: "E1001", role: "employee" };
export const secondEmployee: Actor = { id: "E1002", role: "employee" };
export const staff: Actor = { id: "staff-1", role: "checkin_staff" };
export const hr: Actor = { id: "hr-1", role: "hr_admin" };

const providerTokens = new Map<string, string>();
let activeBrowserProviderToken = "";

export function collectBrowserErrors(page: Page) {
  const errors: string[] = [];
  page.on("console", (message) => {
    const text = message.text();
    if (message.type() !== "error") return;
    if (/status of (401|403|404|409)/.test(text)) return;
    if (/Failed to load resource:.*404/.test(text)) return;
    if (/SSL certificate error/.test(text)) return;
    errors.push(text);
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return errors;
}

export async function loginThroughUi(
  page: Page,
  request: APIRequestContext,
  principalID: string,
  landingHeading: string,
) {
  const providerToken = await providerTokenFor(request, principalID);
  activeBrowserProviderToken = providerToken;
  await prepareProviderForNavigation(page);
  await page.goto("/?debug=1", { waitUntil: "networkidle" });
  await syncBrowserProviderToken(page);
  const loginHeading = page.getByRole("heading", { name: "選擇一個本機身分" });
  if (await loginHeading.isVisible({ timeout: 1_000 }).catch(() => false)) {
    await page.getByRole("button", { name: new RegExp(principalID) }).click();
  }
  await syncBrowserProviderToken(page);
  await expect(
    page.locator("main").getByRole("heading", { name: landingHeading }).first(),
  ).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export async function expectForbiddenRoute(page: Page, path: string) {
  await prepareProviderForNavigation(page);
  await page.goto(path, { waitUntil: "networkidle" });
  await syncBrowserProviderToken(page);
  await expect(page.getByRole("heading", { name: "權限不足" })).toBeVisible();
  await expect(page.getByText("目前登入角色無法進入")).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export async function expectRoute(page: Page, path: string, heading: string) {
  await prepareProviderForNavigation(page);
  await page.goto(path, { waitUntil: "networkidle" });
  await syncBrowserProviderToken(page);
  await expect(mainHeading(page, heading)).toBeVisible();
  await expectNoHorizontalOverflow(page);
}

export function mainHeading(page: Page, name: string | RegExp) {
  return page.locator("main").getByRole("heading", { name }).first();
}

export async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() =>
    Math.ceil(document.documentElement.scrollWidth - window.innerWidth),
  );
  expect(overflow, "no horizontal overflow").toBeLessThanOrEqual(1);
}

export async function chooseComboboxOption(
  page: Page,
  label: string,
  optionLabel: string,
) {
  await page.getByRole("combobox", { name: label }).click();
  await page
    .getByRole("option", { name: new RegExp(escapeRegExp(optionLabel)) })
    .click();
}

export async function signedTokenFor(
  request: APIRequestContext,
  actor: Actor,
  eventID: string,
) {
  const tickets = await api<Ticket[]>(
    request,
    actor,
    "GET",
    "/api/v1/me/tickets",
  );
  const ticket = tickets.find((candidate) => candidate.event_id === eventID);
  expect(
    ticket?.signed_token,
    `ticket token for ${actor.id} on ${eventID}`,
  ).toBeTruthy();
  return ticket?.signed_token || "";
}

export async function signedTokenForEventTitle(
  request: APIRequestContext,
  actor: Actor,
  eventTitle: string,
) {
  const tickets = await api<Ticket[]>(
    request,
    actor,
    "GET",
    "/api/v1/me/tickets",
  );
  const ticket = tickets.find(
    (candidate) => candidate.event_title === eventTitle,
  );
  expect(ticket?.event_id, `ticket event id for ${eventTitle}`).toBeTruthy();
  expect(ticket?.signed_token, `ticket token for ${eventTitle}`).toBeTruthy();
  return {
    event: {
      event_id: ticket?.event_id || "",
      title: ticket?.event_title || eventTitle,
    },
    token: ticket?.signed_token || "",
  };
}

export async function waitForNotificationDelivery(
  page: Page,
  request: APIRequestContext,
) {
  for (let attempt = 0; attempt < 30; attempt++) {
    const deliveries = await api<NotificationDelivery[]>(
      request,
      hr,
      "GET",
      "/api/v1/admin/notifications/deliveries",
    );
    const visible =
      deliveries.find(
        (delivery) =>
          delivery.status !== "failed" && delivery.status !== "dead_letter",
      ) || deliveries[0];
    if (visible) return visible;
    await page.waitForTimeout(1_000);
  }
  throw new Error(
    "notification delivery worker did not create a visible delivery",
  );
}

export async function api<T>(
  request: APIRequestContext,
  actor: Actor,
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const response = await rawApi(request, actor, method, path, body);
  const text = await response.text();
  expect(response.ok(), `${method} ${path} failed: ${text}`).toBe(true);
  const payload = JSON.parse(text) as Envelope<T>;
  expect(
    payload.success,
    `${method} ${path} envelope error: ${payload.error}`,
  ).toBe(true);
  return payload.data;
}

export function apiResponse(
  request: APIRequestContext,
  actor: Actor,
  method: string,
  path: string,
  body?: unknown,
) {
  return rawApi(request, actor, method, path, body);
}

export function futureISO(hours: number) {
  return new Date(Date.now() + hours * 60 * 60 * 1000).toISOString();
}

async function prepareProviderForNavigation(page: Page) {
  if (!activeBrowserProviderToken) return;
  await page.setExtraHTTPHeaders({
    Authorization: `Bearer ${activeBrowserProviderToken}`,
  });
  await page.addInitScript((token) => {
    (
      globalThis as typeof globalThis & { __CETS_PROVIDER_TOKEN__?: string }
    ).__CETS_PROVIDER_TOKEN__ = token;
  }, activeBrowserProviderToken);
}

async function syncBrowserProviderToken(page: Page) {
  if (!activeBrowserProviderToken) return;
  await page.evaluate((token) => {
    (
      globalThis as typeof globalThis & { __CETS_PROVIDER_TOKEN__?: string }
    ).__CETS_PROVIDER_TOKEN__ = token;
  }, activeBrowserProviderToken);
}

async function rawApi(
  request: APIRequestContext,
  actor: Actor,
  method: string,
  path: string,
  body?: unknown,
) {
  const providerToken = await providerTokenFor(request, actor.id);
  return request.fetch(path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${providerToken}`,
    },
    data: body,
  });
}

async function providerTokenFor(request: APIRequestContext, profileID: string) {
  const cached = providerTokens.get(profileID);
  if (cached) return cached;

  const response = await request.post("/api/v1/auth/mock-provider-token", {
    headers: {
      "Content-Type": "application/json",
    },
    data: {
      profile_id: profileID,
    },
  });
  const text = await response.text();
  expect(
    response.ok(),
    `POST /api/v1/auth/mock-provider-token failed for ${profileID}: ${text}`,
  ).toBe(true);
  const payload = JSON.parse(text) as Envelope<MockProviderToken>;
  expect(
    payload.success,
    `mock provider token envelope failed for ${profileID}: ${payload.error}`,
  ).toBe(true);
  expect(
    payload.data.provider_token,
    `mock provider token missing for ${profileID}`,
  ).toBeTruthy();
  providerTokens.set(profileID, payload.data.provider_token);
  return payload.data.provider_token;
}

function escapeRegExp(value: string) {
  return value.replaceAll(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);
}
