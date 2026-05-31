# Phase 2 WS2 — Load and Observability

> Workstream-level spec. Parent: `COR-40` / `[PH2-WS2]`. Owner: Person B.
> Anchors: `docs/specs/phase2-scale-hardening.md` (§1, §3.2, §4), Phase 1 baseline `docs/specs/phase1-production-upper-bound.md`, k6 reference `k6/phase1-production-gate.js`.

## 1. Summary

WS2 owns the evidence base for Phase 2: realistic 50k employee load seed, single-hot-event k6 profile, threshold gate, DB lock / pool wait metrics, queue lag + worker metrics, structured trace/log schema for Phase 2 hops, baseline report that pinpoints the actual bottleneck (registration tx time vs DB lock wait vs DB pool wait vs browse latency vs outbox lag vs worker throughput vs reporting aggregation vs read model freshness), and CI wiring for regression. WS2 does not tune the hot path or change runtime behavior — it produces measurements that WS3/WS4/WS5 use to decide what to change.

WS2 starts Wave 1 in parallel with WS3/WS4/WS5 spec work; the baseline report (`PH2-16`) gates WS3 implementation (`PH2-22`, `PH2-23`, `PH2-25`).

## 2. Scope

In scope (`PH2-10`..`PH2-17`):

- 50k employee load seed command (idempotent, repeatable, deterministic).
- Single-hot-event k6 profile: one limited event of bounded capacity, multiple eligible employees competing — distinct from Phase 1 fan-out across many events.
- k6 threshold gate aligned to acceptance matrix §1.1/§1.3: 270 RPS, 35 booking TPS, 2,400 active VUs; p95 / p99 latency thresholds.
- DB instrumentation: `pgx` pool wait time, lock wait time, transaction duration histogram, statement timeout counter.
- Queue/worker instrumentation: outbox lag (`occurred_at` → `processed_at`), per-kind processing time, retry count, dead-letter count.
- Trace/log schema for Phase 2 hops: request → Redis pre-admission → DB tx → outbox enqueue → worker pickup → SMTP / projection write. One `trace_id` end-to-end.
- Capacity baseline report (`PH2-16`): markdown artifact in `docs/specs/` (or `k6/reports/`) naming the bottleneck with measurements.
- Performance regression CI wiring: thresholds fail CI with non-zero exit; mode toggle for smoke vs full gate.

Out of scope (other WS):

- Redis reservation gate, idempotency hardening, contention reduction — WS3.
- Worker kind split, replay, dead-letter, isolation — WS4.
- Read model freshness contract, ops UI — WS5.
- Acceptance matrix, OpenAPI delta, event envelope — WS1.

## 3. Acceptance Criteria

| AC | Given | When | Then |
| --- | --- | --- | --- |
| WS2-AC-1 | A fresh local Compose DB | `cets seed --profile=phase2-50k` (or equivalent admin process) runs | 50,000 employees + eligibility rules + event fixtures land deterministically; re-running is safe (`ON CONFLICT DO UPDATE`) and produces identical row counts. |
| WS2-AC-2 | The hot-event k6 profile runs against a primed Compose stack | One limited event with bounded capacity is targeted | The script generates booking pressure that exceeds capacity, exercising oversell-prevention paths, with no implicit fan-out across other events. |
| WS2-AC-3 | k6 threshold gate runs | Phase 2 capacity targets (§1.1) are applied | Thresholds for HTTP p95/p99, booking HTTP p95/p99, RPS, TPS, error rate are encoded and the run exits non-zero on any breach. |
| WS2-AC-4 | The app is built with WS2 metrics wired | Booking pressure runs | DB pool wait, lock wait, tx duration, statement timeout counter are exported and visible (Prometheus scrape or `/metrics` endpoint) without requiring a code reread. |
| WS2-AC-5 | The worker runs against backed-up outbox | Lag accumulates | Outbox lag p95 / max, per-kind processing time, retry count, dead-letter count are exported and labeled by `event_type` and `worker_kind`. |
| WS2-AC-6 | A booking request enters the system | Trace is followed | A single `trace_id` correlates HTTP request → Redis call → DB tx → outbox enqueue → worker pickup → SMTP send / projection write, in structured stdout logs (no PII / signed tokens). |
| WS2-AC-7 | `PH2-16` baseline report is produced | Reviewer reads it | Report names the dominant bottleneck among {registration tx time, DB lock wait, DB pool wait, browse/detail latency, outbox lag, worker throughput, reporting aggregation, read model freshness} with measurements, and recommends which WS3/WS4/WS5 issue is unblocked. |
| WS2-AC-8 | CI runs the Phase 2 performance gate | Thresholds regress beyond a configured tolerance | Job fails with a non-zero exit and a link to the breach summary. |

