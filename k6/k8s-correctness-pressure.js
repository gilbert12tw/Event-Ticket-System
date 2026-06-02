import { check } from "k6";
import crypto from "k6/crypto";
import encoding from "k6/encoding";
import exec from "k6/execution";
import http from "k6/http";
import { Counter, Trend } from "k6/metrics";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1";
const hostHeader = __ENV.K6_HOST_HEADER || "";
const providerSecret = __ENV.K6_PROVIDER_TOKEN_SECRET || "";
const runId = __ENV.K6_RUN_ID || `${Date.now()}`;
const duration = __ENV.K6_CORRECTNESS_DURATION || "30s";
const employeePrefix = __ENV.K6_EMPLOYEE_PREFIX || "KC";
const bookingEmployees = Number(__ENV.K6_CORRECTNESS_BOOKING_EMPLOYEES || "1200");
const bookingCapacity = Number(__ENV.K6_CORRECTNESS_BOOKING_CAPACITY || "100");
const checkinTickets = Number(__ENV.K6_CORRECTNESS_CHECKIN_TICKETS || "24");
const bookingRate = Number(__ENV.K6_CORRECTNESS_BOOKING_RPS || "200");
const onlineRate = Number(__ENV.K6_CORRECTNESS_ONLINE_RPS || "120");
const offlineRate = Number(__ENV.K6_CORRECTNESS_OFFLINE_RPS || "24");
const mixedOnlineRate = Number(__ENV.K6_CORRECTNESS_MIXED_ONLINE_RPS || "80");
const mixedOfflineRate = Number(__ENV.K6_CORRECTNESS_MIXED_OFFLINE_RPS || "16");
const invalidRate = Number(__ENV.K6_CORRECTNESS_INVALID_RPS || "40");
const offlineBatches = Math.max(1, Number(__ENV.K6_CORRECTNESS_OFFLINE_BATCHES || "8"));

const providerTokens = {};
const correctnessDuration = new Trend("k8s_correctness_duration", true);
const bookingAttempts = new Counter("k8s_correctness_booking_attempts");
const onlineAccepted = new Counter("k8s_correctness_online_accepted");
const onlineDuplicate = new Counter("k8s_correctness_online_duplicate");
const offlineAccepted = new Counter("k8s_correctness_offline_accepted");
const offlineDuplicate = new Counter("k8s_correctness_offline_duplicate");
const offlineConflict = new Counter("k8s_correctness_offline_conflict");
const mixedOnlineAccepted = new Counter("k8s_correctness_mixed_online_accepted");
const mixedOnlineDuplicate = new Counter("k8s_correctness_mixed_online_duplicate");
const mixedOfflineAccepted = new Counter("k8s_correctness_mixed_offline_accepted");
const mixedOfflineDuplicate = new Counter("k8s_correctness_mixed_offline_duplicate");
const mixedOfflineConflict = new Counter("k8s_correctness_mixed_offline_conflict");
const invalidRejected = new Counter("k8s_correctness_invalid_rejected");
const onlineInvalidRejected = new Counter("k8s_correctness_online_invalid_rejected");
const offlineInvalidRejected = new Counter("k8s_correctness_offline_invalid_rejected");
const backendReplicaHits = new Counter("k8s_correctness_backend_replica_hits");

