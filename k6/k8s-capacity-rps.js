import { check, sleep } from "k6";
import crypto from "k6/crypto";
import encoding from "k6/encoding";
import exec from "k6/execution";
import http from "k6/http";
import { Counter, Trend } from "k6/metrics";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1";
const hostHeader = __ENV.K6_HOST_HEADER || "";
const providerSecret = __ENV.K6_PROVIDER_TOKEN_SECRET || "";
const targetRps = Number(__ENV.K6_TARGET_RPS || "100");
const duration = __ENV.K6_CAPACITY_DURATION || "82s";
const readRatio = Number(__ENV.K6_READ_RATIO || "0.8");
const employeePrefix = __ENV.K6_EMPLOYEE_PREFIX || "KB";
const employeeCount = Number(__ENV.K6_EMPLOYEE_COUNT || "10000");
const hotEventCapacity = Number(__ENV.K6_HOT_EVENT_CAPACITY || String(employeeCount));
const runId = __ENV.K6_RUN_ID || `${Date.now()}`;

const readRate = Math.max(1, Math.floor(targetRps * readRatio));
const bookingRate = Math.max(1, targetRps - readRate);
const providerTokens = {};

const readDuration = new Trend("k8s_read_duration", true);
const bookingDuration = new Trend("k8s_booking_duration", true);
const bookingAttempts = new Counter("k8s_booking_attempts");
const bookingConfirmed = new Counter("k8s_booking_confirmed");
const bookingWaitlisted = new Counter("k8s_booking_waitlisted");
const bookingConflicts = new Counter("k8s_booking_conflicts");
const gatewayReplicaHits = new Counter("k8s_gateway_replica_hits");
const frontendReplicaHits = new Counter("k8s_frontend_replica_hits");
const backendReplicaHits = new Counter("k8s_backend_replica_hits");

export const options = {
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    readTraffic: {
      executor: "constant-arrival-rate",
      exec: "readTraffic",
      rate: readRate,
      timeUnit: "1s",
      duration,
      preAllocatedVUs: Math.max(20, Math.ceil(readRate / 2)),
      maxVUs: Math.max(100, readRate * 2),
      tags: { flow: "read", expected_error: "false" }
    },
    bookingTraffic: {
      executor: "constant-arrival-rate",
      exec: "bookingTraffic",
      rate: bookingRate,
      timeUnit: "1s",
      duration,
      preAllocatedVUs: Math.max(20, bookingRate),
      maxVUs: Math.max(100, bookingRate * 4),
      tags: { flow: "booking", expected_error: "false" }
    }
  },
  thresholds: {
    checks: ["rate>=0.99"],
    "http_req_failed{expected_error:false}": ["rate<0.005"],
    "http_req_duration{flow:read}": ["p(95)<500", "p(99)<1000"],
    k8s_booking_duration: ["p(95)<750", "p(99)<1500"],
    k8s_booking_attempts: ["count>=1"],
    k8s_booking_confirmed: ["count>=1"],
    k8s_gateway_replica_hits: ["count>=3"],
    k8s_frontend_replica_hits: ["count>=3"],
    k8s_backend_replica_hits: ["count>=3"]
  }
};

export function setup() {
  if (!providerSecret) {
    exec.test.abort("K6_PROVIDER_TOKEN_SECRET is required");
  }
  const health = http.get(`${baseUrl}/healthz`, requestParams("setup"));
  const ready = http.get(`${baseUrl}/readyz`, requestParams("setup"));
  recordReplicas(health);
  recordReplicas(ready);

  const event = createHotEvent();
  const ok = check(null, {
    "k8s benchmark health is 200": () => health.status === 200,
    "k8s benchmark ready is 200": () => ready.status === 200,
    "k8s benchmark event created": () => Boolean(event?.event_id)
  });
  if (!ok) {
    exec.test.abort(`k8s benchmark setup failed: health=${health.status} ready=${ready.status}`);
  }
  return { eventId: event.event_id };
}

export function readTraffic(data) {
  const selector = exec.scenario.iterationInTest % 5;
  const actorId = employeeId((exec.scenario.iterationInTest % employeeCount) + 1);
  let response;
  if (selector === 0) {
    response = http.get(`${baseUrl}/`, requestParams("read", actorId));
  } else if (selector === 1) {
    response = http.get(`${baseUrl}/readyz`, requestParams("read", actorId));
  } else if (selector === 2) {
    response = http.get(`${baseUrl}/api/v1/events`, requestParams("read", actorId));
  } else if (selector === 3 && data?.eventId) {
    response = http.get(`${baseUrl}/api/v1/events/${data.eventId}`, requestParams("read", actorId));
  } else {
    response = http.get(`${baseUrl}/api/v1/me/tickets`, requestParams("read", actorId));
  }
  recordReplicas(response);
  readDuration.add(response.timings.duration);
  check(response, { "read traffic status is 200": (res) => res.status === 200 });
  sleep(0.01);
}

