import { browser } from "k6/browser";
import { check, sleep } from "k6";
import exec from "k6/execution";
import http from "k6/http";
import { Trend } from "k6/metrics";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1:8080";
const smokeDuration = __ENV.K6_SMOKE_DURATION || "45s";
const checkinDuration = new Trend("checkin_duration", true);
const providerTokens = {};
let currentPrincipalId = "E1001";

export const options = {
  scenarios: {
    httpGate: {
      executor: "constant-vus",
      exec: "httpGate",
      vus: 1,
      duration: smokeDuration
    },
    browserGate: {
      executor: "shared-iterations",
      exec: "browserGate",
      vus: 1,
      iterations: 1,
      maxDuration: smokeDuration,
      options: {
        browser: {
          type: "chromium"
        }
      }
    }
  },
  thresholds: {
    checks: ["rate>=0.995"],
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500", "p(99)<1000"],
    checkin_duration: ["p(99)<200"],
    browser_web_vital_fcp: ["p(95)<1000"],
    browser_web_vital_lcp: ["p(95)<2000"]
  }
};

export function setup() {
  const health = http.get(`${baseUrl}/healthz`);
  const ready = http.get(`${baseUrl}/readyz`);
  const seeded = rawPost("/api/v1/admin/seed-demo", {}, authHeaders("admin-1"));
  const ok = check(null, {
    "setup health status is 200": () => health.status === 200,
    "setup ready status is 200": () => ready.status === 200,
    "seed demo accepted": () => seeded.status === 200
  });
  if (!ok) {
    exec.test.abort(`phase1 production gate setup failed: health=${health.status} ready=${ready.status} seed=${seeded.status}`);
  }
}

export function httpGate() {
  checkEndpoint("/", "html shell");
  checkEndpoint("/healthz", "health");
  checkEndpoint("/readyz", "ready");

  login("admin-1");
  const event = createEvent();
  requireValue(event?.event_id, "event create returned event_id");

  login("E1001");
  const eventList = get("/api/v1/events", "event browse");
  requireValue(eventList?.length, "event browse returned events");
  const eventDetail = get(`/api/v1/events/${event.event_id}`, "event detail");
  requireValue(eventDetail?.event_id, "event detail returned event_id");
  const booking = book(event.event_id, "E1001", uniqueKey("book-e1001"));
  requireValue(booking?.registration?.registration_id, "employee booking returned registration_id");
  const tickets = get("/api/v1/me/tickets", "employee ticket detail");
  const token = tickets?.[0]?.signed_token || booking?.ticket?.signed_token || "";
  requireValue(token, "employee receives reusable token only in owner flow");

  login("E1002");
  const cancelBooking = book(event.event_id, "E1002", uniqueKey("book-e1002"));
  requireValue(cancelBooking?.registration?.registration_id, "cancel booking returned registration_id");
  post(`/api/v1/events/${event.event_id}/bookings/${cancelBooking.registration.registration_id}/cancel`, {
    idempotency_key: uniqueKey("cancel-e1002"),
    reason: "k6 production gate"
  }, "employee cancellation");

  login("staff-1");
  const offlinePackage = get(`/api/v1/checkins/events/${event.event_id}/offline-package?device_id=gate-k6`, "offline package");
  requireValue(offlinePackage?.batch_id, "offline package returned batch_id");
  requireValue(offlinePackage?.package_signature, "offline package returned signature");
  post("/api/v1/checkins/offline-sync", {
    batch_id: offlinePackage.batch_id,
    event_id: offlinePackage.event_id,
    device_id: offlinePackage.device_id,
    package_signature: offlinePackage.package_signature,
    scans: []
  }, "offline sync");
  const checkinResponse = rawPost("/api/v1/checkins", { signed_token: token, event_id: event.event_id, device_id: "gate-k6" });
  check(checkinResponse, { "online check-in status is 2xx": (res) => res.status >= 200 && res.status < 300 });
  envelopeData(checkinResponse, "online check-in");
  checkinDuration.add(checkinResponse.timings.duration);

  login("hr-1");
  get("/api/v1/admin/reports", "reports");
  const reportExport = post("/api/v1/admin/reports/exports", { report_type: "participation" }, "report export");
  requireValue(reportExport?.export_id, "report export returned export_id");
  const readyExport = waitForReportExport(reportExport.export_id);
  requireValue(readyExport?.object_key, "report export ready object key");
  get("/api/v1/admin/audit-logs?limit=25", "audit cursor page");
  get("/api/v1/admin/notifications/deliveries", "notification deliveries");
  sleep(1);
}

export async function browserGate() {
  const page = await browser.newPage();
  try {
    await loginInBrowser(page, "E1001");
    await expectRoute(page, "/user/tickets", "我的票券");
    await loginInBrowser(page, "staff-1");
    await expectRoute(page, "/admin/checkin", "現場驗票");
    await loginInBrowser(page, "hr-1");
    await expectRoute(page, "/admin/audit", "稽核紀錄");
  } finally {
    await page.close();
  }
}

