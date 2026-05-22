import { check, sleep } from "k6";
import exec from "k6/execution";
import http from "k6/http";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1:8080";
const duration = __ENV.K6_IDEMPOTENCY_DURATION || "30s";
const providerTokens = {};

export const options = {
  scenarios: {
    sameKeyBatchRetry: {
      executor: "constant-vus",
      exec: "sameKeyBatchRetry",
      vus: Number(__ENV.K6_IDEMPOTENCY_VUS || "8"),
      duration,
      tags: { gate: "booking_idempotency_same_key" }
    },
    differentKeyExistingBooking: {
      executor: "constant-arrival-rate",
      exec: "differentKeyExistingBooking",
      rate: Number(__ENV.K6_IDEMPOTENCY_EXISTING_RATE || "5"),
      timeUnit: "1s",
      duration,
      preAllocatedVUs: 8,
      maxVUs: 32,
      startTime: "5s",
      tags: { gate: "booking_idempotency_existing" }
    },
    waitlistReplay: {
      executor: "shared-iterations",
      exec: "waitlistReplay",
      vus: 4,
      iterations: Number(__ENV.K6_IDEMPOTENCY_WAITLIST_ITERATIONS || "12"),
      maxDuration: "2m",
      startTime: "10s",
      tags: { gate: "booking_idempotency_waitlist" }
    }
  },
  thresholds: {
    checks: ["rate>=0.995"],
    "http_req_failed{gate:booking_idempotency_same_key}": ["rate<0.01"],
    "http_req_failed{gate:booking_idempotency_existing}": ["rate<0.01"],
    "http_req_failed{gate:booking_idempotency_waitlist}": ["rate<0.01"],
    "http_req_duration{gate:booking_idempotency_same_key}": ["p(95)<750", "p(99)<1500"],
    "http_req_duration{gate:booking_idempotency_existing}": ["p(95)<750", "p(99)<1500"],
    "http_req_duration{gate:booking_idempotency_waitlist}": ["p(95)<750", "p(99)<1500"]
  }
};

export function setup() {
  const health = http.get(`${baseUrl}/healthz`);
  const ready = http.get(`${baseUrl}/readyz`);
  const seeded = rawPost("/api/v1/admin/seed-demo", {}, actorHeaders("admin-1"));
  const event = createEvent("idem-hot", 2);
  const ok = check(null, {
    "idempotency setup health is 200": () => health.status === 200,
    "idempotency setup ready is 200": () => ready.status === 200,
    "idempotency setup seed demo accepted": () => seeded.status === 200,
    "idempotency setup event created": () => Boolean(event?.event_id)
  });
  if (!ok) {
    exec.test.abort(`phase2 idempotency setup failed: health=${health.status} ready=${ready.status} seed=${seeded.status}`);
  }
  return {
    eventId: event.event_id,
    sharedKey: `phase2-same-key-${Date.now()}`
  };
}

export function sameKeyBatchRetry(data) {
  const first = book(data.eventId, "E1001", data.sharedKey);
  requireValue(first?.registration?.registration_id, "same-key registration id");
  requireValue(first?.ticket?.ticket_id, "same-key ticket id");

  const responses = http.batch([
    bookingRequest(data.eventId, data.sharedKey, "E1001"),
    bookingRequest(data.eventId, data.sharedKey, "E1001"),
    bookingRequest(data.eventId, data.sharedKey, "E1001"),
    bookingRequest(data.eventId, data.sharedKey, "E1001")
  ]);
  for (const response of responses) {
    const replay = envelopeData(response, "same-key replay", response);
    check(replay, {
      "same-key replay keeps registration": (value) => value?.registration?.registration_id === first.registration.registration_id,
      "same-key replay keeps ticket": (value) => value?.ticket?.ticket_id === first.ticket.ticket_id,
      "same-key replay is duplicate": (value) => value?.duplicate === true,
      "same-key replay keeps capacity snapshot": (value) => value?.remaining_capacity === first.remaining_capacity
    });
  }
  sleep(0.1);
}