## 4. Edge Cases

| # | Scenario | Expected behavior |
| --- | --- | --- |
| WS2-E-1 | Seed run interrupted mid-batch | Re-run completes from scratch idempotently; partial state never blocks a clean run. |
| WS2-E-2 | k6 hot profile accidentally spreads load across many events | Threshold gate detects abnormally low contention (e.g. tx duration too low, oversell-prevention paths cold); report flags fixture drift. |
| WS2-E-3 | Trace context dropped at worker handoff | Worker enqueue/pickup logs a missing-trace-id metric; not fatal but visible in baseline report. |
| WS2-E-4 | Metric cardinality explodes (e.g. `event_id` label on every histogram) | Metrics are bucketed by `worker_kind` / `event_type` / `outcome`; high-cardinality labels are documented as opt-in. |
| WS2-E-5 | k6 gate fails because Compose has fewer cores than CI runner | Profile encodes runner expectations; gate documents minimum CPU/RAM and skips on under-resourced runners with a warning rather than a spurious fail. |
| WS2-E-6 | Baseline report identifies no clear bottleneck (everything roughly equal) | Report says so explicitly and recommends a follow-up profile rather than greenlighting hot-path changes. |
| WS2-E-7 | Worker isolation (`PH2-32`) ships before WS2 per-kind metrics | Metrics adapter is backward-compatible: missing `worker_kind` label degrades to `worker_kind=unknown`. |

## 5. Non-Functional Requirements

| Category | Requirement | Metric |
| --- | --- | --- |
| Reproducibility | Seed + k6 profile run from clean Compose with one command sequence. | `dc up -d` → `cets seed --profile=phase2-50k` → `k6 run k6/phase2-hot-event.js` succeeds in CI and locally. |
| Realism | Hot profile creates real contention on one limited event. | Booking oversell-prevention paths fire; tx duration distribution is non-trivial. |
| Cost | Metric cardinality bounded. | Each new histogram ≤ small number of label combinations; documented in `PH2-15`. |
| No-runtime-truth | Metrics never become the authority for correctness (booking truth stays in PostgreSQL). | Reviewer checklist item. |
| Decision-quality | Baseline report is enough to decide next WS3/WS4/WS5 issue. | `PH2-16` cited by every WS3/WS4/WS5 implementation PR. |

## 6. Minimal API / Data Contract

WS2 surfaces metrics and traces; no new product API. Metric / log shape (consumed by WS5 ops UI and reviewers):

Current implementation slice:

