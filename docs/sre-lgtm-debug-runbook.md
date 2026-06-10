# SRE LGTM+P End-to-End Debug Runbook

This runbook demonstrates how to use Grafana LGTM+P (Loki, Grafana, Tempo, Mimir/Prometheus, Pyroscope)
to diagnose a production incident in the Corporate Event Ticketing System (CETS). It works identically
on Docker Compose (static replica names like `backend-1`) and Kubernetes bare-metal (dynamic pod names
like `backend-5f8c7b7d9-xk2j4`) because all dashboards use `label_values(cets_build_info, replica)` for
auto-discovery.

---

## Observability Audit Summary

### Signal Coverage

| Signal | Stack | Key Instruments |
|--------|-------|-----------------|
| Metrics | Prometheus | HTTP RED, booking pipeline stages (12 stages), reservation pre-admission, Redis gate operations, DB pool/locks, outbox lag/dead-letter, worker retries, rate limits, node USE, container USE, blackbox probes |
| Traces | Tempo | HTTP server spans, booking pipeline spans, DB query spans (`peer.service=postgres`), Redis dependency spans, nginx frontend OTEL spans, service graph + span metrics |
| Logs | Loki | Structured JSON via `slog`, `otel_trace_id`/`otel_span_id` correlation, PII redaction, Loki derived field links to Tempo |
| Profiles | Pyroscope | CPU, allocation, in-use, goroutine, mutex, block profiling; Traces-to-Profiles link in Grafana |

### Dashboard Hierarchy

| Dashboard | Purpose | When to Use |
|-----------|---------|-------------|
| ETS 01 — Golden Signals | First-response landing page | Alert fires, oncall starts here |
| ETS 02 — RED Traffic Drilldown | HTTP layer deep dive | Identify affected route + replica |
| ETS 03 — Booking & Redis Pressure | Booking hot-path + Redis gate | Booking-specific latency/errors |
| ETS 04 — USE Infrastructure | Resource saturation | DB pool, CPU, memory, I/O |
| ETS 05 — Outbox & Worker Health | Async flow health | Dead letters, lag, retries |
| ETS 06 — Service Anomaly Investigation | Full investigation | Combines metrics + traces + logs + profiles |

### Docker Compose vs K8s: Replica Identity

- **Docker Compose**: `CETS_REPLICA_ID=backend-1` (static, set per container)
- **K8s bare-metal**: `CETS_REPLICA_ID` unset → falls back to `HOSTNAME` = pod name (dynamic)
- **Dashboards**: Template variable `label_values(cets_build_info{service=~"$service"}, replica)` auto-discovers all replicas regardless of naming scheme
- **K8s Logs**: `pod` label from Alloy Kubernetes discovery + `app` label from pod labels
- **K8s Traces**: `service.name` from `OTEL_SERVICE_NAME` + `otel_trace_id` for cross-signal correlation

---

## Debug Scenario: Booking Latency Spike During Annual Company Picnic

### 1. Incident Summary

**What happened**: Booking requests for "Annual Company Picnic 2026" (`evt_picnic_2026`) are timing
out. Users see spinner then error page. Some bookings succeed but take 4+ seconds. 2,000 eligible
employees, 500 seat capacity.

**SLO/SLI breached**:
- Latency SLI: p99 booking latency >1s for >5m → alert `CETSHighP99Latency` fires
- Error rate SLI: 5xx rate >1% for >5m → alert `CETSHighHTTPErrorRate` fires

**Impact scope**: All users booking Event ID `evt_picnic_2026`. Other events unaffected.
~300 failed booking attempts in 15 minutes.

**Why metrics alone can't prove root cause**: Metrics show *that* latency is high and *where*
(booking endpoint, specific replicas, `capacity` stage). They show DB pool saturation and lock
waiters rising. But they can't show *which SQL query* is slow, *which function* burns CPU,
*what business logic path* triggers the slowness, or *what changed* to cause it.

---

### 2. Metrics Investigation

**Dashboard: ETS 01 — Golden Signals**

