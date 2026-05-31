# Phase 3 Local HA Compose + LGTM Simulation

This spec defines a development-only Phase 3 simulation. It proves the platform shape on one
machine with Docker Compose and explicit replica services, but it does not claim production
multi-AZ availability, managed database failover, disaster recovery, or multi-region active-active
operation is complete.

## Goals

- Simulate the Phase 3 traffic path: edge load balancer -> 3 gateway replicas -> frontend load
  balancer -> 3 frontend replicas -> backend load balancer -> 3 backend replicas -> PostgreSQL /
  Redis / outbox workers.
- Deploy a lightweight LGTM stack that correlates logs, metrics, traces, profiles, and a service
  graph for incident debugging.
- Generate k6 load through the external edge entrypoint so reviewers can prove replica distribution
  and practice a RED -> trace -> logs -> profile investigation loop.
- Preserve ticketing correctness: PostgreSQL remains the final truth for booking, tickets,
  check-in, and audit.
- Keep the architecture modular-monolith-first. Microservices remain a deferred decision gate until
  process-first scaling has measured evidence that it is insufficient.
- Keep implementation SOLID: platform concerns stay in deployment assets and adapters, not in
  ticketing domain logic.

## Architecture

| Component | Local simulation behavior | Production interpretation |
| --- | --- | --- |
| Docker Compose | Single-machine orchestration with explicit `gateway-1..3`, `frontend-1..3`, and `backend-1..3` services. | A future production plan still needs multi-node and multi-AZ evidence. |
| Edge load balancer | Nginx routes traffic to three gateway replicas and propagates request/trace headers. | Equivalent to an edge gateway or ingress tier. |
| Gateway | Three Nginx replicas route to the frontend load balancer. | Equivalent to a stateless gateway tier. |
| Frontend | Three static React/Nginx replicas serve assets and proxy API/readiness calls to backend load balancer. | Can be moved to CDN or a dedicated web tier later. |
| Backend | Three Go API replicas serve ticketing APIs and health endpoints. | Stateless app tier; data truth remains in PostgreSQL. |
| Workers | Same-binary worker processes split by kind for notification, projection, compensation, and export. | Scale independently by worker kind and queue lag. |
| Backing services | Local PostgreSQL, Redis, MinIO, and Mailhog resources. | Not a production HA data layer. |
| LGTM | Grafana, Loki, Tempo, Prometheus, Pyroscope, Alloy-compatible Docker log/trace collection, and direct Go profiling. | Production can swap storage and retention without changing domain code. |

## k6 Load And Investigation Plan

Phase 3 k6 runs must exercise the same external path a browser uses:

```text
k6 -> edge-lb -> gateway-1..3 -> frontend-lb -> frontend-1..3 -> backend-lb -> backend-1..3
```

The load profile has three modes:

- `smoke`: short validation that the edge path, auth, event browsing, booking, ticket listing,
  check-in-adjacent APIs, reports/audit, and controlled error generation work.
- `stress`: longer sustained traffic through the edge path, used to populate RED metrics, Tempo
  traces, Loki logs, Pyroscope profiles, and service graph metrics.
- `investigate`: targeted pressure that may additionally call a single backend replica from inside
  the Compose network to create an obvious backend CPU/profile hotspot for incident walkthroughs.

k6 must treat expected validation/business errors separately from unexpected failures. Controlled
errors should use existing application paths such as unknown mock profile, invalid booking payload,
or duplicate/conflicting business actions; do not add production-only fault endpoints.

Replica distribution is evidence-based. The Phase 3 route must expose non-sensitive replica identity
headers for gateway, frontend, and backend responses, and the k6 summary must prove at least three
unique frontend replicas and three unique backend replicas handled requests during the run. Header
values may contain container hostnames or upstream instance names only; they must not contain PII,
tokens, provider claims, idempotency keys, or request bodies.

The LGTM investigation flow is:

1. Use Grafana RED panels to identify a route/status/replica with rising latency or errors.
2. Drill into Tempo traces for the affected backend route and verify span names use route patterns,
   not raw IDs.
3. Use the trace ID to query Loki backend logs through the canonical `otel_trace_id` field.
4. Use Tempo/Grafana profile links or Pyroscope queries to find the backend CPU hot function.
5. Use service graph metrics to identify whether the bottleneck is edge/gateway/frontend/backend or
   worker-adjacent.

## Microservices Decision Gate

Phase 3 local HA does not introduce full microservices. The current architecture stays as a modular
monolith with same-binary process isolation because booking, ticket, check-in, and audit correctness
depend on PostgreSQL transactions and unique constraints. Extract a module into a separately
deployed service only after evidence shows process-first scaling cannot satisfy an independently
measured bottleneck, failure-isolation, ownership, or release-cadence need.

## SOLID Requirements