- `GET /metrics` exposes Prometheus text format from the existing app process.
- HTTP RED metrics use route pattern, method, and status class labels; raw IDs, signed tokens, QR payloads, and provider tokens must not appear in labels.
- PostgreSQL pool acquire wait, current lock-waiting sessions, outbox backlog / oldest lag, and terminal outbox publish latency are scrapeable as white-box signals for the Phase 2 baseline.
- Optional local pipeline: `docker compose --profile observability ... up` starts Prometheus, VictoriaMetrics, Alertmanager, Grafana, blackbox exporter, Redis exporter, node exporter, cAdvisor, Loki, Promtail, Tempo, and Pyroscope with provisioned Grafana datasources. This is a review/demo profile, not a required production backing service.
- Prometheus keeps short local retention (`--storage.tsdb.retention.time=2d`) and remote-writes scraped and recorded series to optional VictoriaMetrics (`-retentionPeriod=30d`) for local long-term trend review. Recording rules in `observability/rules/cets-rollups.yml` create 5-minute RED, DB lock, and outbox lag rollups so older trend queries can use lower-granularity series.
- `observability/rules/cets-service-level.yml` records starter service-level indicators, SLO target series, and error-budget burn/remaining-ratio series for the API success-rate and P99-latency objectives. These are queryable review/demo series only; they do not enforce production SLA policy or change request handling.
- Loki stores app/worker stdout logs collected by Promtail from the Docker socket, keeping the application log contract as stdout/stderr only.
- Tempo exposes local OTLP gRPC/HTTP receivers for trace ingestion. The app can opt in to route-bounded HTTP server spans with `OTEL_TRACES_ENABLED=true`; the default remains off so product traffic does not depend on the trace backend. When tracing is enabled, app request logs include `otel_trace_id` / `otel_span_id`, and Grafana provisions a Loki derived field that opens the matching Tempo trace.
- Pyroscope exposes a local continuous profiling backend. The app can opt in with `PYROSCOPE_ENABLED=true`; the default remains off so product traffic does not depend on the profile backend.
- Black-box probing is provided by optional `blackbox-exporter`; Prometheus probes `http://app:8080/`, `http://app:8080/healthz`, and `http://app:8080/readyz` through `/probe` to represent the external symptom view separately from in-process white-box metrics. Probes do not call product APIs and do not change booking, check-in, worker, or reporting behavior.
- Redis backing-service metrics are provided by optional `redis-exporter`; Prometheus scrapes Redis memory and client metrics for review/demo visibility without changing Redis reservation semantics, Redis config, or app/worker runtime dependencies.
- Host and container USE metrics are provided by optional `node-exporter` and `cadvisor` services. Prometheus scrapes them for CPU, memory, disk, network, and container saturation signals only; app and worker do not depend on these exporters and expose no new product API for them.
- Grafana provisions a `CETS Golden Signals` dashboard that maps the Google SRE signals to existing telemetry: traffic from request rate, errors from 4xx/5xx ratio, latency split into success and error P99 panels, and saturation from DB pool wait, DB locks, and outbox lag. This reuses existing scrape data only; no product path, handler, or worker behavior changes.
- `services/api/deploy/reference/kubernetes-observability/` keeps reference-only Kubernetes observability examples for orchestration-state, container metrics, alert routing, container log pipeline visibility, long-term metrics storage alternatives, application lifecycle/networking automation, and vendor-neutral telemetry routing in a future container-platform decision gate. They cover kube-state-metrics, node exporter as a DaemonSet, app-specific exporter, Alertmanager local-review and PagerDuty-style receiver routing, Fluent Bit / Fluentd / Vector log collector examples, Prometheus and Loki as StatefulSets with persistent storage, Grafana as a stateless Deployment, Thanos Receive / Cortex / Grafana Agent remote-write reference shapes, Elasticsearch/Kibana EFK examples, Logstash/OpenSearch log analysis alternatives, scrape examples, rolling updates, health probes, HPA scaling, stable Service names, PodDisruptionBudget, node-spread hints, an OpenTelemetry Collector OTLP entry point that routes traces to Tempo and logs to Loki, and Jaeger/Zipkin trace backend alternatives. The examples are not loaded by the local Compose profile and do not change app, worker, booking, check-in, reporting, or audit behavior.
- Prometheus loads starter alert rules from `services/api/deploy/observability/rules/` and routes them to the local Alertmanager `local-review` receiver. This adds version-controlled SLO breach detection and local alert grouping for PR #43 metrics only; it does not add paging, PagerDuty secrets, or a production monitoring dependency.

```text
current shipped metrics (prometheus-style):
  cets_http_requests_total{route, method, status_class}
  cets_http_request_seconds_bucket{route, method, status_class}
  cets_db_pool_acquire_wait_seconds_total
  cets_db_pool_acquire_count_total
  cets_db_pool_conns{state}
  cets_db_lock_waiting_sessions
  cets_outbox_pending_total{event_type, worker_kind, status}
  cets_outbox_lag_seconds_bucket{event_type, worker_kind}
  cets_outbox_lag_seconds_sum{event_type, worker_kind}
  cets_outbox_lag_seconds_count{event_type, worker_kind}
  cets_outbox_oldest_lag_seconds{event_type, worker_kind, status}
  cets_outbox_retry_count{event_type, worker_kind, status}
  cets_outbox_dead_letter_total{event_type, worker_kind}
  cets_worker_retry_total{worker_kind, reason}
  cets_worker_deadletter_total{worker_kind}
  cets_metrics_scrape_errors_total{collector}
  probe_success{probe_scope="blackbox"}
  probe_duration_seconds{probe_scope="blackbox"}
  node_cpu_seconds_total{mode, cpu}
  node_load1
  node_memory_MemAvailable_bytes
  node_memory_MemTotal_bytes
  node_vmstat_pgpgin / node_vmstat_pgpgout
  node_filesystem_avail_bytes / node_filesystem_size_bytes
  node_network_receive_drop_total / node_network_transmit_drop_total
  container_cpu_usage_seconds_total{name}
  container_memory_working_set_bytes{name}

current shipped alert rules:
  CETSHighHTTPErrorRate          -> cets_http_requests_total 5xx ratio > 1%
  CETSHighP99Latency            -> cets_http_request_seconds_bucket P99 > 1s
  CETSDBPoolAcquireWaitHigh     -> pool acquire wait average > 50ms
  CETSDBLockWaitingSessions     -> cets_db_lock_waiting_sessions > 0
  CETSOutboxOldestLagHigh       -> cets_outbox_oldest_lag_seconds > 300s
  CETSMetricsScrapeErrors       -> cets_metrics_scrape_errors_total increasing

future WS2 target metrics:
  cets_db_lock_wait_seconds_bucket{le=...}
  cets_booking_tx_duration_seconds_bucket{outcome="confirmed|rejected|conflict"}
  cets_worker_process_seconds_bucket{worker_kind, outcome}

structured log fields (stdout JSON):
  ts, level, msg, trace_id, span_id, route, employee_id_hash,
  event_id, registration_id, idempotency_key, worker_kind,
  outcome, latency_ms, retry_count
```