| Metric | Normal | Current | PromQL |
|--------|--------|---------|--------|
| P99 Latency | ~200ms | **4.2s** | `histogram_quantile(0.99, sum by (le)(rate(cets_http_request_seconds_bucket[5m])))` |
| 5xx Error Rate | <0.1% | **8.2%** | `sum(rate(cets_http_requests_total{status_class="5xx"}[5m])) / clamp_min(sum(rate(cets_http_requests_total[5m])), 0.001)` |
| DB Pool Saturation | ~30% | **92%** | `sum(cets_db_pool_conns{state="acquired"}) / clamp_min(sum(cets_db_pool_conns{state="total"}), 1)` |
| Total RPS | ~50 | **120** | `sum(rate(cets_http_requests_total[1m]))` |
| Active Replicas | 3 | 3 | `count(cets_build_info)` |

**Dashboard: ETS 02 — RED Traffic Drilldown** (set `$service=cets-backend`):

```promql
# Which route is affected?
sum by (route, replica) (rate(cets_http_requests_total{status_class="5xx", service=~"$service"}[1m]))
# → POST /api/v1/events/{id}/registrations [backend-2] dominates errors

# Latency by route
histogram_quantile(0.99, sum by (le, route)(rate(cets_http_request_seconds_bucket{service=~"$service"}[5m])))
# → /api/v1/events/{id}/registrations: 4.2s p99 (vs 200ms normal)
```

**Dashboard: ETS 03 — Booking & Redis Pressure**:

```promql
# Which booking stage is the bottleneck?
histogram_quantile(0.95, sum by (le, stage)(rate(cets_booking_stage_seconds_bucket[5m])))
# → stage="capacity": 3.1s p95 (normally <50ms)    ← BOTTLENECK
# → stage="event_lock": 800ms p95 (normally <10ms)  ← SECONDARY (waiting for capacity lock)

# Redis gate health (new panel)
sum by (operation, result) (rate(cets_redis_operation_total[5m]))
# → reserve [ok]: normal rate, reserve [timeout]: 0 → Redis gate is healthy, not the cause

# Reservation outcomes
sum by (outcome) (rate(cets_reservation_attempt_total[5m]))
# → outcome="exhausted" rising, outcome="error" rising
```

**Dashboard: ETS 04 — USE Infrastructure**:

```promql
# DB pool near exhaustion
cets_db_pool_conns{state="acquired"} by (replica)
# → backend-2: 24/25 acquired

# Lock contention
cets_db_lock_waiting_sessions  # → 6 sessions waiting (normally 0)

# Pool acquire wait
rate(cets_db_pool_acquire_wait_seconds_total[5m]) / rate(cets_db_pool_acquire_count_total[5m])
# → 340ms average (normally <5ms)
```

**Label filtering strategy** (same for both Docker Compose and K8s):
- `service=~"$service"` → `cets-backend`, `cets-worker-notification`, etc.
- `replica=~"$replica"` → Docker: `backend-1` / K8s: `backend-5f8c7b7d9-xk2j4`
- `route`, `stage`, `outcome`, `outage_mode` → endpoint and pipeline drill-down

**Metrics conclusion**: Booking `capacity` stage is the bottleneck (3.1s p95). DB pool near-
exhausted with 6 lock-waiting sessions. Isolated to registration endpoint for one event.
Redis gate is healthy. Metrics tell us *where* and *how bad*, not *why*.

---

### 3. Trace Investigation

**Dashboard: ETS 06 — Service Anomaly Investigation, STEP 3**

TraceQL query:
```traceql
{resource.service.name=~"cets-.*" && status=error}
```

Click a slow trace (4.3s). Waterfall shows:

```
cets-edge-lb (nginx)                ── 4.3s total
└─ cets-backend (backend-2)         ── 4.28s
     ├─ booking.preadmission           12ms   ✓
     ├─ booking.begin_tx                3ms   ✓
     ├─ booking.idempotency_lock        8ms   ✓
     ├─ booking.event_lock            780ms   ⚠ (waiting for capacity lock)
     ├─ booking.validate               15ms   ✓
     ├─ booking.duplicate_lookup        5ms   ✓
     ├─ booking.capacity            3,100ms   ❌ CRITICAL PATH
     │    ├─ db.select              2,800ms   ← SLOW QUERY
     │    │   peer.service=postgres
     │    │   cets.db.command_tag="SELECT 847"
     │    └─ db.select                280ms
     ├─ booking.create_response        22ms   ✓
     └─ booking.commit                 45ms   ✓

Span attributes on total span:
  cets.event_id = "evt_picnic_2026"
  cets.booking.stage = "total"
```