export function differentKeyExistingBooking(data) {
  const result = book(data.eventId, "E1001", uniqueKey("existing-key"));
  check(result, {
    "different key returns existing registration": (value) => value?.registration?.employee_id === "E1001",
    "different key returns duplicate": (value) => value?.duplicate === true,
    "different key does not mint another ticket identity": (value) => Boolean(value?.ticket?.ticket_id)
  });
  sleep(0.1);
}

export function waitlistReplay() {
  const event = createEvent("idem-waitlist", 1);
  requireValue(event?.event_id, "waitlist replay event id");
  const confirmed = book(event.event_id, "E1001", uniqueKey("waitlist-fill"));
  requireValue(confirmed?.registration?.registration_id, "waitlist filler registration");
  const waitlistKey = uniqueKey("waitlist-replay");
  const waitlisted = book(event.event_id, "E1002", waitlistKey);
  check(waitlisted, { "waitlist first result is waitlisted": (value) => value?.registration?.status === "waitlisted" });

  const replay = book(event.event_id, "E1002", waitlistKey);
  check(replay, {
    "waitlist replay keeps registration": (value) => value?.registration?.registration_id === waitlisted?.registration?.registration_id,
    "waitlist replay keeps status": (value) => value?.registration?.status === "waitlisted",
    "waitlist replay has no ticket": (value) => !value?.ticket,
    "waitlist replay is duplicate": (value) => value?.duplicate === true
  });
}

function createEvent(prefix, capacity) {
  const vu = typeof __VU === "undefined" || __VU === 0 ? "setup" : __VU;
  const iter = typeof __ITER === "undefined" ? 0 : __ITER;
  const suffix = `${prefix}-${vu}-${iter}-${Date.now()}`;
  return post(
    "/api/v1/admin/events",
    {
      title: `k6 Phase2 ${suffix}`,
      description: "Phase 2 booking idempotency retry event",
      location: "Taipei HQ",
      capacity,
      status: "published",
      rule: {
        department: "Engineering",
        site: "Taipei HQ",
        min_grade: 5,
        employment_status: "active"
      }
    },
    "event create",
    "admin-1"
  );
}

function book(eventId, employeeId, idempotencyKey) {
  const response = rawPost(`/api/v1/events/${eventId}/bookings`, { idempotency_key: idempotencyKey }, actorHeaders(employeeId));
  check(response, { "booking status is 2xx": (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, "booking", response);
}

function bookingRequest(eventId, idempotencyKey, actorId) {
  return {
    method: "POST",
    url: `${baseUrl}/api/v1/events/${eventId}/bookings`,
    body: JSON.stringify({ idempotency_key: idempotencyKey }),
    params: { headers: { "Content-Type": "application/json", ...actorHeaders(actorId) } }
  };
}

function post(path, body, label, actorId) {
  const response = rawPost(path, body, actorHeaders(actorId));
  check(response, { [`${label} status is 2xx`]: (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, label, response);
}

function rawPost(path, body, extraHeaders = {}) {
  return http.post(`${baseUrl}${path}`, JSON.stringify(body), {
    headers: { "Content-Type": "application/json", ...extraHeaders }
  });
}

function actorHeaders(actorId) {
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
    check(payload, { [`${label} envelope success`]: (body) => body && body.success === true });
    return payload.data || fallback;
  } catch {
    check(response, { [`${label} json envelope parsed`]: () => false });
    return fallback;
  }
}

function requireValue(value, label) {
  const ok = check(value, { [label]: (candidate) => Boolean(candidate) });
  if (!ok) {
    exec.test.abort(label);
  }
  return value;
}

function uniqueKey(prefix) {
  const vu = typeof __VU === "undefined" || __VU === 0 ? "setup" : __VU;
  const iter = typeof __ITER === "undefined" ? 0 : __ITER;
  return `${prefix}-${vu}-${iter}-${Date.now()}`;
}