`cets_outbox_lag_seconds_*` is a histogram of rows that reached `publish_status='published'`, measuring `published_at - created_at`. It is not the live backlog gauge. Current queue pressure and dead-letter pressure remain exposed through `cets_outbox_pending_total`, `cets_outbox_oldest_lag_seconds`, and `cets_outbox_dead_letter_total`.

`employee_id_hash` and any PII-derived field is salted/truncated per existing Phase 1 redaction rules. No signed tokens, provider tokens, or full names in any field.

Seed command contract (admin process):

```text
cets seed --profile=phase2-50k [--events=N] [--eligibility-coverage=0.0..1.0]
  - idempotent; safe to re-run
  - exits 0 on success, non-zero on partial failure with structured error log
  - does not require app/worker to be running
```

## 7. 12-Factor Notes

- **Config**: The current metrics slice uses the existing app listener and adds only local observability profile knobs in compose (`PROMETHEUS_PORT`, `VICTORIA_METRICS_PORT`, `ALERTMANAGER_PORT`, `BLACKBOX_EXPORTER_PORT`, `REDIS_EXPORTER_PORT`, `NODE_EXPORTER_PORT`, `CADVISOR_PORT`, `LOKI_PORT`, `TEMPO_PORT`, `TEMPO_OTLP_GRPC_PORT`, `TEMPO_OTLP_HTTP_PORT`, `OTEL_TRACES_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_SERVICE_VERSION`, `PYROSCOPE_PORT`, `PYROSCOPE_ENABLED`, `PYROSCOPE_SERVER_ADDRESS`, `PYROSCOPE_APPLICATION_NAME`, `GRAFANA_PORT`, `GRAFANA_ADMIN_USER`, `GRAFANA_ADMIN_PASSWORD`). Future k6, seed, production Alertmanager receiver, or PagerDuty knobs must be added through typed config before their implementations land. No secrets.
- **Backing services**: No required production backing service is added. Metrics scrape is pull-based against the existing app/worker ports and optional exporters; local Prometheus, VictoriaMetrics, Alertmanager, blackbox exporter, Redis exporter, node exporter, cAdvisor, Loki, Tempo, Pyroscope, and Grafana are optional review/demo services.
- **Build / release / run**: Same binary; metrics wiring is in-process. k6 runs from `grafana/k6:1.7.1-with-browser` (already used by Phase 1 gate).
- **Processes**: No new application process types; seed runs as `cets seed` admin one-off. Prometheus, VictoriaMetrics, Promtail, Loki, Tempo, Pyroscope, Alertmanager, Redis exporter, node exporter, and cAdvisor are optional local observability profile services only.
- **Logs**: Structured JSON to stdout — additive fields only, never logging tokens or full PII. OTEL trace/span IDs may be added only when tracing is enabled so Loki can link logs to Tempo. CI log-scan from Phase 1 (`ci.yml` "Scan live gate logs") stays green.
- **Admin processes**: `cets seed --profile=phase2-50k` is a same-codebase admin command; baseline report run is a one-off `k6 run` + write to artifact path.
- **Disposability**: Metric exporters drain on SIGTERM with the rest of the app.

## 8. Tests / Verification