**Service Dependency Graph** (Tempo Service Map):
```
cets-edge-lb → cets-backend → postgres
                             → redis (via preadmission)
```

In K8s: "Node Graph" inside the trace shows `service.instance.id` = exact pod name.

**Trace conclusion**: `booking.capacity` contains a `db.select` returning 847 rows in 2.8s.
The `cets.event_id` span attribute confirms which event. The trace shows the exact service
path and bottleneck span, but not which Go function is slow or why 847 rows are returned.

---

### 4. Profile Investigation

**From slow trace → "Profiles for this span"** (Pyroscope link, ±5min window):

CPU Flame Graph (`process_cpu:cpu:nanoseconds`):

```
main.main
└── httpapi.ServeHTTP
    └── ticketing.(*RegistrationsService).CreateRegistration
        └── ticketing.(*RegistrationsService).checkCapacity          68% CPU
            ├── postgres.(*Store).GetEventCapacity                   12%
            ├── ticketing.(*EligibilityChecker).CheckAllRules        42% ← HOTSPOT
            │   ├── ticketing.matchDepartmentRules                    8%
            │   ├── ticketing.matchRoleRules                          6%
            │   └── ticketing.matchSeniorityRules                    28% ← TOP
            │       └── sort.Slice                                   22%
            └── ticketing.(*CapacityCalculator).Recalculate          14%
```

Allocation Profile (`alloc_space`): `matchSeniorityRules → runtime.makeslice` = 340MB in 5min
(O(n^2) slice growth).

Goroutine Profile: 18 goroutines blocked on `pgxpool.(*Pool).Acquire` (confirms DB pool saturation).

**Profile conclusion**: `matchSeniorityRules` consumes 28% CPU with O(n^2) sort inside a loop.
With 847 eligible employees it becomes pathologically slow. The DB query itself is slow due to
a missing index, but the in-memory eligibility re-check is the dominant cost.

---

### 5. Logs Investigation

**Dashboard: ETS 06 — STEP 4**, or from trace → "Logs for this trace":

```logql
# K8s
{namespace="cets", app="backend"} |= `<otel_trace_id>` | json

# Docker Compose
{service="cets-backend"} |= `<otel_trace_id>` | json
```

Key log events:

```json
{"level":"info","msg":"booking attempt","otel_trace_id":"abc123...","event_id":"evt_picnic_2026",
 "action":"registration.create","actor_role":"employee","replica":"backend-2"}

{"level":"warn","msg":"eligibility recheck slow","otel_trace_id":"abc123...",
 "duration_ms":2847,"eligible_count":847,"event_id":"evt_picnic_2026"}

{"level":"error","msg":"db pool acquire timeout","otel_trace_id":"def456...",
 "wait_ms":5023,"replica":"backend-2"}
```

Broader context search:
```logql
{namespace="cets", app="backend"} | json | event_id="evt_picnic_2026" | level=~"warn|error"
```

Timeline from logs:
- 14:02:03 — Event published: `capacity_limit: 500`, `eligible_employees: 847`
- 14:02:15 — First bookings arrive, normal latency
- 14:03:41 — Eligibility recheck warnings begin (duration >1s)
- 14:04:12 — DB pool acquire timeouts start
- 14:04:30 — Rate limiter dropping event-scoped requests
- 14:05:01 — `CETSHighP99Latency` alert fires

**Log conclusion**: Logs confirm `eligible_count=847` — the parameter that makes the eligibility
check expensive. Business context (event_id, actor_role, eligible_count) that traces and metrics lack.

---

### 6. Root Cause

**Evidence chain**:

1. **Metrics** → `booking.capacity` stage 3.1s p95, DB pool 92% saturated, 6 lock waiters
2. **Traces** → `db.select` returning 847 rows in 2.8s inside capacity span, `cets.event_id=evt_picnic_2026`
3. **Profiles** → `matchSeniorityRules` = 28% CPU, O(n^2) sort + 340MB allocations
4. **Logs** → `eligible_count=847`, eligibility recheck warnings, event created with large pool

**Root Cause Statement**:

> `matchSeniorityRules` uses an O(n^2) sort-within-loop that becomes pathologically slow when
> eligible_count exceeds ~500. Event `evt_picnic_2026` has 847 eligible employees. Each booking
> request re-evaluates all 847 records at transaction time (correctness invariant). Under 120 RPS
> concurrent load, this saturated the DB pool and created row lock contention, cascading into 5xx
> timeouts. A missing index on the eligibility lookup contributed 2.8s of the 3.1s capacity-check
> latency.

---

### 7. Fix and Verification

**Immediate mitigation**:
1. `CREATE INDEX CONCURRENTLY idx_eligibility_event_active ON eligibility(event_id, status) WHERE status = 'active'`
2. Increase DB pool size 25→50 temporarily
3. Enable eligibility result caching (30s TTL) if feature flag available

**Permanent fix**:
1. Refactor `matchSeniorityRules`: pre-sorted slice + binary search (O(n log n) + O(log n))
2. Add index to migration scripts permanently
3. Add `cets_eligibility_check_seconds` histogram metric
4. Add integration test with >500 eligible employees

**Post-fix verification**:

| Signal | Check | Expected |
|--------|-------|----------|
| Metrics | `cets_booking_stage_seconds{stage="capacity"}` p95 | <100ms |
| Metrics | `cets_db_pool_conns{state="acquired"}` | <60% of total |
| Metrics | `cets_db_lock_waiting_sessions` | 0 |
| Metrics | `cets_http_request_seconds` p99 | <500ms |
| Metrics | 5xx error rate | <0.1% |
| Traces | `booking.capacity` span duration | <100ms |
| Profiles | `matchSeniorityRules` CPU share | <5% |
| Logs | No `eligibility recheck slow` warnings | Clean |
| Logs | No `db pool acquire timeout` errors | Clean |

---

### 8. Why LGTM+P Works

| Signal | Role | Blind Spot Without It |
|--------|------|-----------------------|
| **Metrics** | What's broken, how bad, when? Aggregated across replicas/time. Triggers alerts. | Can't see *why* — shows `capacity` slow but not what's inside |
| **Traces** | Where in the request path? Follows one request across services, shows span durations. | Can't see *code-level* — shows DB query slow but not which function burns CPU |
| **Profiles** | Which function/resource? Flame graphs at function granularity. | Can't see *business context* — shows hot function but not that eligible_count=847 |
| **Logs** | Exact parameters, errors, business context. | Can't see *patterns* — one log line doesn't show systemic vs isolated |

**The closed investigation loop**:

```
Alert fires (Metrics)
  → Which service/endpoint? (Metrics: RED)
    → Which pipeline stage? (Metrics: booking stages)
      → What does one slow request look like? (Traces: waterfall)
        → Which function burns resources? (Profiles: flame graph)
          → What business parameters caused it? (Logs: eligible_count=847)
            → Root cause: O(n^2) eligibility check + missing index
```

Without any one signal, the investigation stalls:
- No metrics → don't know there's a problem until users complain
- No traces → know something is slow but can't isolate which hop
- No profiles → know the DB call is slow but can't find the O(n^2) algorithm
- No logs → know the function is hot but can't determine eligible_count=847 is the trigger

---

## Appendix A: k6 1000 RPS Workload Generator

The script `k6/k6-lgtm-debug-1000rps.js` generates 1000 RPS of mixed traffic to reproduce
an error-rate spike visible in all four LGTM signals. Traffic breakdown:

| Scenario | RPS | Description | Expected Status |
|----------|-----|-------------|-----------------|
| `readTraffic` | 600 | GET /healthz, /readyz, /api/v1/events, /api/v1/me/tickets | 200 |
| `bookingTraffic` | 250 | POST /api/v1/events/{id}/bookings (valid) | 2xx |
| `invalidBookingTraffic` | 90 | POST bookings without idempotency_key | 400 |
| `unauthorizedTraffic` | 30 | GET /api/v1/me/tickets without auth | 401/403 |
| `missingEventTraffic` | 30 | GET /api/v1/events/{nonexistent} | 404 |

### Running on Docker Compose (phase3-ha)

```bash
# Start the full LGTM stack
docker compose -f services/api/deploy/compose.yaml \
  -f services/api/deploy/compose.phase3-ha.yaml \
  --profile observability up -d

# Run k6 (uses mock-provider-token endpoint, no HMAC secret needed)
k6 run --env BASE_URL=http://127.0.0.1:18080 \
       --env LGTM_DURATION=120s \
       k6/k6-lgtm-debug-1000rps.js

# Open Grafana at http://localhost:3000 → ETS 01 — Golden Signals
```

