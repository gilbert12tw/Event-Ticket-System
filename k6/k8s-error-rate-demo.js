import { check, sleep } from "k6";
import crypto from "k6/crypto";
import encoding from "k6/encoding";
import exec from "k6/execution";
import http from "k6/http";
import { Counter, Trend } from "k6/metrics";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1";
const hostHeader = __ENV.K6_HOST_HEADER || "";
const providerSecret = __ENV.K6_PROVIDER_TOKEN_SECRET || "";
const targetRps = Number(__ENV.K6_ERROR_DEMO_RPS || "350");
const duration = __ENV.K6_ERROR_DEMO_DURATION || "120s";
const errorRatio = clampRatio(Number(__ENV.K6_ERROR_DEMO_ERROR_RATIO || "0.4"));
const employeePrefix = __ENV.K6_EMPLOYEE_PREFIX || "KE";
const employeeCount = Number(__ENV.K6_EMPLOYEE_COUNT || "10000");
const hotEventCapacity = Number(__ENV.K6_ERROR_DEMO_HOT_EVENT_CAPACITY || String(employeeCount));
const runId = __ENV.K6_RUN_ID || `${Date.now()}`;

const expectedErrorRate = Math.max(1, Math.floor(targetRps * errorRatio));
const validRate = Math.max(1, targetRps - expectedErrorRate);
const validReadRate = Math.max(1, Math.floor(validRate * 0.8));
const validBookingRate = Math.max(1, validRate - validReadRate);
const invalidBookingRate = Math.max(1, Math.floor(expectedErrorRate * 0.6));
const unauthorizedReadRate = Math.max(1, Math.floor(expectedErrorRate * 0.25));
const missingEventRate = Math.max(1, expectedErrorRate - invalidBookingRate - unauthorizedReadRate);
const providerTokens = {};

const validDuration = new Trend("k8s_error_demo_valid_duration", true);
const expectedErrorDuration = new Trend("k8s_error_demo_expected_error_duration", true);
const expectedErrors = new Counter("k8s_error_demo_expected_errors");
const unexpectedResponses = new Counter("k8s_error_demo_unexpected_responses");
const validResponses = new Counter("k8s_error_demo_valid_responses");
const gatewayReplicaHits = new Counter("k8s_error_demo_gateway_replica_hits");
const frontendReplicaHits = new Counter("k8s_error_demo_frontend_replica_hits");
const backendReplicaHits = new Counter("k8s_error_demo_backend_replica_hits");

export const options = {
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    validReadTraffic: constantRateScenario("validReadTraffic", validReadRate, "valid_read", false),
    validBookingTraffic: constantRateScenario("validBookingTraffic", validBookingRate, "valid_booking", false),
    invalidBookingTraffic: constantRateScenario("invalidBookingTraffic", invalidBookingRate, "invalid_booking", true),
    unauthorizedReadTraffic: constantRateScenario("unauthorizedReadTraffic", unauthorizedReadRate, "unauthorized_read", true),
    missingEventTraffic: constantRateScenario("missingEventTraffic", missingEventRate, "missing_event", true)
  },
  thresholds: {
    checks: ["rate>=0.95"],
    "http_req_failed{error_demo_expected:false}": ["rate<0.02"],
    k8s_error_demo_expected_errors: ["count>=1"],
    k8s_error_demo_valid_responses: ["count>=1"],
    k8s_error_demo_backend_replica_hits: ["count>=3"]
  }
};

function constantRateScenario(execName, rate, flow, expectedError) {
  return {
    executor: "constant-arrival-rate",
    exec: execName,
    rate,
    timeUnit: "1s",
    duration,
    preAllocatedVUs: Math.max(20, Math.ceil(rate / 2)),
    maxVUs: Math.max(100, rate * 4),
    tags: { flow: `error_demo_${flow}`, error_demo_expected: String(expectedError) }
  };
}

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
    "error demo health is 200": () => health.status === 200,
    "error demo ready is 200": () => ready.status === 200,
    "error demo event created": () => Boolean(event?.event_id)
  });
  if (!ok) {
    exec.test.abort(`error demo setup failed: health=${health.status} ready=${ready.status}`);
  }
  return { eventId: event.event_id };
}

