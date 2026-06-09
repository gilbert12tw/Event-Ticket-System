import { check, sleep } from "k6";
import crypto from "k6/crypto";
import encoding from "k6/encoding";
import exec from "k6/execution";
import http from "k6/http";
import { Counter, Trend } from "k6/metrics";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1:18080";
const hostHeader = __ENV.K6_HOST_HEADER || "";
const providerSecret = __ENV.K6_PROVIDER_TOKEN_SECRET || "";
const useMockProvider = __ENV.K6_USE_MOCK_PROVIDER || (!providerSecret ? "true" : "false");
const duration = __ENV.K6_DURATION || "120s";
const employeePrefix = __ENV.K6_EMPLOYEE_PREFIX || "LD";
const employeeCount = Number(__ENV.K6_EMPLOYEE_COUNT || "10000");
const hotEventCapacity = Number(__ENV.K6_HOT_EVENT_CAPACITY || "500");
const runId = __ENV.K6_RUN_ID || `lgtm-${Date.now()}`;

const readRate = Number(__ENV.K6_READ_RPS || "600");
const bookingRate = Number(__ENV.K6_BOOKING_RPS || "250");
const invalidBookingRate = Number(__ENV.K6_INVALID_BOOKING_RPS || "90");
const unauthorizedRate = Number(__ENV.K6_UNAUTHORIZED_RPS || "30");
const missingEventRate = Number(__ENV.K6_MISSING_EVENT_RPS || "30");

const validDuration = new Trend("lgtm_valid_duration", true);
const errorDuration = new Trend("lgtm_error_duration", true);
const validResponses = new Counter("lgtm_valid_responses");
const expectedErrors = new Counter("lgtm_expected_errors");
const unexpectedResponses = new Counter("lgtm_unexpected_responses");
const gatewayReplicaHits = new Counter("lgtm_gateway_replica_hits");
const frontendReplicaHits = new Counter("lgtm_frontend_replica_hits");
const backendReplicaHits = new Counter("lgtm_backend_replica_hits");

const providerTokens = {};

export const options = {
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    readTraffic: rateScenario("readTraffic", readRate, "read", false),
    bookingTraffic: rateScenario("bookingTraffic", bookingRate, "booking", false),
    invalidBookingTraffic: rateScenario("invalidBookingTraffic", invalidBookingRate, "invalid_booking", true),
    unauthorizedTraffic: rateScenario("unauthorizedTraffic", unauthorizedRate, "unauthorized", true),
    missingEventTraffic: rateScenario("missingEventTraffic", missingEventRate, "missing_event", true)
  },
  thresholds: {
    checks: ["rate>=0.95"],
    "http_req_duration{lgtm_expected_error:false}": ["p(95)<2000", "p(99)<5000"],
    "http_req_failed{lgtm_expected_error:false}": ["rate<0.02"],
    lgtm_valid_responses: ["count>=1"],
    lgtm_expected_errors: ["count>=1"],
    lgtm_backend_replica_hits: ["count>=1"]
  }
};

function rateScenario(execName, rate, flow, expectedError) {
  return {
    executor: "constant-arrival-rate",
    exec: execName,
    rate,
    timeUnit: "1s",
    duration,
    preAllocatedVUs: Math.max(20, Math.ceil(rate / 2)),
    maxVUs: Math.max(100, rate * 4),
    tags: { lgtm_flow: flow, lgtm_expected_error: String(expectedError) }
  };
}

export function setup() {
  const health = http.get(`${baseUrl}/healthz`, reqParams("setup"));
  const ready = http.get(`${baseUrl}/readyz`, reqParams("setup"));
  recordReplicas(health);
  recordReplicas(ready);

  const event = createHotEvent();
  const ok = check(null, {
    "setup health 200": () => health.status === 200,
    "setup ready 200": () => ready.status === 200,
    "setup event created": () => Boolean(event?.event_id)
  });
  if (!ok) {
    exec.test.abort(`setup failed: health=${health.status} ready=${ready.status}`);
  }
  return { eventId: event.event_id };
}

export function readTraffic(data) {
  const selector = exec.scenario.iterationInTest % 5;
  const actorId = employeeId((exec.scenario.iterationInTest % employeeCount) + 1);
  let response;
  if (selector === 0) {
    response = http.get(`${baseUrl}/readyz`, reqParams("read_readyz", actorId));
  } else if (selector === 1) {
    response = http.get(`${baseUrl}/api/v1/events`, reqParams("read_events", actorId));
  } else if (selector === 2 && data?.eventId) {
    response = http.get(`${baseUrl}/api/v1/events/${data.eventId}`, reqParams("read_event_detail", actorId));
  } else if (selector === 3) {
    response = http.get(`${baseUrl}/api/v1/me/tickets`, reqParams("read_tickets", actorId));
  } else {
    response = http.get(`${baseUrl}/healthz`, reqParams("read_healthz", actorId));
  }
  recordValid(response, "read status 200", (res) => res.status === 200);
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
    reqParams("booking", actorId)
  );
  recordValid(response, "booking 2xx", (res) => res.status >= 200 && res.status < 300);
}

