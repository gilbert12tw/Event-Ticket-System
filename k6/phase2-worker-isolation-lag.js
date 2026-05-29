import { check, sleep } from "k6";
import exec from "k6/execution";
import http from "k6/http";

const baseUrl = __ENV.BASE_URL || "http://127.0.0.1:8080";
const p95MaxSeconds = Number(__ENV.K6_OUTBOX_P95_MAX_SECONDS || "60");
const maxSeconds = Number(__ENV.K6_OUTBOX_MAX_SECONDS || "180");
const exportAttempts = Number(__ENV.K6_OUTBOX_EXPORT_ATTEMPTS || "30");
const pollSeconds = Number(__ENV.K6_OUTBOX_POLL_SECONDS || "0.5");
const requiredWorkerKinds = parseRequiredWorkerKinds(__ENV.K6_REQUIRED_WORKER_KINDS || "export");
const providerTokens = {};

export const options = {
  scenarios: {
    workerIsolationLag: {
      executor: "shared-iterations",
      exec: "workerIsolationLag",
      vus: 1,
      iterations: 1,
      maxDuration: "1m",
      tags: { gate: "ws4_worker_isolation_lag" }
    }
  },
  thresholds: {
    checks: ["rate>=1.0"],
    "http_req_failed{gate:ws4_worker_isolation_lag}": ["rate<0.01"],
    "http_req_duration{gate:ws4_worker_isolation_lag}": ["p(95)<1000"]
  }
};

export function setup() {
  const health = http.get(`${baseUrl}/healthz`);
  const ready = http.get(`${baseUrl}/readyz`);
  const seeded = rawPost("/api/v1/admin/seed-demo", {}, actorHeaders("admin-1"));
  const ok = check(null, {
    "worker lag setup health is 200": () => health.status === 200,
    "worker lag setup ready is 200": () => ready.status === 200,
    "worker lag setup seed demo accepted": () => seeded.status === 200
  });
  if (!ok) {
    exec.test.abort(`phase2 worker lag setup failed: health=${health.status} ready=${ready.status} seed=${seeded.status}`);
  }
}

export function workerIsolationLag() {
  const event = createEvent();
  requireValue(event?.event_id, "worker lag notification pressure event id");
  book(event.event_id, "E1001", uniqueKey("notify-e1001"));
  book(event.event_id, "E1002", uniqueKey("notify-e1002"));
  waitForNotificationPressure();

  const reportExport = post(
    "/api/v1/admin/reports/exports",
    { report_type: "participation" },
    "worker lag report export",
    "hr-1"
  );
  const exportId = requireValue(reportExport?.export_id, "worker lag report export id");

  for (let attempt = 0; attempt < exportAttempts; attempt++) {
    assertNonNotificationLag(scrapeMetrics());
    const current = get(`/api/v1/admin/reports/exports/${exportId}`, "worker lag report export status", "hr-1");
    if (current?.status === "ready") {
      const finalMetrics = scrapeMetrics();
      assertNotificationPressure(finalMetrics);
      assertNonNotificationLag(finalMetrics, requiredWorkerKinds);
      check(current, {
        "worker isolation export drained": (value) => value.status === "ready" && Boolean(value.object_key)
      });
      return;
    }
    if (current?.status === "failed") {
      exec.test.abort(`worker isolation export ${exportId} failed`);
    }
    sleep(pollSeconds);
  }

  assertNonNotificationLag(scrapeMetrics());
  exec.test.abort(`worker isolation export ${exportId} did not become ready`);
}

function scrapeMetrics() {
  const response = http.get(`${baseUrl}/metrics`);
  const body = response.body || "";
  const ok = check(response, {
    "metrics scrape status is 200": (res) => res.status === 200,
    "outbox lag histogram is present": () => body.includes("# TYPE cets_outbox_lag_seconds histogram"),
    "outbox backlog gauge is present": () => body.includes("cets_outbox_oldest_lag_seconds"),
    "outbox metrics scrape has no collector error": () =>
      !body.includes('cets_metrics_scrape_errors_total{collector="outbox"') &&
      !body.includes('cets_metrics_scrape_errors_total{collector="outbox_lag_histogram"')
  });
  if (!ok) {
    exec.test.abort("outbox lag metrics scrape failed");
  }
  return body;
}

