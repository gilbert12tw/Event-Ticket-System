import { check, sleep } from "k6";
import exec from "k6/execution";
import http from "k6/http";
import { Counter } from "k6/metrics";

const baseUrl = __ENV.BASE_URL || __ENV.EDGE_URL || "http://127.0.0.1:18080";
const directBackendUrl = __ENV.DIRECT_BACKEND_URL || "";
const profile = __ENV.K6_PHASE3_PROFILE || "smoke";
const providerTokens = {};

const gatewayReplicaHits = new Counter("phase3_gateway_replica_hits");
const frontendReplicaHits = new Counter("phase3_frontend_replica_hits");
const backendReplicaHits = new Counter("phase3_backend_replica_hits");
const controlledErrors = new Counter("phase3_controlled_errors");
const investigationBackendHits = new Counter("phase3_investigation_backend_hits");

const durations = {
  smoke: __ENV.K6_PHASE3_DURATION || "20s",
  stress: __ENV.K6_PHASE3_DURATION || "2m",
  investigate: __ENV.K6_PHASE3_DURATION || "45s"
};

export const options = {
  scenarios: scenarioOptions(profile),
  thresholds: {
    checks: ["rate>=0.99"],
    "http_req_failed{phase3_expected_error:false}": ["rate<0.02"],
    "phase3_gateway_replica_hits": ["count>=3"],
    "phase3_frontend_replica_hits": ["count>=3"],
    "phase3_backend_replica_hits": ["count>=3"],
    "phase3_controlled_errors": ["count>=1"]
  }
};

export function setup() {
  const health = http.get(`${baseUrl}/healthz`, requestParams("setup"));
  const ready = http.get(`${baseUrl}/readyz`, requestParams("setup"));
  recordReplicas(health);
  recordReplicas(ready);
  const seeded = rawPost(baseUrl, "/api/v1/admin/seed-demo", {}, actorHeaders("admin-1"), "setup");
  recordReplicas(seeded);
  const event = createEvent("phase3-core", 250);
  const ok = check(null, {
    "phase3 setup health is 200": () => health.status === 200,
    "phase3 setup ready is 200": () => ready.status === 200,
    "phase3 setup seed accepted": () => seeded.status === 200,
    "phase3 setup event created": () => Boolean(event?.event_id)
  });
  if (!ok) {
    exec.test.abort(`phase3 setup failed: health=${health.status} ready=${ready.status} seed=${seeded.status}`);
  }
  return { eventId: event.event_id };
}

export function frontendReplicaTraffic(data) {
  const paths = ["/", "/user/events", "/user/tickets", "/admin/checkin"];
  const response = http.get(`${baseUrl}${paths[(__VU + __ITER) % paths.length]}`, requestParams("frontend"));
  recordReplicas(response);
  check(response, { "frontend traffic status is 200": (res) => res.status === 200 });
  if (__ITER % 3 === 0) {
    get(baseUrl, "/readyz", "frontend readyz", "E1001");
  }
  if (data?.eventId && __ITER % 5 === 0) {
    get(baseUrl, `/api/v1/events/${data.eventId}`, "frontend event detail", "E1001");
  }
  sleep(0.1);
}

export function backendCoreFlow(data) {
  get(baseUrl, "/api/v1/events", "event browse", "E1001");
  if (data?.eventId) {
    get(baseUrl, `/api/v1/events/${data.eventId}`, "event detail", "E1001");
    if (__ITER % 4 === 0) {
      book(data.eventId, "E1001", uniqueKey("phase3-book"));
    }
  }
  if (__ITER % 5 === 0) {
    get(baseUrl, "/api/v1/me/tickets", "ticket list", "E1001");
  }
  if (__ITER % 8 === 0) {
    get(baseUrl, "/api/v1/admin/audit-logs?limit=5", "audit list", "hr-1");
  }
  sleep(0.1);
}

export function controlledErrorTraffic(data) {
  const unknownProfile = http.post(`${baseUrl}/api/v1/auth/mock-provider-token`, JSON.stringify({ profile_id: "unknown" }), {
    headers: { "Content-Type": "application/json" },
    tags: { phase3_flow: "controlled_error", phase3_expected_error: "true" }
  });
  recordReplicas(unknownProfile);
  countExpectedError(unknownProfile, "unknown profile", 401);

  if (data?.eventId) {
    const invalidBooking = rawPost(baseUrl, `/api/v1/events/${data.eventId}/bookings`, {}, actorHeaders("E1001"), "controlled_error", true);
    recordReplicas(invalidBooking);
    countExpectedError(invalidBooking, "invalid booking", 400);
  }
  sleep(0.2);
}

export function investigateBackendHotspot(data) {
  const url = directBackendUrl || baseUrl;
  const response = http.get(`${url}/readyz`, requestParams("investigate"));
  recordReplicas(response);
  investigationBackendHits.add(1);
  check(response, { "investigation backend readyz status is 200": (res) => res.status === 200 });
  if (data?.eventId) {
    get(url, `/api/v1/events/${data.eventId}`, "investigation event detail", "E1001");
  }
  sleep(0.05);
}