export function validReadTraffic(data) {
  const selector = exec.scenario.iterationInTest % 4;
  const actorId = employeeId((exec.scenario.iterationInTest % employeeCount) + 1);
  let response;
  if (selector === 0) {
    response = http.get(`${baseUrl}/readyz`, requestParams("valid_read", actorId));
  } else if (selector === 1) {
    response = http.get(`${baseUrl}/api/v1/events`, requestParams("valid_read", actorId));
  } else if (selector === 2 && data?.eventId) {
    response = http.get(`${baseUrl}/api/v1/events/${data.eventId}`, requestParams("valid_read", actorId));
  } else {
    response = http.get(`${baseUrl}/api/v1/me/tickets`, requestParams("valid_read", actorId));
  }
  recordValidResponse(response, "valid read status is 200", (res) => res.status === 200);
}

export function validBookingTraffic(data) {
  if (!data?.eventId) {
    exec.test.abort("valid booking traffic missing eventId");
  }
  const iteration = exec.scenario.iterationInTest;
  const actorId = employeeId((iteration % employeeCount) + 1);
  const response = http.post(
    `${baseUrl}/api/v1/events/${data.eventId}/bookings`,
    JSON.stringify({ idempotency_key: `${runId}-valid-book-${iteration}`, family_count: 0 }),
    requestParams("valid_booking", actorId)
  );
  recordValidResponse(response, "valid booking status is 2xx", (res) => res.status >= 200 && res.status < 300);
}

export function invalidBookingTraffic(data) {
  if (!data?.eventId) {
    exec.test.abort("invalid booking traffic missing eventId");
  }
  const actorId = employeeId((exec.scenario.iterationInTest % employeeCount) + 1);
  const response = http.post(
    `${baseUrl}/api/v1/events/${data.eventId}/bookings`,
    JSON.stringify({ family_count: 0 }),
    requestParams("invalid_booking", actorId, true)
  );
  recordExpectedError(response, "invalid booking rejected", (res) => res.status === 400);
}

export function unauthorizedReadTraffic() {
  const response = http.get(`${baseUrl}/api/v1/me/tickets`, requestParams("unauthorized_read", "", true));
  recordExpectedError(response, "unauthorized read rejected", (res) => res.status === 401 || res.status === 403);
}

export function missingEventTraffic() {
  const actorId = employeeId((exec.scenario.iterationInTest % employeeCount) + 1);
  const missingId = `evt_missing_${runId}_${exec.scenario.iterationInTest}`;
  const response = http.get(`${baseUrl}/api/v1/events/${missingId}`, requestParams("missing_event", actorId, true));
  recordExpectedError(response, "missing event rejected", (res) => res.status === 404);
}

function createHotEvent() {
  const now = Date.now();
  const response = http.post(
    `${baseUrl}/api/v1/admin/events`,
    JSON.stringify({
      title: `k8s error demo ${runId}`,
      description: "K8s error-rate demo event",
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
      tags: ["k8s", "error-demo"],
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
  check(response, { "error demo event create status is 2xx": (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, "error demo event create");
}

function requestParams(flow, actorId = "", expectedError = false) {
  const headers = {
    "Content-Type": "application/json",
    "X-CETS-Benchmark": "k8s-error-demo"
  };
  if (hostHeader) {
    headers.Host = hostHeader;
  }
  if (actorId) {
    headers.Authorization = `Bearer ${providerTokenFor(actorId)}`;
  }
  return {
    headers,
    tags: { flow: `error_demo_${flow}`, error_demo_expected: String(expectedError) }
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
    display_name: actorId === "admin-1" ? "Error Demo Admin" : `Error Demo ${actorId}`,
    role_claims: [role],
    department: actorId === "admin-1" ? "Welfare Committee" : "Engineering",
    site: "Taipei HQ",
    city: "Taipei",
    grade: actorId === "admin-1" ? 7 : 5,
    employment_status: "active",
    exp: Math.floor(Date.now() / 1000) + 3600
  };
}

function recordValidResponse(response, label, predicate) {
  recordReplicas(response);
  validDuration.add(response.timings.duration);
  const ok = check(response, { [label]: predicate });
  if (ok) {
    validResponses.add(1);
  } else {
    unexpectedResponses.add(1);
  }
  sleep(0.01);
}

function recordExpectedError(response, label, predicate) {
  recordReplicas(response);
  expectedErrorDuration.add(response.timings.duration);
  const ok = check(response, { [label]: predicate });
  if (ok) {
    expectedErrors.add(1);
  } else {
    unexpectedResponses.add(1);
  }
  sleep(0.01);
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

function clampRatio(value) {
  if (!Number.isFinite(value)) return 0.4;
  return Math.min(Math.max(value, 0.05), 0.95);
}