- **SRP**: edge routing, gateway routing, frontend serving, backend business behavior, worker
  processing, and observability collection are separate Compose services/configs.
- **OCP**: Phase 3 adds deployment and observability profiles without rewriting ticketing domain
  rules.
- **LSP**: local Compose adapters must preserve the behavior of existing PostgreSQL, Redis, object
  storage, mail, and worker boundaries.
- **ISP**: deployment verification scripts expose narrow checks: replica readiness, endpoint smoke,
  failure drill, and observability health.
- **DIP**: domain and application packages must not import or depend on Docker, Nginx, Grafana,
  Loki, Tempo, Prometheus, Pyroscope, or Alloy.

## Acceptance Criteria

| AC | Requirement |
| --- | --- |
| PH3-AC-1 | Docker Compose config renders with the base compose file, worker-isolation overlay, and Phase 3 HA overlay. |
| PH3-AC-2 | Gateway, frontend, and backend each run exactly 3 explicit Compose replicas. |
| PH3-AC-3 | The external edge entrypoint serves the React app and API `/healthz` and `/readyz` endpoints. |
| PH3-AC-4 | Stopping one gateway, frontend, or backend replica does not cause sustained smoke-test failure; the stopped replica can be restored. |
| PH3-AC-5 | Grafana datasources for logs, metrics, traces, and profiles are provisioned and healthy. |
| PH3-AC-6 | Backend traffic is visible through dashboard metrics, Docker logs, trace drilldown, profile data, and service graph; verification asserts queryable service graph and profile data. |
| PH3-AC-7 | Booking, ticket, check-in, worker retry, and audit correctness tests still pass. |
| PH3-AC-8 | Telemetry does not include PII, signed QR tokens, provider secrets, raw idempotency keys, email bodies, or raw recipient email; redaction canaries cover each sensitive class. |
| PH3-AC-9 | k6 smoke and stress profiles prove external edge traffic reaches at least three frontend replicas and three backend replicas. |
| PH3-AC-10 | Controlled error traffic is visible in RED metrics and Loki/Tempo evidence without being counted as unexpected k6 failure. |
| PH3-AC-11 | Grafana/LGTM evidence is end-to-end queryable for the k6 path: Prometheus RED data must include route, method, status class, latency histogram, and all three backend instances; Tempo must return a backend trace with route evidence; Loki must return backend logs for that exact Tempo trace ID; Pyroscope must return backend CPU samples; service graph metrics must include an edge whose server is `cets-backend`. |

## Stop Conditions

- Stop if any document describes this single-machine simulation as production HA, production
  multi-AZ, complete disaster recovery, or multi-region active-active.
- Stop if Redis, a read model, local files, queues, worker memory, or observability data becomes
  the source of truth for booking, ticket, check-in, or audit decisions.
- Stop if gateway/frontend/backend splitting changes existing ticketing API contracts without a
  compatibility plan.
- Stop if telemetry exposes PII, signed tokens, provider secrets, raw idempotency keys, email
  bodies, or raw recipient email.
- Stop if scripts require legacy cluster tooling, Phase 3 cluster manifests, or destructive
  host-cluster changes.
- Stop if verification can pass without `/readyz`, k6 replica-distribution evidence, route/status/instance
  RED evidence, service graph validation involving `cets-backend`, exact Tempo trace ID to Loki log
  correlation, or profile validation.

## Verification

- `scripts/compose/phase3-deploy.sh` builds images and starts the Phase 3 Compose HA topology.
- `scripts/compose/phase3-k6.sh` runs `k6/phase3-ha-lgtm.js` in `smoke`, `stress`, or
  `investigate` mode and fails when replica distribution evidence is incomplete.
- `scripts/compose/phase3-verify.sh` checks replica state, endpoint smoke, Grafana datasource
  provisioning, k6 stress evidence, Prometheus RED targets, Tempo trace ingest, Tempo service graph
  metrics, Pyroscope profile data, Loki trace-correlated logs, and telemetry redaction canaries.
  Its LGTM assertions are intentionally data-level checks, not just health checks: RED must prove
  request/error/duration evidence by route/status/backend instance, Tempo must produce a trace ID,
  Loki must return backend logs for that same trace ID, Pyroscope must return non-zero backend CPU
  samples, and service graph metrics must include `cets-backend` as a server node.
- `scripts/compose/phase3-drill.sh` stops one stateless replica at a time and verifies recovery.
- `cd services/api && go test ./... -count=1`.
- `pnpm --filter cets-web lint`, `pnpm --filter cets-web test`, and `pnpm --filter cets-web build`.

## Iteration Rule

Before closing each Phase 3 local HA iteration, treat missing datasource health, trace ingest,
service graph data, profile data, or redaction evidence as a blocker and either fix it or record
the remaining gap in `goal.md`.
