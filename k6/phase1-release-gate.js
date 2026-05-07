import { check, sleep } from "k6";
import exec from "k6/execution";
import http from "k6/http";
import { Trend } from "k6/metrics";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1:8080";
const releaseDuration = __ENV.K6_RELEASE_DURATION || "30s";
const checkinDuration = new Trend("release_checkin_duration", true);

export const options = {
  scenarios: {
    appRpsGate: {
      executor: "constant-arrival-rate",
      exec: "browseGate",
      rate: 36,
      timeUnit: "1s",
      duration: releaseDuration,
      preAllocatedVUs: 12,
      maxVUs: 72,
      tags: { gate: "app_rps" }
    },
    bookingTpsGate: {
      executor: "constant-arrival-rate",
      exec: "bookingGate",
      rate: 5,
      timeUnit: "1s",
      duration: releaseDuration,
      preAllocatedVUs: 16,
      maxVUs: 96,
      startTime: "5s",
      tags: { gate: "booking_tps" }
    },
    adminReleaseGate: {
      executor: "shared-iterations",
      exec: "adminGate",
      vus: 2,
      iterations: 4,
      maxDuration: "2m",
      startTime: "40s",
      tags: { gate: "admin_release" }
    }
  },
  thresholds: {
    checks: ["rate>=0.995"],
    "http_req_failed{gate:app_rps}": ["rate<0.01"],
    "http_req_failed{gate:booking_tps}": ["rate<0.01"],
    "http_req_failed{gate:admin_release}": ["rate<0.01"],
    "http_req_duration{gate:app_rps}": ["p(95)<500", "p(99)<1000"],
    "http_req_duration{gate:booking_tps}": ["p(95)<750", "p(99)<1500"],
    "http_req_duration{gate:admin_release}": ["p(95)<1000", "p(99)<2000"],
    release_checkin_duration: ["p(99)<200"]
  }
};

export function setup() {
  const health = http.get(`${baseUrl}/healthz`);
  const ready = http.get(`${baseUrl}/readyz`);
  const seeded = rawPost("/api/v1/admin/seed-demo", {}, actorHeaders("admin-1", "activity_admin"));
  const ok = check(null, {
    "release setup health is 200": () => health.status === 200,
    "release setup ready is 200": () => ready.status === 200,
    "release setup seed demo accepted": () => seeded.status === 200
  });
  if (!ok) {
    exec.test.abort(`phase1 release gate setup failed: health=${health.status} ready=${ready.status} seed=${seeded.status}`);
  }
}

export function browseGate() {
  checkEndpoint("/", "html shell");
  checkEndpoint("/healthz", "health");
  checkEndpoint("/readyz", "ready");
  get("/api/v1/events?employee_id=E1001", "employee browse", "E1001", "employee");
}

export function bookingGate() {
  const event = createEvent("booking", 1);
  requireValue(event?.event_id, "booking gate event id");

  const bookingKey = uniqueKey("book-e1001");
  const first = book(event.event_id, "E1001", bookingKey);
  requireValue(first?.registration?.registration_id, "booking gate first registration");
  const retry = book(event.event_id, "E1001", bookingKey);
  check(retry, {
    "idempotent booking retry returns same registration": (value) =>
      value?.registration?.registration_id === first.registration.registration_id
  });

  const waitlisted = book(event.event_id, "E1002", uniqueKey("book-e1002"));
  check(waitlisted, { "second booking enters waitlist": (value) => value?.registration?.status === "waitlisted" });

  post(
    `/api/v1/events/${event.event_id}/bookings/${first.registration.registration_id}/cancel`,
    {
      idempotency_key: uniqueKey("cancel-e1001"),
      reason: "k6 release waitlist promotion setup"
    },
    "cancel first booking",
    "E1001",
    "employee"
  );
  const tickets = get("/api/v1/employees/E1002/tickets", "promoted ticket lookup", "E1002", "employee");
  const promotedTicket = tickets?.find((ticket) => ticket.event_id === event.event_id && ticket.status === "active");
  check(promotedTicket, {
    "cancellation auto-promotes waitlisted employee": (ticket) => Boolean(ticket?.signed_token)
  });
  const token = promotedTicket?.signed_token || "";
  requireValue(token, "promoted employee receives ticket token");

  const checkin = rawPost("/api/v1/checkins", { signed_token: token, device_id: "k6-release" }, actorHeaders("staff-1", "checkin_staff"));
  check(checkin, { "release check-in accepted": (res) => res.status >= 200 && res.status < 300 });
  checkinDuration.add(checkin.timings.duration);
  envelopeData(checkin, "release check-in");

  const duplicate = rawPostExpected(
    "/api/v1/checkins",
    { signed_token: token, device_id: "k6-release" },
    actorHeaders("staff-1", "checkin_staff"),
    http.expectedStatuses(409)
  );
  check(duplicate, { "duplicate check-in rejected": (res) => res.status === 409 });
}