### Running on K8s Baremetal

**Recommended**: Use the wrapper script, which handles MetalLB IP, host header, K8s secret
extraction, employee seeding, and Docker-based k6 execution automatically:

```bash
# Basic run (1000 RPS, 120s, no 5xx injection)
infra/k8s/baremetal/scripts/73-lgtm-debug-demo.sh

# With failure drill: kills one backend pod mid-test to generate real 5xx
infra/k8s/baremetal/scripts/73-lgtm-debug-demo.sh --with-failure-drill
```

**Manual alternative** (if you need custom options):

```bash
# Port-forward Grafana (if not already exposed)
kubectl port-forward -n observability svc/kube-prometheus-stack-grafana 3000:80 &

# Get the ingress IP or use NodePort
INGRESS_IP=$(kubectl get svc -n ingress-nginx ingress-nginx-controller \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Run k6 with HMAC provider token
k6 run --env BASE_URL=http://${INGRESS_IP} \
       --env K6_HOST_HEADER=cets.local \
       --env K6_PROVIDER_TOKEN_SECRET=$(kubectl get secret -n cets cets-runtime-env \
         -o jsonpath='{.data.PROVIDER_TOKEN_SECRET}' | base64 -d) \
       --env K6_USE_MOCK_PROVIDER=false \
       --env LGTM_DURATION=120s \
       k6/k6-lgtm-debug-1000rps.js
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://127.0.0.1:18080` | Target URL (edge LB / ingress) |
| `K6_HOST_HEADER` | (none) | Host header for ingress routing |
| `K6_PROVIDER_TOKEN_SECRET` | (none) | HMAC secret for K8s; omit for Docker Compose (uses mock endpoint) |
| `K6_USE_MOCK_PROVIDER` | auto | Set `true` to force mock-provider-token endpoint |
| `LGTM_DURATION` | `120s` | Test duration (not `K6_DURATION` — that is a reserved k6 option name) |
| `K6_READ_RPS` | `600` | Read traffic rate |
| `K6_BOOKING_RPS` | `250` | Valid booking traffic rate |
| `K6_INVALID_BOOKING_RPS` | `90` | Invalid booking (400) traffic rate |
| `K6_UNAUTHORIZED_RPS` | `30` | Unauthorized (401) traffic rate |
| `K6_MISSING_EVENT_RPS` | `30` | Missing event (404) traffic rate |
| `K6_HOT_EVENT_CAPACITY` | `500` | Hot event seat capacity |
| `K6_EMPLOYEE_COUNT` | `10000` | Employee pool size |

### What to Observe During the Run

1. **ETS 01 — Golden Signals**: RPS rises to ~1000, error rate climbs as capacity
   exhausts (409 Conflict), p99 latency increases under booking contention.
   5xx appears only when combined with failure drill (`--with-failure-drill`)
2. **ETS 02 — RED Traffic**: POST `/api/v1/events/{id}/bookings` shows highest error rate;
   compare replica distribution
3. **ETS 03 — Booking & Redis**: `preadmission` stage latency rises as Redis gate handles
   1000 RPS; `reservation_attempt_total{outcome="exhausted"}` climbs after capacity fills
4. **ETS 04 — USE Infrastructure**: DB pool saturation and CPU usage under load
5. **ETS 05 — Outbox & Workers**: Outbox pending count rises as booking confirmations
   generate notification/projection events
6. **ETS 06 — Service Anomaly**: Click error traces → waterfall → logs → profiles

### Step-by-Step Debug Walkthrough

1. **Open ETS 01** — Confirm error rate >1% alert threshold is breached
2. **Drill to ETS 02** — Set `$service=cets-backend`, identify which route + replica
3. **Drill to ETS 03** — Check booking stage latencies and Redis gate errors
4. **Open ETS 06 STEP 3** — Find an error trace, click to open waterfall
5. **In Tempo** — Click "Logs for this trace" → see correlated Loki logs
6. **In Tempo** — Click "Profiles for this span" → see CPU flame graph in Pyroscope
7. **Correlate** — Match the trace_id across metrics (span metrics), logs (otel_trace_id),
   and profiles (time window) to reconstruct the full picture

---

## Appendix B: K8s Dynamic Pod Names — Cross-Signal Identity