export function invalidBookingTraffic(data) {
  if (!data?.eventId) {
    exec.test.abort("invalid booking missing eventId");
  }
  const actorId = employeeId((exec.scenario.iterationInTest % employeeCount) + 1);
  const response = http.post(
    `${baseUrl}/api/v1/events/${data.eventId}/bookings`,
    JSON.stringify({ family_count: 0 }),
    reqParams("invalid_booking", actorId, true)
  );
  recordExpected(response, "invalid booking 400", (res) => res.status === 400);
}

export function unauthorizedTraffic() {
  const response = http.get(
    `${baseUrl}/api/v1/me/tickets`,
    reqParams("unauthorized", "", true)
  );
  recordExpected(response, "unauthorized 401/403", (res) => res.status === 401 || res.status === 403);
}

export function missingEventTraffic() {
  const actorId = employeeId((exec.scenario.iterationInTest % employeeCount) + 1);
  const missingId = `evt_missing_${runId}_${exec.scenario.iterationInTest}`;
  const response = http.get(
    `${baseUrl}/api/v1/events/${missingId}`,
    reqParams("missing_event", actorId, true)
  );
  recordExpected(response, "missing event 404", (res) => res.status === 404);
}

function createHotEvent() {
  const now = Date.now();
  const response = http.post(
    `${baseUrl}/api/v1/admin/events`,
    JSON.stringify({
      title: `k6 LGTM Debug ${runId}`,
      description: "1000 RPS LGTM debug workload event",
      location: "Taipei HQ",
      event_city: "Taipei",
      event_site: "Taipei HQ",
      starts_at: new Date(now + 7 * 24 * 60 * 60 * 1000).toISOString(),
      registration_start: new Date(now - 60 * 60 * 1000).toISOString(),
      registration_close: new Date(now + 6 * 24 * 60 * 60 * 1000).toISOString(),
      capacity_type: "limited",
      capacity: hotEventCapacity,
      allows_family: false,
      status: "published",
      category: "demo",
      tags: ["lgtm", "debug-1000rps"],
      entry_method: "qr",
      visibility: "eligible",
      rule: {
        department: "Engineering",
        site: "Taipei HQ",
        min_grade: 1,
        employment_status: "active"
      }
    }),
    reqParams("setup", "admin-1")
  );
  recordReplicas(response);
  check(response, { "event create 2xx": (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response);
}

function reqParams(flow, actorId = "", expectedError = false) {
  const headers = {
    "Content-Type": "application/json",
    "X-CETS-Benchmark": "lgtm-debug-1000rps"
  };
  if (hostHeader) {
    headers.Host = hostHeader;
  }
  if (actorId) {
    headers.Authorization = `Bearer ${tokenFor(actorId)}`;
  }
  return {
    headers,
    tags: { lgtm_flow: flow, lgtm_expected_error: String(expectedError) }
  };
}

function tokenFor(actorId) {
  if (providerTokens[actorId]) return providerTokens[actorId];
  if (useMockProvider === "true") {
    const response = http.post(
      `${baseUrl}/api/v1/auth/mock-provider-token`,
      JSON.stringify({ profile_id: actorId }),
      { headers: { "Content-Type": "application/json" }, tags: { lgtm_flow: "mock_token", lgtm_expected_error: "false" } }
    );
    const token = envelopeData(response)?.provider_token || "";
    if (!token) exec.test.abort(`mock provider token for ${actorId} not issued`);
    providerTokens[actorId] = token;
    return token;
  }
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
    display_name: actorId === "admin-1" ? "LGTM Debug Admin" : `LGTM ${actorId}`,
    role_claims: [role],
    department: actorId === "admin-1" ? "Welfare Committee" : "Engineering",
    site: "Taipei HQ",
    city: "Taipei",
    grade: actorId === "admin-1" ? 7 : 5,
    employment_status: "active",
    exp: Math.floor(Date.now() / 1000) + 3600
  };
}

function recordValid(response, label, predicate) {
  recordReplicas(response);
  validDuration.add(response.timings.duration);
  if (check(response, { [label]: predicate })) {
    validResponses.add(1);
  } else {
    unexpectedResponses.add(1);
  }
  sleep(0.01);
}

function recordExpected(response, label, predicate) {
  recordReplicas(response);
  errorDuration.add(response.timings.duration);
  if (check(response, { [label]: predicate })) {
    expectedErrors.add(1);
  } else {
    unexpectedResponses.add(1);
  }
  sleep(0.01);
}

function envelopeData(response) {
  try {
    const payload = response.json();
    check(payload, { "envelope success": (body) => body && body.success === true });
    return payload.data || null;
  } catch {
    check(response, { "json envelope parsed": () => false });
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

function employeeId(index) {
  return `${employeePrefix}${String(index).padStart(6, "0")}`;
}