export function adminGate() {
  const event = createEvent("lottery", 1);
  requireValue(event?.event_id, "lottery gate event id");
  const first = book(event.event_id, "E1001", uniqueKey("lottery-book-e1001"));
  requireValue(first?.registration?.registration_id, "lottery gate first registration");
  const waitlisted = book(event.event_id, "E1002", uniqueKey("lottery-book-e1002"));
  check(waitlisted, { "lottery candidate waitlisted": (value) => value?.registration?.status === "waitlisted" });
  patch(
    `/api/v1/admin/events/${event.event_id}`,
    { capacity: 2 },
    "lottery capacity setup",
    "admin-1",
    "activity_admin"
  );

  const seed = uniqueKey("lottery-seed");
  const lottery = post(
    `/api/v1/admin/events/${event.event_id}/lottery-runs`,
    { seed },
    "lottery run",
    "admin-1",
    "activity_admin"
  );
  check(lottery, {
    "lottery result persisted": (value) => Boolean(value?.run_id),
    "lottery has deterministic seed": (value) => value?.seed === seed,
    "lottery selected at least one winner": (value) => value?.winner_count > 0
  });
  const lotteryTickets = get("/api/v1/employees/E1002/tickets", "lottery winner ticket lookup", "E1002", "employee");
  const lotteryTicket = lotteryTickets?.find((ticket) => ticket.event_id === event.event_id && ticket.status === "active");
  check(lotteryTicket, {
    "lottery winner receives active ticket": (ticket) => Boolean(ticket?.signed_token)
  });

  get("/api/v1/admin/reports", "reports aggregate", "hr-1", "hr_admin");
  const reportExport = post(
    "/api/v1/admin/reports/exports",
    { report_type: "participation" },
    "report export request",
    "hr-1",
    "hr_admin"
  );
  requireValue(reportExport?.export_id, "report export id");
  waitForReportExport(reportExport.export_id);

  const auditPage = get("/api/v1/admin/audit-logs?limit=5", "audit first page", "hr-1", "hr_admin");
  requireValue(auditPage?.length, "audit first page records");
  const last = auditPage[auditPage.length - 1];
  const cursor = encodeURIComponent(`${last.created_at}|${last.audit_id}`);
  get(`/api/v1/admin/audit-logs?limit=5&cursor=${cursor}`, "audit cursor page", "hr-1", "hr_admin");
  get("/api/v1/admin/notifications/deliveries", "notification deliveries", "admin-1", "activity_admin");
}

function checkEndpoint(path, label) {
  const response = http.get(`${baseUrl}${path}`);
  check(response, {
    [`${label} status is 200`]: (res) => res.status === 200,
    [`${label} body present`]: (res) => Boolean(res.body && res.body.length > 0)
  });
}

function createEvent(prefix, capacity) {
  const suffix = `${prefix}-${__VU}-${__ITER}-${Date.now()}`;
  return post(
    "/api/v1/admin/events",
    {
      title: `k6 Release ${suffix}`,
      description: "Phase 1 release gate event",
      location: "Taipei HQ",
      capacity,
      status: "published",
      rule: {
        department: "Engineering",
        site: "Taipei",
        min_grade: 5,
        employment_status: "active"
      }
    },
    "event create",
    "admin-1",
    "activity_admin"
  );
}

function book(eventId, employeeId, idempotencyKey) {
  return post(
    `/api/v1/events/${eventId}/bookings`,
    {
      employee_id: employeeId,
      idempotency_key: idempotencyKey
    },
    "booking",
    employeeId,
    "employee"
  );
}

function get(path, label, actorId, role) {
  const response = http.get(`${baseUrl}${path}`, {
    headers: actorHeaders(actorId, role)
  });
  check(response, { [`${label} status is 200`]: (res) => res.status === 200 });
  return envelopeData(response, label);
}

function post(path, body, label, actorId, role) {
  const response = rawPost(path, body, actorHeaders(actorId, role));
  check(response, { [`${label} status is 2xx`]: (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, label, response);
}

function patch(path, body, label, actorId, role) {
  const response = http.patch(`${baseUrl}${path}`, JSON.stringify(body), {
    headers: { "Content-Type": "application/json", ...actorHeaders(actorId, role) }
  });
  check(response, { [`${label} status is 2xx`]: (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, label, response);
}

function rawPost(path, body, extraHeaders = {}) {
  return http.post(`${baseUrl}${path}`, JSON.stringify(body), {
    headers: { "Content-Type": "application/json", ...extraHeaders }
  });
}

function rawPostExpected(path, body, extraHeaders, responseCallback) {
  return http.post(`${baseUrl}${path}`, JSON.stringify(body), {
    headers: { "Content-Type": "application/json", ...extraHeaders },
    responseCallback
  });
}

function actorHeaders(actorId, role) {
  return { "X-Actor-ID": actorId, "X-Role": role };
}

function envelopeData(response, label, fallback = null) {
  try {
    const payload = response.json();
    check(payload, { [`${label} envelope success`]: (body) => body && body.success === true });
    return payload.data || fallback;
  } catch {
    check(response, { [`${label} json envelope parsed`]: () => false });
    return fallback;
  }
}

function waitForReportExport(exportId) {
  for (let attempt = 0; attempt < 30; attempt++) {
    const reportExport = get(`/api/v1/admin/reports/exports/${exportId}`, "report export status", "hr-1", "hr_admin");
    if (reportExport?.status === "ready") {
      check(reportExport, {
        "report export ready": (value) => value.status === "ready",
        "report export object key exists": (value) => Boolean(value.object_key)
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
  const ok = check(value, { [label]: (candidate) => Boolean(candidate) });
  if (!ok) {
    exec.test.abort(label);
  }
  return value;
}

function uniqueKey(prefix) {
  return `${prefix}-${__VU}-${__ITER}-${Date.now()}`;
}