function assertNonNotificationLag(metrics, requiredWorkerKinds = []) {
  const series = parseOutboxLagBuckets(metrics);
  const backlog = parseOutboxOldestLag(metrics);
  const p95Breaches = [];
  const maxBreaches = [];
  const backlogBreaches = [];
  const observedWorkerKinds = {};

  for (const key of Object.keys(series)) {
    const item = series[key];
    if (item.workerKind === "notification" || item.workerKind === "unknown") {
      continue;
    }
    const total = item.buckets["+Inf"] || 0;
    if (total === 0) {
      continue;
    }
    observedWorkerKinds[item.workerKind] = true;
    const withinP95 = bucketCount(item.buckets, p95MaxSeconds) / total >= 0.95;
    const withinMax = bucketCount(item.buckets, maxSeconds) === total;
    if (!withinP95) {
      p95Breaches.push(`${item.workerKind}/${item.eventType}: ${bucketCount(item.buckets, p95MaxSeconds)}/${total}`);
    }
    if (!withinMax) {
      maxBreaches.push(`${item.workerKind}/${item.eventType}: ${bucketCount(item.buckets, maxSeconds)}/${total}`);
    }
  }
  for (const item of backlog) {
    if (item.workerKind === "notification" || item.workerKind === "unknown") {
      continue;
    }
    if (item.status !== "dead_letter" && item.lagSeconds > p95MaxSeconds) {
      backlogBreaches.push(`${item.workerKind}/${item.status}/${item.eventType}: ${item.lagSeconds}s`);
      continue;
    }
    if (item.lagSeconds > maxSeconds) {
      backlogBreaches.push(`${item.workerKind}/${item.status}/${item.eventType}: ${item.lagSeconds}s`);
    }
  }
  const missingEvidence = requiredWorkerKinds.filter((kind) => !observedWorkerKinds[kind]);

  const ok = check(null, {
    [`non-notification outbox p95 lag <= ${p95MaxSeconds}s`]: () => p95Breaches.length === 0,
    [`non-notification outbox max lag <= ${maxSeconds}s`]: () => maxBreaches.length === 0,
    "non-notification backlog age stays bounded": () => backlogBreaches.length === 0,
    "worker isolation lag evidence is non-empty": () => missingEvidence.length === 0
  });
  if (!ok) {
    exec.test.abort(
      `non-notification outbox lag breached: p95=[${p95Breaches.join("; ")}] ` +
        `max=[${maxBreaches.join("; ")}] backlog=[${backlogBreaches.join("; ")}] ` +
        `missing=[${missingEvidence.join(",")}]`
    );
  }
}

function waitForNotificationPressure() {
  for (let attempt = 0; attempt < exportAttempts; attempt++) {
    const metrics = scrapeMetrics();
    if (notificationBacklogCount(metrics) > 0) {
      assertNotificationPressure(metrics);
      return;
    }
    sleep(pollSeconds);
  }
  exec.test.abort("notification backlog pressure was not observed");
}

function assertNotificationPressure(metrics) {
  const count = notificationBacklogCount(metrics);
  const ok = check(null, {
    "worker isolation notification pressure is present": () => count > 0
  });
  if (!ok) {
    exec.test.abort("notification backlog pressure disappeared before isolation evidence was collected");
  }
}

function notificationBacklogCount(metrics) {
  let count = 0;
  const pattern = /^cets_outbox_pending_total\{([^}]*)\}\s+([0-9.]+)$/gm;
  let match = pattern.exec(metrics);
  while (match !== null) {
    const labels = parseLabels(match[1]);
    if (labels.worker_kind === "notification") {
      count += Number(match[2]);
    }
    match = pattern.exec(metrics);
  }
  return count;
}