Kubernetes pod names are dynamic (e.g., `backend-5f8c7b7d9-xk2j4`). Each observability
signal resolves pod identity differently, but all dashboards handle this automatically.

### Identity Resolution by Signal

| Signal | Stable Key | Dynamic Key | How It's Set | Query Example |
|--------|-----------|-------------|--------------|---------------|
| **Metrics** | `service` | `replica` | `CETS_REPLICA_ID` env → falls back to `HOSTNAME` (= pod name via downward API) | `cets_http_requests_total{service="cets-backend", replica=~"$replica"}` |
| **Traces** | `resource.service.name` | Node Graph `service.instance.id` | `OTEL_SERVICE_NAME` env + OTel resource attributes | `{resource.service.name="cets-backend"}` in TraceQL |
| **Logs** | `app` label | `pod` label | Alloy K8s pod discovery: `__meta_kubernetes_pod_name` → `pod` | `{namespace="cets", pod=~"backend-.*"}` in LogQL |
| **Profiles** | `service_name` | `service.instance.id` tag | `PYROSCOPE_APPLICATION_NAME` env | `{service_name="cets-backend"}` in Pyroscope |

### Dashboard Variable Auto-Discovery

All dashboards use two cascading template variables:

```
$service  = label_values(cets_build_info, service)          → stable names
$replica  = label_values(cets_build_info{service=~"$service"}, replica)  → dynamic pod names
```

This works identically for:
- **Docker Compose**: `replica` = `backend-1`, `backend-2`, `backend-3` (static)
- **K8s Baremetal**: `replica` = `backend-5f8c7b7d9-xk2j4` (dynamic, auto-discovered)

### Worker Pods

Worker pods use per-kind service names for clean separation:

| Worker Kind | `OTEL_SERVICE_NAME` | `PYROSCOPE_APPLICATION_NAME` | Metrics `service` |
|-------------|--------------------|-----------------------------|-------------------|
| notification | `cets-worker-notification` | `cets-worker-notification` | `cets-worker-notification` |
| projection | `cets-worker-projection` | `cets-worker-projection` | `cets-worker-projection` |
| compensation | `cets-worker-compensation` | `cets-worker-compensation` | `cets-worker-compensation` |
| export | `cets-worker-export` | `cets-worker-export` | `cets-worker-export` |

### Cross-Signal Correlation

To follow one request across all four signals:

1. **Metrics** → detect anomaly on `service=cets-backend, replica=backend-xxx`
2. **Traces** → `{resource.service.name="cets-backend" && status=error}` → click trace
3. **Logs** → from trace detail, click "Logs for this trace" → Loki filters by `otel_trace_id`
   and shows `pod=backend-xxx` label
4. **Profiles** → from trace detail, click "Profiles for this span" → Pyroscope shows CPU
   flame graph for `service_name=cets-backend` within the span's time window

---

## Appendix C: Observability Coverage & Gap Analysis

### Full Signal Matrix

| Metric Family | Metric Name | Labels | Dashboard |
|---------------|-------------|--------|-----------|
| HTTP RED | `cets_http_requests_total` | service, replica, route, method, status, status_class | 01, 02, 06 |
| HTTP Latency | `cets_http_request_seconds_bucket` | (same as above) | 01, 02, 06 |
| Build Info | `cets_build_info` | service, replica | all (variable source) |
| Booking Stages | `cets_booking_stage_seconds_bucket` | stage (12 values), outcome (11 values) | 03 |
| Reservation | `cets_reservation_attempt_total` | outcome, capacity_type, outage_mode | 03 |
| Pre-admission | `cets_booking_preadmission_seconds_bucket` | outcome, capacity_type, outage_mode | 03 |
| Redis Ops | `cets_redis_operation_total` | operation (8 values), result (3 values) | 03 |
| Redis Latency | `cets_redis_operation_seconds_bucket` | operation, result | 03 |
| Rate Limiting | `cets_rate_limit_drop_total` | scope (actor, event) | 03 |
| Compensation | `cets_reservation_compensation_total` | action, result | 03 |
| Drift Guard | `cets_reservation_counter_drift_total` | result | 03 |
| DB Pool | `cets_db_pool_conns` | pool, state (acquired, idle, total) | 04 |
| DB Wait | `cets_db_pool_acquire_wait_seconds_total` | pool | 04 |
| DB Acquire | `cets_db_pool_acquire_count_total` | pool | 04 |
| DB Locks | `cets_db_lock_waiting_sessions` | — | 04, 06 |
| Outbox Pending | `cets_outbox_pending_total` | event_type, worker_kind, status | 05 |
| Outbox Lag | `cets_outbox_oldest_lag_seconds` | event_type, worker_kind, status | 05 |
| Outbox Histogram | `cets_outbox_lag_seconds_bucket` | event_type, worker_kind | 05 |
| Worker Retries | `cets_worker_retry_total` | worker_kind, reason | 05 |
| Worker DLQ | `cets_worker_deadletter_total` | worker_kind | 05 |
| Scrape Errors | `cets_metrics_scrape_errors_total` | collector | (alert rule) |
| Service Graph | `traces_service_graph_request_total` | client, server | 06 |
| Span Metrics | `traces_spanmetrics_calls_total` | service_name, span_name | 06 |
| Container CPU | `container_cpu_usage_seconds_total` | pod (K8s) / container (Compose) | 04 |
| Container Mem | `container_memory_working_set_bytes` | pod / container | 04 |
| Pod Restarts | `kube_pod_container_status_restarts_total` | pod | 04 |