export const options = {
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    bookingPressure: constantRateScenario("bookingPressure", bookingRate, 50, 400, "correctness_booking"),
    onlineDuplicatePressure: constantRateScenario("onlineDuplicatePressure", onlineRate, 50, 400, "correctness_online_checkin"),
    offlineDuplicatePressure: constantRateScenario("offlineDuplicatePressure", offlineRate, 20, 100, "correctness_offline_sync"),
    mixedOnlinePressure: constantRateScenario("mixedOnlinePressure", mixedOnlineRate, 40, 240, "correctness_mixed_online"),
    mixedOfflinePressure: constantRateScenario("mixedOfflinePressure", mixedOfflineRate, 20, 100, "correctness_mixed_offline"),
    invalidTicketPressure: constantRateScenario("invalidTicketPressure", invalidRate, 20, 120, "correctness_invalid")
  },
  thresholds: {
    checks: ["rate>=0.99"],
    "http_req_failed{expected_error:false}": ["rate<0.005"],
    k8s_correctness_duration: ["p(95)<1000", "p(99)<2000"],
    k8s_correctness_booking_attempts: ["count>=1"],
    k8s_correctness_online_accepted: ["count>=1"],
    k8s_correctness_online_duplicate: ["count>=1"],
    k8s_correctness_offline_accepted: ["count>=1"],
    k8s_correctness_offline_duplicate: ["count>=1"],
    k8s_correctness_mixed_online_duplicate: ["count>=1"],
    k8s_correctness_mixed_offline_duplicate: ["count>=1"],
    k8s_correctness_invalid_rejected: ["count>=1"],
    k8s_correctness_online_invalid_rejected: ["count>=1"],
    k8s_correctness_offline_invalid_rejected: ["count>=1"],
    k8s_correctness_backend_replica_hits: ["count>=3"]
  }
};

function constantRateScenario(execName, rate, minVUs, minMaxVUs, flow) {
  return {
    executor: "constant-arrival-rate",
    exec: execName,
    rate,
    timeUnit: "1s",
    duration,
    preAllocatedVUs: Math.max(rate, minVUs),
    maxVUs: Math.max(rate * 4, minMaxVUs),
    tags: { flow, expected_error: "false" }
  };
}

export function setup() {
  if (!providerSecret) {
    exec.test.abort("K6_PROVIDER_TOKEN_SECRET is required");
  }
  const health = http.get(`${baseUrl}/healthz`, requestParams("setup"));
  const ready = http.get(`${baseUrl}/readyz`, requestParams("setup"));
  recordBackendReplica(health);
  recordBackendReplica(ready);
  const ok = check(null, {
    "correctness setup health is 200": () => health.status === 200,
    "correctness setup ready is 200": () => ready.status === 200
  });
  if (!ok) {
    exec.test.abort(`correctness setup failed: health=${health.status} ready=${ready.status}`);
  }

  const bookingEvent = createEvent("booking", bookingCapacity);
  const online = createTicketFixture("online", 1, checkinTickets);
  const offline = createTicketFixture("offline", 1001, checkinTickets);
  const mixed = createTicketFixture("mixed", 2001, checkinTickets);
  const offlinePackages = createOfflinePackages(offline.eventId, "offline", offlineBatches);
  const mixedPackages = createOfflinePackages(mixed.eventId, "mixed", offlineBatches);
  const invalidOfflinePackages = createOfflinePackages(offline.eventId, "invalid-offline", offlineBatches);
  const mixedOnlineWinners = mixed.tokens.slice(0, Math.max(1, Math.floor(mixed.tokens.length / 2)));
  redeemMixedOnlineWinners(mixed.eventId, mixedOnlineWinners);

  return {
    bookingEventId: bookingEvent.event_id,
    onlineEventId: online.eventId,
    mixedEventId: mixed.eventId,
    onlineTokens: online.tokens,
    mixedTokens: mixed.tokens,
    offlineSyncs: offlinePackages.map((pkg, index) => syncRequest(offline.eventId, pkg, offline.tokens, index === 0, true)),
    invalidOfflineSyncs: invalidOfflinePackages.map((pkg) => syncRequest(offline.eventId, pkg, [], true, false)),
    mixedSyncs: mixedPackages.map((pkg) => syncRequest(mixed.eventId, pkg, mixed.tokens, false, false))
  };
}

