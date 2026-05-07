# Backend Directory Architecture

## Summary

Refactor `services/api` into smaller, module-oriented files while preserving the current Go package boundaries and Phase 1 modular monolith behavior. This is a directory and cohesion cleanup only: no HTTP contract, database schema, environment variable, Docker, worker, or business-rule changes are in scope.

## Current State

- `services/api/internal/ticketing/service.go`, `production.go`, `boundaries.go`, and `models.go` mix Event, Eligibility, Registration, Ticket, Check-in, Notification, Reporting, Audit, demo, and helper code in large files.
- `services/api/internal/httpapi/api.go` registers and implements handlers for every backend module in one file, while `router.go` exposes one broad service interface.
- `services/api/cmd/cets/main.go` contains serve, migrate, ready, seed, and worker command flow in one file.
- Several hand-written Go files exceed the repository's 500-line hard limit.

## Target Shape

Keep the first pass conservative by retaining package names and import paths:

```text
services/api/
  cmd/cets/
    main.go
    serve.go
    worker.go
    admin_commands.go
  internal/
    httpapi/
      router.go
      routes.go
      contracts.go
      responses.go
      auth.go
      handlers_events.go
      handlers_eligibility.go
      handlers_registrations.go
      handlers_tickets.go
      handlers_checkin.go
      handlers_notifications.go
      handlers_reporting.go
      handlers_audit.go
      handlers_demo.go
    ticketing/
      service.go
      events_service.go
      eligibility_service.go
      registrations_service.go
      tickets_service.go
      checkin_service.go
      notifications_service.go
      reporting_service.go
      audit_service.go
      demo_service.go
      *_models.go
```

## Acceptance Criteria

- [ ] AC-1: Given the refactor is complete, when `go test ./services/api/...` runs, then all existing backend tests pass without requiring API or schema changes.
- [ ] AC-2: Given any existing frontend or API client, when it calls current Phase 1 routes, then paths, methods, request JSON, response JSON, and status codes are unchanged.
- [ ] AC-3: Given any hand-written Go source file under `services/api`, when the file-size guard runs, then no file exceeds 500 lines.
- [ ] AC-4: Given package imports are inspected, when the architecture guard runs, then `ticketing` does not import `httpapi`.
- [ ] AC-5: Given docs are inspected, when Phase 1 architecture is described, then it remains a Docker Compose modular monolith and does not claim microservices, Kafka, Kubernetes, or cross-region HA are complete.

## Edge Cases

| # | Scenario | Expected Behavior |
|---|----------|-------------------|
| E-1 | A method is moved between files | Keep the receiver, exported name, input types, return types, and side effects unchanged. |
| E-2 | Handler helpers are split | Keep error envelopes, auth behavior, logging, trace IDs, and status code mapping unchanged. |
| E-3 | Models are split by module | Preserve JSON tags, enum values, zero-value behavior, and exported type names. |
| E-4 | Tests use package-local helpers | Move helpers only when needed and avoid changing test intent. |
| E-5 | A deeper package split looks attractive | Defer it unless this spec is updated with import-cycle and migration details. |

## Non-Functional Requirements

| Category | Requirement | Metric |
|----------|-------------|--------|
| Maintainability | Keep files cohesive and reviewable. | Hand-written Go files stay under 500 lines; target 200-300 lines. |
| Compatibility | Preserve current runtime behavior. | No API, schema, env, Docker, or frontend contract changes. |
| Testability | Keep module boundaries visible. | `httpapi` exposes module-sized interfaces embedded into the aggregate service contract. |
| Reliability | Preserve ticketing correctness guarantees. | No change to oversell prevention, idempotency, eligibility recheck, check-in uniqueness, outbox, or audit behavior. |
| Operability | Preserve 12-Factor process behavior. | Config remains env-based; logs stay on stdout/stderr; app and worker stay stateless. |

## Minimal Contract

No public contract changes are introduced. Internal compatibility requirements:

- `services/api/internal/ticketing.NewService` remains the construction entry point.
- Existing exported `ticketing` types and service methods remain available with the same names and signatures.
- `httpapi.NewRouter` keeps accepting an aggregate ticketing service plus router options.
- `cmd/cets` commands keep the same CLI names and environment variable behavior.

## Test Mapping

| Acceptance | Verification |
|------------|--------------|
| AC-1 | `go test ./services/api/...` |
| AC-2 | Existing `httpapi` tests plus unchanged route registration. |
| AC-3 | Add a backend architecture test that checks hand-written Go file line counts. |
| AC-4 | Add a backend architecture test that scans imports for forbidden `ticketing -> httpapi` dependency. |
| AC-5 | Add a docs architecture test that rejects Phase 1 completion claims for microservices, Kafka, Kubernetes, and cross-region HA. |

## 12-Factor Notes

- Codebase: this remains one repository and one Phase 1 application codebase.
- Dependencies: no new runtime dependencies are required for this refactor.
- Config: no new deploy-specific values are introduced.
- Backing services: PostgreSQL, Redis, MinIO, and Mailhog remain attached resources configured outside code.
- Build / release / run: build and run commands remain unchanged.
- Processes: serve and worker processes remain stateless.
- Port binding: HTTP binding stays in the server command and remains env-configured.
- Concurrency: no scaling or worker concurrency behavior changes.
- Disposability: graceful shutdown behavior must remain unchanged.
- Dev / prod parity: Docker Compose remains the Phase 1 local development entry point.
- Logs: no local log files or full PII logs are introduced.
- Admin processes: migrate, ready, and seed remain one-off commands in the same codebase.