function checkEndpoint(path, label) {
  const response = http.get(`${baseUrl}${path}`);
  check(response, {
    [`${label} status is 200`]: (res) => res.status === 200,
    [`${label} body present`]: (res) => Boolean(res.body && res.body.length > 0)
  });
}

function login(principalId) {
  currentPrincipalId = principalId;
  const claims = get("/api/v1/auth/me", `provider claims ${principalId}`);
  check(claims, { [`${principalId} provider claims loaded`]: (value) => value?.employee_id === principalId });
  return claims;
}

function createEvent() {
  const suffix = `${__VU}-${__ITER}-${Date.now()}`;
  return post("/api/v1/admin/events", {
    title: `k6 Phase 1 Gate ${suffix}`,
    description: "Production gate event",
    location: "Taipei HQ",
    capacity: 2,
    status: "published",
    rule: {
      department: "Engineering",
      site: "Taipei",
      min_grade: 5,
      employment_status: "active"
    }
  }, "event create");
}

function book(eventId, employeeId, idempotencyKey) {
  return post(`/api/v1/events/${eventId}/bookings`, {
    idempotency_key: idempotencyKey
  }, `booking ${employeeId}`);
}

function get(path, label) {
  const response = http.get(`${baseUrl}${path}`, {
    headers: authHeaders(currentPrincipalId)
  });
  check(response, { [`${label} status is 200`]: (res) => res.status === 200 });
  return envelopeData(response, label);
}

function post(path, body, label = path) {
  const response = rawPost(path, body);
  check(response, { [`${label} status is 2xx`]: (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, label, response);
}

function rawPost(path, body, extraHeaders = {}) {
  return http.post(`${baseUrl}${path}`, JSON.stringify(body), {
    headers: { "Content-Type": "application/json", ...authHeaders(currentPrincipalId), ...extraHeaders }
  });
}

function authHeaders(actorId) {
  return { Authorization: `Bearer ${providerTokenFor(actorId)}` };
}

function providerTokenFor(actorId) {
  if (providerTokens[actorId]) return providerTokens[actorId];
  const response = http.post(`${baseUrl}/api/v1/auth/mock-provider-token`, JSON.stringify({ profile_id: actorId }), {
    headers: { "Content-Type": "application/json" }
  });
  const token = envelopeData(response, `mock provider token ${actorId}`)?.provider_token || "";
  if (!token) {
    exec.test.abort(`mock provider token ${actorId} was not issued`);
  }
  providerTokens[actorId] = token;
  return token;
}

function envelopeData(response, label, fallback = null) {
  try {
    const payload = response.json();
    check(payload, { [`${label} envelope success`]: (body) => body?.success === true });
    return payload.data || fallback;
  } catch {
    check(response, { [`${label} json envelope parsed`]: () => false });
    return fallback;
  }
}

function waitForReportExport(exportId) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const reportExport = get(`/api/v1/admin/reports/exports/${exportId}`, "report export status");
    if (reportExport?.status === "ready") {
      check(reportExport, {
        "report export reached ready status": (value) => value.status === "ready",
        "report export has object key": (value) => Boolean(value.object_key)
      });
      return reportExport;
    }
    if (reportExport?.status === "failed") {
      exec.test.abort(`report export ${exportId} failed`);
    }
    sleep(0.5);
  }
  exec.test.abort(`report export ${exportId} did not become ready`);
}

function requireValue(value, label) {
  const ok = check(value, { [label]: Boolean });
  if (!ok) {
    exec.test.abort(label);
  }
  return value;
}

async function loginInBrowser(page, principalId) {
  await page.goto(baseUrl, { waitUntil: "networkidle" });
  await page.evaluate(() => {
    const button = Array.from(document.querySelectorAll("button")).find((candidate) => candidate.textContent?.includes("切換 Profile"));
    button?.click();
  });
  await page.waitForLoadState("networkidle");
  await page.locator(`//button[contains(., "${principalId}")]`).click();
  await page.waitForLoadState("networkidle");
}

async function expectRoute(page, path, heading) {
  await page.evaluate((nextPath) => {
    globalThis.history.pushState({}, "", nextPath);
    globalThis.dispatchEvent(new Event("popstate"));
  }, path);
  await page.waitForLoadState("networkidle");
  const text = await page.locator(`//*[contains(., "${heading}")]`).first().textContent();
  check(text, { [`browser route ${path} visible`]: (value) => value?.includes(heading) === true });
  const overflow = await page.evaluate(() => Math.ceil(document.documentElement.scrollWidth - globalThis.innerWidth));
  check(overflow, { [`browser route ${path} no horizontal overflow`]: (value) => value <= 1 });
}

function uniqueKey(prefix) {
  return `${prefix}-${__VU}-${__ITER}-${Date.now()}`;
}