function parseOutboxLagBuckets(metrics) {
  const series = {};
  const pattern = /^cets_outbox_lag_seconds_bucket\{([^}]*)\}\s+([0-9.]+)$/gm;
  let match = pattern.exec(metrics);
  while (match !== null) {
    const labels = parseLabels(match[1]);
    const eventType = labels.event_type || "unknown";
    const workerKind = labels.worker_kind || "unknown";
    const le = labels.le || "";
    const key = `${workerKind}\u0000${eventType}`;
    if (!series[key]) {
      series[key] = { eventType, workerKind, buckets: {} };
    }
    series[key].buckets[le] = Number(match[2]);
    match = pattern.exec(metrics);
  }
  return series;
}

function parseOutboxOldestLag(metrics) {
  const rows = [];
  const pattern = /^cets_outbox_oldest_lag_seconds\{([^}]*)\}\s+([0-9.]+)$/gm;
  let match = pattern.exec(metrics);
  while (match !== null) {
    const labels = parseLabels(match[1]);
    rows.push({
      eventType: labels.event_type || "unknown",
      workerKind: labels.worker_kind || "unknown",
      status: labels.status || "unknown",
      lagSeconds: Number(match[2])
    });
    match = pattern.exec(metrics);
  }
  return rows;
}

function parseLabels(labelText) {
  const labels = {};
  const pattern = /([a-zA-Z_]+)="([^"]*)"/g;
  let match = pattern.exec(labelText);
  while (match !== null) {
    labels[match[1]] = match[2];
    match = pattern.exec(labelText);
  }
  return labels;
}

function bucketCount(buckets, limit) {
  const exact = String(limit);
  if (buckets[exact] !== undefined) {
    return buckets[exact];
  }
  let bestLimit = Number.POSITIVE_INFINITY;
  let bestCount = buckets["+Inf"] || 0;
  for (const label of Object.keys(buckets)) {
    if (label === "+Inf") {
      continue;
    }
    const bucketLimit = Number(label);
    if (bucketLimit >= limit && bucketLimit < bestLimit) {
      bestLimit = bucketLimit;
      bestCount = buckets[label];
    }
  }
  return bestCount;
}

function get(path, label, actorId) {
  const response = http.get(`${baseUrl}${path}`, {
    headers: actorHeaders(actorId)
  });
  check(response, { [`${label} status is 200`]: (res) => res.status === 200 });
  return envelopeData(response, label);
}

function post(path, body, label, actorId) {
  const response = rawPost(path, body, actorHeaders(actorId));
  check(response, { [`${label} status is 2xx`]: (res) => res.status >= 200 && res.status < 300 });
  return envelopeData(response, label, response);
}

function createEvent() {
  const suffix = `worker-isolation-${Date.now()}`;
  return post(
    "/api/v1/admin/events",
    {
      title: `k6 WS4 ${suffix}`,
      description: "Worker isolation notification pressure event",
      location: "Taipei HQ",
      capacity: 4,
      status: "published",
      rule: {
        department: "Engineering",
        site: "Taipei HQ",
        min_grade: 5,
        employment_status: "active"
      }
    },
    "worker lag event create",
    "admin-1"
  );
}

function book(eventId, employeeId, idempotencyKey) {
  return post(
    `/api/v1/events/${eventId}/bookings`,
    { idempotency_key: idempotencyKey },
    `worker lag booking ${employeeId}`,
    employeeId
  );
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

function parseRequiredWorkerKinds(raw) {
  return raw
    .split(",")
    .map((value) => value.trim())
    .filter((value) => value.length > 0);
}

function uniqueKey(prefix) {
  const vu = typeof __VU === "undefined" || __VU === 0 ? "setup" : __VU;
  const iter = typeof __ITER === "undefined" ? 0 : __ITER;
  return `${prefix}-${vu}-${iter}-${Date.now()}`;
}