### Known Gaps

| Area | What's Missing | Impact | Workaround |
|------|---------------|--------|------------|
| Redis cache hit/miss | No per-key hit/miss counter | Cannot measure cache effectiveness | Use `cets_redis_operation_total{result="ok"}` as success proxy |
| SQL query latency | No per-statement histogram in metrics | Cannot rank slow queries from metrics | Dependency tracing spans show per-query latency in Tempo |
| Feature flags | No flag activation metrics | N/A — no feature flag system exists | — |
| Tenant / region | No tenant or region labels | N/A — single-tenant, single-region | — |
| SLO burn-rate | No burn-rate dashboard | Cannot see error budget consumption velocity | Alert rules cover threshold-based SLO; add burn-rate panel if needed |
| Docker Compose base | Tracing + profiling off by default | Dev compose has metrics + logs only | Use `compose.phase3-ha.yaml` overlay for full LGTM+P |
| Synthetic checks (K8s) | No blackbox exporter in K8s | No external HTTP probe validation | Add blackbox exporter via Helm if needed |

### Alert Rules (SLO Thresholds)

| Alert | Condition | For | Severity |
|-------|-----------|-----|----------|
| `CETSHighHTTPErrorRate` | 5xx rate > 1% | 5m | critical |
| `CETSHighP99Latency` | p99 > 1s | 5m | warning |
| `CETSDBPoolAcquireWaitHigh` | avg acquire wait > 50ms | 5m | warning |
| `CETSDBLockWaitingSessions` | lock waiters > 0 | 2m | warning |
| `CETSOutboxOldestLagHigh` | oldest lag > 300s | 5m | warning |
| `CETSMetricsScrapeErrors` | any scrape error | 1m | warning |

---

## Appendix D: 1 分鐘 Demo 流程 (中文)

### 為什麼需要可觀測性？

傳統 monitoring 只能告訴你「系統掛了」，無法回答「為什麼掛」。LGTM+P 五大信號組合
讓你從 **發現問題 → 定位根因** 只需要點擊，不需要 SSH 進機器翻 log。

```
┌─────────────────────────────────────────────────────────────────┐
│ Metrics (Prometheus)  ◄─── span metrics ───►  Traces (Tempo)    │
│       │                                            │    │       │
│  dashboard                              "Logs for  │    │       │
│  drill-down                             this trace"│    │       │
│       ▼                                            ▼    ▼       │
│  01→02→03→04→05                              Loki Logs  Pyroscope│
│  (Golden→RED→Booking→USE→Outbox)               │    CPU flamegraph│
│                                          derived field          │
│                                          link back to Tempo     │
└─────────────────────────────────────────────────────────────────┘
```

### Demo 腳本 (1 分鐘)

> **前提**：已執行 `73-lgtm-debug-demo.sh --with-failure-drill` 產生 1000 RPS 壓力
> 並殺掉一個 Pod 製造真實 5xx。