export function bookingPressure(data) {
  const iteration = exec.scenario.iterationInTest;
  const actorId = employeeId((iteration % bookingEmployees) + 1);
  const response = http.post(
    `${baseUrl}/api/v1/events/${data.bookingEventId}/bookings`,
    JSON.stringify({ idempotency_key: `${runId}-correctness-book-${iteration}`, family_count: 0 }),
    requestParams("correctness_booking", actorId)
  );
  recordBackendReplica(response);
  bookingAttempts.add(1);
  correctnessDuration.add(response.timings.duration);
  check(response, { "booking pressure status is 2xx": (res) => res.status >= 200 && res.status < 300 });
  const payload = envelopeData(response, "booking pressure");
  check(payload?.registration?.status || "", {
    "booking pressure result is confirmed or waitlisted": (status) => status === "confirmed" || status === "waitlisted"
  });
}

export function onlineDuplicatePressure(data) {
  const token = data.onlineTokens[exec.scenario.iterationInTest % data.onlineTokens.length];
  const response = checkIn(token, data.onlineEventId, "online", `k6-${runId}-online`);
  const outcome = classifyCheckIn(response);
  if (outcome === "accepted") {
    onlineAccepted.add(1);
  } else if (outcome === "duplicate") {
    onlineDuplicate.add(1);
  }
}

export function offlineDuplicatePressure(data) {
  const req = data.offlineSyncs[exec.scenario.iterationInTest % data.offlineSyncs.length];
  const response = offlineSync(req, "offline");
  countOfflineResult(response, "offline");
}

export function mixedOnlinePressure(data) {
  const token = data.mixedTokens[exec.scenario.iterationInTest % data.mixedTokens.length];
  const response = checkIn(token, data.mixedEventId, "mixed_online", `k6-${runId}-mixed-online`);
  const outcome = classifyCheckIn(response);
  if (outcome === "accepted") {
    mixedOnlineAccepted.add(1);
  } else if (outcome === "duplicate") {
    mixedOnlineDuplicate.add(1);
  }
}

export function mixedOfflinePressure(data) {
  const req = data.mixedSyncs[exec.scenario.iterationInTest % data.mixedSyncs.length];
  const response = offlineSync(req, "mixed_offline");
  countOfflineResult(response, "mixed");
}

export function invalidTicketPressure(data) {
  const iteration = exec.scenario.iterationInTest;
  if (iteration % 2 === 0) {
    const response = checkIn(`tampered.${runId}.${iteration}`, data.onlineEventId, "invalid_online", `k6-${runId}-invalid-online`);
    if (classifyCheckIn(response) === "invalid") {
      invalidRejected.add(1);
      onlineInvalidRejected.add(1);
    }
    return;
  }
  const baseReq = data.invalidOfflineSyncs[iteration % data.invalidOfflineSyncs.length];
  const req = {
    batch_id: baseReq.batch_id,
    event_id: baseReq.event_id,
    device_id: baseReq.device_id,
    package_signature: baseReq.package_signature,
    scans: [{ signed_token: `tampered.${runId}.${iteration}`, scanned_at: stableScannedAt(9999) }]
  };
  const response = offlineSync(req, "invalid_offline");
  const payload = responsePayload(response);
  if (payload?.conflict > 0 || payload?.results?.some((result) => result.conflict_reason === "invalid_ticket_token")) {
    invalidRejected.add(1);
    offlineInvalidRejected.add(1);
  }
}

function createTicketFixture(kind, employeeStart, count) {
  const event = createEvent(kind, count);
  const tokens = [];
  for (let i = 0; i < count; i += 1) {
    const actorId = employeeId(employeeStart + i);
    const booking = book(event.event_id, actorId, `${runId}-correctness-${kind}-book-${i}`);
    const token = booking?.ticket?.signed_token || "";
    check(token, { [`${kind} fixture ticket token issued`]: (value) => Boolean(value) });
    if (!token) {
      exec.test.abort(`${kind} fixture booking did not issue a ticket at index ${i}`);
    }
    tokens.push(token);
  }
  return { eventId: event.event_id, tokens };
}