- `cd services/api && go test ./deploy -run 'Observability|Blackbox|Infra|Exporter|USE' -count=1` — Compose/profile contract plus black-box probe target boundaries, Redis exporter, and optional exporter contracts.
- `docker manifest inspect prom/alertmanager:v0.28.1` and `docker manifest inspect victoriametrics/victoria-metrics:v1.129.1` — optional Alertmanager and long-term metrics image tags resolve.
- `docker manifest inspect prom/node-exporter:v1.9.1` and `docker manifest inspect gcr.io/cadvisor/cadvisor:v0.52.1` — optional exporter image tags resolve.
- `docker run --rm --entrypoint promtool -v "$PWD/services/api/deploy/observability:/etc/prometheus:ro" prom/prometheus:v3.6.0 check config /etc/prometheus/prometheus.yml` — Prometheus scrape config parses, including black-box relabeling and infra exporter jobs.
- `docker run --rm --entrypoint amtool -v "$PWD/services/api/deploy/observability/alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro" prom/alertmanager:v0.28.1 check-config /etc/alertmanager/alertmanager.yml` — Alertmanager route and receivers parse.
- `docker run --rm -v "$PWD/services/api/deploy/observability/blackbox.yml:/etc/blackbox_exporter/config.yml:ro" prom/blackbox-exporter:v0.27.0 --config.file=/etc/blackbox_exporter/config.yml --config.check` — blackbox exporter config parses.
- `cd services/api && go test ./internal/observability/... -count=1` (new package) — metric registration, label cardinality bound, redaction.
- Seed command test: `go test ./services/api/cmd/cets -run TestSeedPhase2 -count=1` against a temp DB; asserts deterministic counts.
- k6 dry-run in CI: `k6 inspect k6/phase2-hot-event.js` + smoke run with reduced VUs.
- k6 threshold gate: `k6 run k6/phase2-hot-event.js` against live Compose in `live-gates` job; thresholds fail on regression.
- Trace assertion: integration test issues a booking, scrapes structured logs, asserts a single `trace_id` appears in HTTP, DB, outbox enqueue, worker pickup, and SMTP send entries.
- Baseline report (`PH2-16`): markdown artifact reviewed by Person A + at least one consumer WS owner.

## 9. Rollback / Disable

Runtime additions in WS2 are observability only:

- Metrics endpoint: `GET /metrics` is mounted on the existing app listener for this slice. It has no separate `METRICS_LISTEN_ADDR` disable knob; rollback is reverting the additive route/instrumentation or disabling the Prometheus scrape target.
- Log aggregation: Loki/Promtail are optional Compose services only. Rollback is removing them from the `observability` profile; app and worker keep logging to stdout/stderr.
- Trace backend: Tempo is optional Compose service only. App OTLP export and log-to-trace correlation are opt-in via typed env config and default off; rollback is setting `OTEL_TRACES_ENABLED=false` or removing the observability profile.
- Profile backend: Pyroscope is optional Compose service only. App continuous profiling is opt-in via typed env config and defaults off; rollback is setting `PYROSCOPE_ENABLED=false` or removing the observability profile.
- New scrape collectors: errors degrade into `cets_metrics_scrape_errors_total` samples rather than blocking booking, check-in, worker, or audit paths.
- Seed profile: idempotent and re-runnable; rollback = `DROP TABLE`/`TRUNCATE` via existing migrate tooling, not a new admin process.
- k6 CI gate: gated by workflow input / branch filter; disable by reverting the workflow change. No runtime impact.

## 10. Non-Goals

- Do not change booking, eligibility, check-in, worker, or notification behavior to "make the numbers move." Tuning lives in WS3/WS4/WS5.
- Do not require a separate metrics backend service as a Phase 2 runtime deliverable. The optional local Prometheus/Grafana profile is for review and baseline evidence only.
- Do not put metrics in the booking commit path (no synchronous metric writes that can block a tx).
- Do not use baseline report numbers to justify Kafka, Kubernetes, service mesh, microservice split, or cross-region HA. Those remain deferred decision-gate topics.

## 11. Cross-Stream Dependencies

| Consumer WS | Consumes from WS2 | Where it lands |
| --- | --- | --- |
| WS3 | `PH2-16` baseline report, pool/lock metrics | Decide whether to ship `PH2-22` (Redis Lua), `PH2-23` (TTL compensation), `PH2-25` (hot-row contention). |
| WS4 | Outbox lag + worker metrics, trace schema | Tune retry/backoff (`PH2-33`), confirm isolation (`PH2-32`) helped. |
| WS5 | Outbox lag + report freshness signals, ops UI feed shape | Reports freshness contract (`PH2-44`), ops UI (`PH2-46`). |
| WS1 | k6 thresholds + baseline report | Release checklist (`PH2-06`) cites these as acceptance evidence. |