| 秒數 | 步驟 | 畫面 | 講什麼 |
|------|------|------|--------|
| 0-10 | **STEP 1 — 發現異常** | 開 ETS 01 Golden Signals | 「RPS 升到 1000，error rate 飆升、p99 延遲從 200ms 拉到 4 秒、DB pool 接近飽和。四個黃金指標一眼看出問題範圍。」 |
| 10-20 | **STEP 2 — 縮小範圍** | 點連結到 ETS 02 RED Traffic | 「按 route 拆分：`POST /bookings` 錯誤最高。按 replica 看：特定 Pod 在重啟期間產生 5xx。**Metrics 告訴你 where**。」 |
| 20-30 | **STEP 3 — 追蹤根因** | 切到 ETS 06 → Error Traces 面板 | 「TraceQL 撈出 error trace，點進去看 waterfall：`preadmission 12ms → event_lock 780ms → capacity 3.1s`。**Traces 告訴你 why**。」 |
| 30-40 | **STEP 4 — 關聯 Log** | 在 Tempo trace 詳情點 "Logs for this trace" | 「一鍵跳到 Loki，自動帶入 trace_id 過濾。看到結構化 JSON log 顯示具體的 error message 和 stack。**Log 告訴你 what happened**。」 |
| 40-50 | **STEP 5 — 效能剖析** | 在 Tempo trace 詳情點 "Profiles for this span" | 「跳到 Pyroscope CPU flame graph，看到哪個 function 吃最多 CPU。時間窗口自動對齊 span 範圍。**Profile 告訴你 where the CPU goes**。」 |
| 50-60 | **總結** | 回到 ETS 01 | 「從發現到根因，全程零 SSH、零手動拼 query。四個信號透過 trace_id 自動關聯。這就是 LGTM+P 的完整 debug loop。」 |

### 每步的關鍵技術點

| 步驟 | 信號 | 技術實現 | 為什麼必要 |
|------|------|---------|-----------|
| STEP 1 | Metrics | Prometheus scrape `cets_http_requests_total`, `cets_http_request_seconds_bucket` | 秒級告警、趨勢判斷 |
| STEP 2 | Metrics | 按 `route`, `replica`, `status_class` label 拆分 | 定位到具體 Pod 和 API |
| STEP 3 | Traces | Tempo TraceQL + Alloy OTLP collector | 看到跨服務呼叫鏈和每個 span 耗時 |
| STEP 4 | Logs | Loki `tracesToLogsV2` derived field (`otel_trace_id`) | 用 trace_id 串接，不用手動搜尋 |
| STEP 5 | Profiles | Pyroscope `tracesToProfiles` link | CPU flame graph 定位 hot function |

### 四信號關聯機制

```
Metrics ──────── service, replica labels ────────── 找到哪個 Pod 有問題
    │
Traces ─── otel_trace_id (32 hex) ─── 看到呼叫鏈，點擊進入 Tempo
    │                │
    │          tracesToLogsV2
    │                │
    │                ▼
Logs ──── {namespace="cets"} |= "trace_id" ──── 結構化 JSON + Loki derived field 反向連結 Tempo
    │
Traces ─── tracesToProfiles
    │                │
    │                ▼
Profiles ── process_cpu{service_name="cets-backend"} ── 時間窗口自動對齊 span
```

**關聯的關鍵設定** (在 `kube-prometheus-stack-values.yaml`):
- Loki → Tempo: `derivedFields` regex 抓 `otel_trace_id` → 點擊跳到 Tempo
- Tempo → Loki: `tracesToLogsV2` 用 trace_id 過濾 → 點擊 "Logs for this trace"
- Tempo → Prometheus: `tracesToMetrics` 用 span tags 查 `traces_spanmetrics_calls_total`
- Tempo → Pyroscope: `tracesToProfiles` 用 span 時間窗口查 CPU profile

### Demo 前準備清單

```bash
# 1. 確保 6 個 dashboard 都在 Grafana
kubectl -n observability get configmap cets-k8s-lgtm-dashboard

# 2. 產生壓力 + 5xx (約 2 分半完成)
infra/k8s/baremetal/scripts/73-lgtm-debug-demo.sh --with-failure-drill

# 3. 開 Grafana (如需 port-forward)
kubectl -n observability port-forward svc/kube-prometheus-stack-grafana 3000:80 &

# 4. 瀏覽器開 http://localhost:3000 → Event-Ticket-System 資料夾 → ETS 01
```