function createEvent(kind, capacity) {
  const now = Date.now();
  const response = http.post(
    `${baseUrl}/api/v1/admin/events`,
    JSON.stringify({
      title: `k8s correctness ${runId} ${kind}`,
      description: "K8s pressure correctness benchmark event",
      location: "Taipei HQ",
      event_city: "Taipei",
      event_site: "Taipei HQ",
      starts_at: new Date(now + 7 * 24 * 60 * 60 * 1000).toISOString(),
      registration_start: new Date(now - 60 * 60 * 1000).toISOString(),
      registration_close: new Date(now + 6 * 24 * 60 * 60 * 1000).toISOString(),
      capacity_type: "limited",
      capacity,
      allows_family: false,
      status: "published",
      category: "capacity",
      tags: ["k8s", "correctness"],
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
  recordBackendReplica(response);
  check(response, { [`${kind} event create status is 2xx`]: (res) => res.status >= 200 && res.status < 300 });
  const event = envelopeData(response, `${kind} event create`);
  if (!event?.event_id) {
    exec.test.abort(`${kind} event setup failed`);
  }
  return event;
}

function book(eventId, actorId, idempotencyKey) {
  const response = http.post(
    `${baseUrl}/api/v1/events/${eventId}/bookings`,
    JSON.stringify({ idempotency_key: idempotencyKey, family_count: 0 }),
    requestParams("setup", actorId)
  );
  recordBackendReplica(response);
  check(response, { "fixture booking status is 2xx": (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, "fixture booking");
}

function createOfflinePackage(eventId, deviceId) {
  const response = http.get(
    `${baseUrl}/api/v1/checkins/events/${eventId}/offline-package?device_id=${encodeURIComponent(deviceId)}`,
    requestParams("setup", "staff-1")
  );
  recordBackendReplica(response);
  check(response, { "offline package status is 200": (res) => res.status === 200 });
  const pkg = envelopeData(response, "offline package");
  if (!pkg?.batch_id || !pkg?.package_signature) {
    exec.test.abort(`offline package setup failed for ${eventId}`);
  }
  return pkg;
}

function createOfflinePackages(eventId, kind, count) {
  const packages = [];
  for (let i = 0; i < count; i += 1) {
    packages.push(createOfflinePackage(eventId, `k6-${runId}-${kind}-${i}`));
  }
  return packages;
}

function redeemMixedOnlineWinners(eventId, tokens) {
  tokens.forEach((token, index) => {
    const response = checkIn(token, eventId, "setup_mixed_online_winner", `k6-${runId}-mixed-online`);
    const outcome = classifyCheckIn(response);
    check(outcome, { [`mixed setup online winner ${index} accepted`]: (value) => value === "accepted" });
    if (outcome !== "accepted") {
      exec.test.abort(`mixed online winner setup failed at index ${index}`);
    }
    mixedOnlineAccepted.add(1);
  });
}

function syncRequest(eventId, pkg, tokens, includeInvalid = false, duplicateEachToken = true) {
  const scans = tokens.flatMap((token, index) => {
    const firstScan = { signed_token: token, scanned_at: stableScannedAt(index) };
    if (!duplicateEachToken) {
      return [firstScan];
    }
    return [firstScan, { signed_token: token, scanned_at: stableScannedAt(index + 5000) }];
  });
  if (includeInvalid) {
    scans.push({ signed_token: `tampered.${runId}.offline`, scanned_at: stableScannedAt(9000) });
  }
  return {
    batch_id: pkg.batch_id,
    event_id: eventId,
    device_id: pkg.device_id,
    package_signature: pkg.package_signature,
    scans
  };
}

function stableScannedAt(index) {
  return new Date(Date.UTC(2026, 5, 2, 9, 0, index % 60, 0)).toISOString();
}

function checkIn(token, eventId, flow, deviceId) {
  const response = http.post(
    `${baseUrl}/api/v1/checkins`,
    JSON.stringify({ signed_token: token, event_id: eventId, device_id: deviceId }),
    requestParams(flow, "staff-1", http.expectedStatuses(200, 400, 409))
  );
  recordBackendReplica(response);
  correctnessDuration.add(response.timings.duration);
  check(response, { [`${flow} check-in status is expected`]: (res) => [200, 400, 409].includes(res.status) });
  return response;
}

function offlineSync(req, flow) {
  const response = http.post(
    `${baseUrl}/api/v1/checkins/offline-sync`,
    JSON.stringify(req),
    requestParams(flow, "staff-1", http.expectedStatuses(200, 409))
  );
  recordBackendReplica(response);
  correctnessDuration.add(response.timings.duration);
  check(response, { [`${flow} offline sync status is expected`]: (res) => res.status === 200 || res.status === 409 });
  return response;
}

function countOfflineResult(response, kind) {
  const payload = responsePayload(response);
  if (!payload) {
    return;
  }
  if (payload.accepted > 0) {
    if (kind === "mixed") {
      mixedOfflineAccepted.add(payload.accepted);
    } else {
      offlineAccepted.add(payload.accepted);
    }
  }
  if (payload.duplicate > 0) {
    if (kind === "mixed") {
      mixedOfflineDuplicate.add(payload.duplicate);
    } else {
      offlineDuplicate.add(payload.duplicate);
    }
  }
  if (payload.conflict > 0) {
    if (kind === "mixed") {
      mixedOfflineConflict.add(payload.conflict);
    } else {
      offlineConflict.add(payload.conflict);
    }
  }
  if (payload.results?.some((result) => result.conflict_reason === "invalid_ticket_token")) {
    invalidRejected.add(1);
    offlineInvalidRejected.add(1);
  }
}

function requestParams(flow, actorId = "", expectedStatuses = undefined) {
  const headers = {
    "Content-Type": "application/json",
    "X-CETS-Benchmark": "k8s-correctness"
  };
  if (hostHeader) {
    headers.Host = hostHeader;
  }
  if (actorId) {
    headers.Authorization = `Bearer ${providerTokenFor(actorId)}`;
  }
  const params = {
    headers,
    tags: { flow, expected_error: "false" }
  };
  if (expectedStatuses) {
    params.responseCallback = expectedStatuses;
  }
  return params;
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
  let role = "employee";
  if (actorId === "admin-1") {
    role = "activity_admin";
  } else if (actorId === "staff-1") {
    role = "checkin_staff";
  }
  return {
    employee_id: actorId,
    display_name: actorId,
    role_claims: [role],
    department: role === "employee" ? "Engineering" : "Welfare Committee",
    site: "Taipei HQ",
    city: "Taipei",
    grade: role === "employee" ? 5 : 7,
    employment_status: "active",
    exp: Math.floor(Date.now() / 1000) + 3600
  };
}

function employeeId(index) {
  return `${employeePrefix}${String(index).padStart(6, "0")}`;
}

function responsePayload(response) {
  return envelope(response, "response")?.data || null;
}

function classifyCheckIn(response) {
  const payload = envelope(response, "check-in response");
  const data = payload?.data || {};
  if (response.status >= 200 && response.status < 300 && data.status === "accepted") {
    return "accepted";
  }
  if (
    data.duplicate === true ||
    data.reason_code === "duplicate_scan" ||
    data.conflict_reason === "ticket_already_redeemed" ||
    payload?.error === "ticket has already been redeemed" ||
    payload?.error === "ticket already redeemed"
  ) {
    return "duplicate";
  }
  if (
    data.reason_code === "invalid_ticket_token" ||
    data.conflict_reason === "invalid_ticket_token" ||
    payload?.error === "invalid ticket token"
  ) {
    return "invalid";
  }
  if (response.status === 409) {
    return "conflict";
  }
  return "unexpected";
}

function envelopeData(response, label) {
  return envelope(response, label)?.data || null;
}

function envelope(response, label) {
  try {
    const payload = response.json();
    check(payload, { [`${label} envelope is parsed`]: (body) => Boolean(body) });
    return payload || null;
  } catch {
    check(response, { [`${label} json envelope parsed`]: () => false });
    return null;
  }
}

function recordBackendReplica(response) {
  const replica = response.headers["X-CETS-Backend-Replica"] || response.headers["X-Cets-Backend-Replica"];
  if (replica) {
    backendReplicaHits.add(1, { replica: String(replica).slice(0, 80) });
  }
}