export function bookingTraffic(data) {
  if (!data?.eventId) {
    exec.test.abort("booking traffic missing eventId");
  }
  const iteration = exec.scenario.iterationInTest;
  const actorId = employeeId((iteration % employeeCount) + 1);
  const response = http.post(
    `${baseUrl}/api/v1/events/${data.eventId}/bookings`,
    JSON.stringify({ idempotency_key: `${runId}-book-${iteration}`, family_count: 0 }),
    requestParams("booking", actorId)
  );
  recordReplicas(response);
  bookingAttempts.add(1);
  bookingDuration.add(response.timings.duration);
  check(response, { "booking traffic status is 2xx": (res) => res.status >= 200 && res.status < 300 });
  const payload = envelopeData(response, "booking");
  const status = payload?.registration?.status || "";
  if (status === "confirmed") {
    bookingConfirmed.add(1);
  } else if (status === "waitlisted") {
    bookingWaitlisted.add(1);
  } else if (response.status === 409) {
    bookingConflicts.add(1);
  }
  check(status, {
    "booking result is confirmed or waitlisted": (value) => value === "confirmed" || value === "waitlisted"
  });
  sleep(0.01);
}

function createHotEvent() {
  const now = Date.now();
  const startsAt = new Date(now + 7 * 24 * 60 * 60 * 1000).toISOString();
  const registrationStart = new Date(now - 60 * 60 * 1000).toISOString();
  const registrationClose = new Date(now + 6 * 24 * 60 * 60 * 1000).toISOString();
  const response = http.post(
    `${baseUrl}/api/v1/admin/events`,
    JSON.stringify({
      title: `k8s capacity ${runId}`,
      description: "K8s production-like capacity benchmark hot event",
      location: "Taipei HQ",
      event_city: "Taipei",
      event_site: "Taipei HQ",
      starts_at: startsAt,
      registration_start: registrationStart,
      registration_close: registrationClose,
      capacity_type: "limited",
      capacity: hotEventCapacity,
      allows_family: false,
      status: "published",
      category: "capacity",
      tags: ["k8s", "capacity"],
      entry_method: "qr",
      visibility: "eligible",
      rule: {
        department: "Engineering",
        site: "Taipei HQ",
        min_grade: 1,
        employment_status: "active"
      }
    }),
    requestParams("setup", "admin-1")
  );
  recordReplicas(response);
  check(response, { "hot event create status is 2xx": (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, "hot event create");
}

function requestParams(flow, actorId = "") {
  const headers = {
    "Content-Type": "application/json",
    "X-CETS-Benchmark": "k8s-capacity"
  };
  if (hostHeader) {
    headers.Host = hostHeader;
  }
  if (actorId) {
    headers.Authorization = `Bearer ${providerTokenFor(actorId)}`;
  }
  return {
    headers,
    tags: { flow, expected_error: "false" }
  };
}

function providerTokenFor(actorId) {
  if (providerTokens[actorId]) return providerTokens[actorId];
  const payload = JSON.stringify(claimsFor(actorId));
  const payloadPart = encoding.b64encode(payload, "rawurl");
  const signature = crypto.hmac("sha256", providerSecret, payloadPart, "base64rawurl");
  providerTokens[actorId] = `${payloadPart}.${signature}`;
  return providerTokens[actorId];
}

function claimsFor(actorId) {
  const role = actorId === "admin-1" ? "activity_admin" : "employee";
  return {
    employee_id: actorId,
    display_name: actorId === "admin-1" ? "Benchmark Admin" : `Benchmark ${actorId}`,
    role_claims: [role],
    department: actorId === "admin-1" ? "Welfare Committee" : "Engineering",
    site: "Taipei HQ",
    city: "Taipei",
    grade: actorId === "admin-1" ? 7 : 5,
    employment_status: "active",
    exp: Math.floor(Date.now() / 1000) + 3600
  };
}

function employeeId(index) {
  return `${employeePrefix}${String(index).padStart(6, "0")}`;
}

function envelopeData(response, label) {
  try {
    const payload = response.json();
    check(payload, { [`${label} envelope success`]: (body) => body && body.success === true });
    return payload.data || null;
  } catch {
    check(response, { [`${label} json envelope parsed`]: () => false });
    return null;
  }
}

function recordReplicas(response) {
  addReplicaHit(gatewayReplicaHits, response.headers["X-CETS-Gateway-Replica"] || response.headers["X-Cets-Gateway-Replica"]);
  addReplicaHit(frontendReplicaHits, response.headers["X-CETS-Frontend-Replica"] || response.headers["X-Cets-Frontend-Replica"]);
  addReplicaHit(backendReplicaHits, response.headers["X-CETS-Backend-Replica"] || response.headers["X-Cets-Backend-Replica"]);
}

function addReplicaHit(counter, replica) {
  if (!replica) return;
  counter.add(1, { replica: String(replica).slice(0, 80) });
}
