# 12-Factor and SOLID Compliance Matrix

This document records how the current codebase and deployment assets implement the
[Twelve-Factor App](https://12factor.net/) methodology and SOLID design principles.
It is the descriptive companion to the prescriptive rules in
[`twelve-factor-rule.md`](./twelve-factor-rule.md) and [`solid.md`](./solid.md):
those documents say what new code must do; this one maps each principle to where the
repository already does it, so reviewers can verify claims against concrete files.

Scope honesty: this matrix describes the Go modular monolith with Docker Compose and the
bare-metal Kubernetes/ArgoCD deployment. It does not claim microservices, Kafka, or
cross-region HA.

## Twelve-Factor Compliance

| #    | Factor              | Project practice                                                                                                                                                                                                                                                                       |
| ---- | ------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| I    | Codebase            | One Git monorepo holds the Go API (`services/api`), React SPA (`apps/web`), and deployment declarations; the GitOps state ArgoCD watches lives in `infra/k8s/baremetal/gitops`. The same codebase deploys to local Compose and bare-metal Kubernetes with config-only differences.       |
| II   | Dependencies        | Go modules (`go.work`, `services/api/go.mod`) and pnpm (`pnpm-lock.yaml`) pin every dependency; multi-stage Dockerfiles (`services/api/Dockerfile`, `apps/web/Dockerfile`) build from those manifests so no host-installed tool leaks into images.                                       |
| III  | Config              | All deploy-specific values are injected as environment variables parsed by `services/api/internal/config` (`config.Load()` collects load errors and fails loudly); `services/api/deploy/.env.example` documents the contract. No database URL, secret, or threshold is hard-coded.      |
| IV   | Backing services    | PostgreSQL, Redis, MinIO, mail (Mailhog/SMTP), HR, and SSO are attached resources reached via URLs (`DATABASE_URL`, `REDIS_URL`, ...) behind narrow adapters (`internal/postgres`, `internal/notification`, `internal/objectstore`); domain code accepts ports, not concrete clients.    |
| V    | Build, release, run | CI builds immutable images tagged with the commit SHA (`.github/workflows/baremetal-cd.yml`), promotion is a GitOps commit bumping `kustomization.yaml` `newTag`, and ArgoCD applies it — build, release, and run are three separate, auditable steps. Runtime never rebuilds.           |
| VI   | Processes           | API and worker pods are stateless; bookings, tickets, check-ins, idempotency keys, and audit records live in PostgreSQL as the single source of truth. Redis is a reservation gate and cache, never the final transaction truth.                                                         |
| VII  | Port binding        | The API binds its own HTTP port from env (`cmd/cets/serve.go`); workers expose a metrics port via `WORKER_METRICS_PORT` (default 9090). Traffic enters through ingress-nginx (`gitops/app/ingress.yaml`); nothing relies on runtime injection of a server.                                |
| VIII | Concurrency         | Scaling is by process type: a 6-replica backend Deployment plus four independent worker Deployments (`worker-notification`, `worker-projection`, `worker-compensation`, `worker-export` in `gitops/app/workers.yaml`), each horizontally replaceable.                                    |
| IX   | Disposability       | `cmd/cets/serve.go` traps SIGINT/SIGTERM and drains within `ShutdownTimeout`; workers take outbox leases and process idempotently, so duplicate or interrupted deliveries are safe to retry. Readiness/liveness probes gate traffic during restarts.                                     |
| X    | Dev/prod parity     | Local Compose (`services/api/deploy/compose.yaml`) and Kubernetes run the same images against the same backing-service types (PostgreSQL, Redis, MinIO, mail mock); tests use deterministic fakes rather than weakened production code paths.                                            |
| XI   | Logs                | Structured `slog` output goes to stdout/stderr only — no log files, no full PII, no secrets. The LGTM stack (Loki/Grafana/Tempo/Pyroscope, dashboards under `infra/k8s/baremetal/dashboards`) collects and correlates streams outside the app.                                            |
| XII  | Admin processes     | Migrations run as an ArgoCD `PreSync` Job (`gitops/app/migrate-job.yaml`) using the same `cets-api` image; seeding and demo resets are explicit one-off commands (`cets migrate`, `cets seed`, `cets reset-demo-db` in `cmd/cets/admin_commands.go`), never package-init side effects.    |

## SOLID Compliance

| Principle | How the codebase applies it                                                                                                                                                                                                                                              |
| --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SRP       | Modules are cohesive around one actor or domain: `internal/ticketing` splits per use case (`checkin_service.go`, `eligibility_service.go`, `report_export_worker.go`, ...), with hand-written files held to the 200–500 line limit so unrelated reasons to change stay apart. |
| OCP       | Extension points exist only at proven variation points — notification delivery (`NotificationSender`), report export storage, outbox worker kinds, reporting projections — so new variants plug in without editing stable booking/ticketing policy.                          |
| LSP       | Adapters are substitutable behind behavioral contracts: test fakes (e.g. `fakeTicketingService`, deterministic mail/object-store fakes) honor the same semantics as production adapters, which is what lets the same handler and worker tests drive both.                    |
| ISP       | Interfaces are small and client-specific: `httpapi/contracts.go` splits `TicketingService`, `EventService`, and `EventAssetService` per handler group, and the export worker depends on `ReportObjectStore` / `ReportObjectReader` / `ReportObjectExistenceChecker` slices.  |
| DIP       | Domain services receive capabilities through constructors (`NewService(pool, signer, logger)`); PostgreSQL, Redis, MinIO, SMTP, HR, SSO, and time are passed through narrow boundaries, so stable policy never imports volatile adapter detail.                              |

## Verification pointers

- Factor III/IV: `services/api/internal/config/config.go`, `services/api/deploy/.env.example`
- Factor V: `.github/workflows/baremetal-cd.yml`, `infra/k8s/baremetal/gitops/app/kustomization.yaml`
- Factor VIII/XII: `infra/k8s/baremetal/gitops/app/workers.yaml`, `migrate-job.yaml`
- Correctness guarantees backing Factor VI/IX (oversell prevention, one-time redemption,
  idempotent retries) are specified in `docs/agent-rules/correctness.md`.