function scenarioOptions(selectedProfile) {
  const duration = durations[selectedProfile] || durations.smoke;
  const base = {
    frontendReplicaTraffic: {
      executor: "constant-vus",
      exec: "frontendReplicaTraffic",
      vus: Number(__ENV.K6_PHASE3_FRONTEND_VUS || (selectedProfile === "stress" ? "8" : "2")),
      duration,
      tags: { phase3_flow: "frontend", phase3_expected_error: "false" }
    },
    backendCoreFlow: {
      executor: "constant-vus",
      exec: "backendCoreFlow",
      vus: Number(__ENV.K6_PHASE3_BACKEND_VUS || (selectedProfile === "stress" ? "12" : "3")),
      duration,
      tags: { phase3_flow: "backend_core", phase3_expected_error: "false" }
    },
    controlledErrorTraffic: {
      executor: "constant-arrival-rate",
      exec: "controlledErrorTraffic",
      rate: Number(__ENV.K6_PHASE3_ERROR_RATE || "1"),
      timeUnit: "1s",
      duration,
      preAllocatedVUs: 2,
      maxVUs: 8,
      tags: { phase3_flow: "controlled_error", phase3_expected_error: "true" }
    }
  };
  if (selectedProfile === "investigate") {
    base.investigateBackendHotspot = {
      executor: "constant-vus",
      exec: "investigateBackendHotspot",
      vus: Number(__ENV.K6_PHASE3_INVESTIGATE_VUS || "10"),
      duration,
      tags: { phase3_flow: "investigate_backend", phase3_expected_error: "false" }
    };
  }
  return base;
}

function createEvent(prefix, capacity) {
  const suffix = `${prefix}-${Date.now()}`;
  return post(baseUrl, "/api/v1/admin/events", {
    title: `k6 Phase3 ${suffix}`,
    description: "Phase 3 Compose HA LGTM load event",
    location: "Taipei HQ",
    capacity,
    status: "published",
    rule: {
      department: "Engineering",
      site: "Taipei HQ",
      min_grade: 5,
      employment_status: "active"
    }
  }, "event create", "admin-1");
}

function book(eventId, actorId, idempotencyKey) {
  return post(baseUrl, `/api/v1/events/${eventId}/bookings`, { idempotency_key: idempotencyKey }, "booking", actorId);
}

function get(url, path, label, actorId) {
  const response = http.get(`${url}${path}`, {
    headers: actorHeaders(actorId),
    tags: { phase3_flow: label, phase3_expected_error: "false" }
  });
  recordReplicas(response);
  check(response, { [`${label} status is 200`]: (res) => res.status === 200 });
  return envelopeData(response, label, response);
}

function post(url, path, body, label, actorId) {
  const response = rawPost(url, path, body, actorHeaders(actorId), label);
  recordReplicas(response);
  check(response, { [`${label} status is 2xx`]: (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, label, response);
}

function rawPost(url, path, body, extraHeaders = {}, label = path, expectedError = false) {
  return http.post(`${url}${path}`, JSON.stringify(body), {
    headers: { "Content-Type": "application/json", ...extraHeaders },
    tags: { phase3_flow: label, phase3_expected_error: String(expectedError) }
  });
}

function requestParams(flow) {
  return {
    tags: { phase3_flow: flow, phase3_expected_error: "false" }
  };
}

function actorHeaders(actorId) {
  return { Authorization: `Bearer ${providerTokenFor(actorId)}` };
}

function providerTokenFor(actorId) {
  if (providerTokens[actorId]) return providerTokens[actorId];
  const response = http.post(`${baseUrl}/api/v1/auth/mock-provider-token`, JSON.stringify({ profile_id: actorId }), {
    headers: { "Content-Type": "application/json" },
    tags: { phase3_flow: "mock_provider_token", phase3_expected_error: "false" }
  });
  recordReplicas(response);
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

function countExpectedError(response, label, status) {
  const ok = check(response, { [`${label} expected status ${status}`]: (res) => res.status === status });
  if (ok) {
    controlledErrors.add(1);
  }
}

function recordReplicas(response) {
  addReplicaHit(gatewayReplicaHits, response.headers["X-Cets-Gateway-Replica"] || response.headers["X-CETS-Gateway-Replica"]);
  addReplicaHit(frontendReplicaHits, response.headers["X-Cets-Frontend-Replica"] || response.headers["X-CETS-Frontend-Replica"]);
  addReplicaHit(backendReplicaHits, response.headers["X-Cets-Backend-Replica"] || response.headers["X-CETS-Backend-Replica"]);
}

function addReplicaHit(counter, replica) {
  if (!replica) return;
  counter.add(1, { replica: String(replica).slice(0, 80) });
}

function uniqueKey(prefix) {
  return `${prefix}-${__VU}-${__ITER}-${Date.now()}`;
}
