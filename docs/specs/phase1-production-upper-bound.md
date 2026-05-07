# Feature: Phase 1 Production Upper Bound

## Summary

Extend the current Phase 1 modular monolith into a production-shaped internal ticketing system while keeping the Phase 1 deployment boundary: one Go application, one optional worker process from the same binary, PostgreSQL as the source of truth, and Docker Compose backing services for PostgreSQL, Redis, MinIO, and Mailhog. This feature adds production pages, database-backed event governance, eligibility versioning, registration management, ticket hardening, online/offline check-in boundaries, notifications, reporting, and audit filtering without introducing microservices, Kafka, Kubernetes, or cross-region high availability.

## Acceptance Criteria Completion Matrix

Phase 1 production is complete only when every row below has implementation evidence, a service or integration test, both Playwright e2e and k6 coverage where user-facing, 12-Factor/release evidence, and reviewer approval. Fake-service route exposure, route-only tests, and documentation-only coverage do not satisfy an AC.

| AC | Production behavior | Implementation evidence | Required tests and gates |
|---|---|---|---|
| AC-1 | Activity admins create, update, duplicate, publish, close, cancel, or archive events with current state, event versions, and audit logs in PostgreSQL. | Event admin service, HTTP admin event routes, event version inserts, audit inserts. | `event_admin_service` tests, `api_test` route/handler tests, DB integration for state/version/audit, CI Go gate. |
| AC-2 | Eligibility preview/save persists rule versions, returns match count, rejects zero-match save unless explicitly confirmed, and audits changes. | Eligibility service preview/save flow, `eligibility_rule_versions`, audit rows. | Eligibility service tests, API contract tests, frontend eligibility governance tests, Playwright admin event routes, CI web and Go gates. |
| AC-3 | HR sync batches create impact reviews for affected active registrations/tickets and notify admins through outbox. | Same-binary `hr-sync` command, HR sync service, `hr_sync_batches`, impact reviews, outbox/audit rows. | `hr_sync_service_test`, command tests, DB integration, worker/outbox tests, CI Go gate. |
| AC-4 | Employees browse/filter events and details with eligibility reason, remaining capacity, ticket rules, and current booking status. Event asset upload/serving is out of Phase 1 production unless a new asset spec is written. | Employee events pages, typed API client, event summary/detail services. | Frontend unit tests, Playwright live browse/detail flow, k6 browse/detail scenario, CI web/e2e/k6 gates. |
| AC-5 | Booking and cancellation are retry-safe and PostgreSQL constraints prevent duplicate booking and oversell. | Booking/cancel services, idempotency keys, DB constraints and transactions. | Booking, cancellation, oversell, idempotency DB tests; k6 book/cancel scenario; CI DB integration and k6 gates. |
| AC-6 | Cancellation-triggered or manual waitlist promotion promotes exactly one earliest eligible employee with ticket, audit, outbox, and no oversell; no-op or failed promotion attempts remain auditable. | Waitlist promotion service inside DB transaction, ticket issuance, audit/outbox rows. | Waitlist promotion integration tests, admin registration route tests, Playwright admin registration route, k6 waitlist scenario, CI Go/e2e/k6 gates. |
| AC-7 | Lottery allocation with `(event_id, seed)` is deterministic and replay-safe, with winners, waitlist assignments, tickets, audit, and outbox. | Lottery admin service, deterministic sort, idempotent lottery run lookup. | Lottery deterministic integration tests, API route tests, k6 admin flow coverage, CI Go/k6 gates. |
| AC-8 | Tickets have sequence number, signed-token hash, display payload, expiry, and audited issued/revoked/expired/cancelled-equivalent transitions. | Ticket services, signer/hash verification, revocation and expiry transitions. | Ticket ownership, issue audit, tamper, revoke, expiry, check-in tests; frontend QR tests; k6 ticket/check-in scenario. |
| AC-9 | Online and offline check-in redeem a ticket once, preserve duplicate/conflict details, and audit invalid or conflict outcomes. | Check-in service, offline package/sync service, first-commit-wins DB behavior. | Online check-in tests, offline package/sync conflict tests, Playwright check-in/offline routes, k6 check-in/offline sync gate. |
| AC-10 | Worker processes required notification outbox events with Mailhog email and in-app delivery records, retry, suppression, duplicate prevention, and dead-letter state. | Same-binary worker, PostgreSQL outbox leases, notification delivery rows, SMTP adapter. | Worker retry/suppression/dead-letter tests, SMTP context tests, Compose app/worker gate, k6 smoke/release gate. |
| AC-11 | HR/admin reports and exports expose aggregate participation only; exports are authorized, object-storage backed, audited, and limited to the Phase 1 field whitelist. | Reporting service, report export service, S3-compatible/MinIO adapter boundary, audit/outbox rows. | Report/export worker tests, object store adapter tests, Playwright reports route, k6 report/export scenario. |
| AC-12 | Audit filtering is server-side with stable cursor ordering and redacted metadata/log payloads. | Audit query helpers, cursor parser, HTTP audit filters, metadata redaction. | Audit filter/cursor/redaction tests, API handler tests, Playwright audit route, k6 audit scenario. |
| AC-13 | React SPA is accessible at 375px, 768px, 1024px, and 1440px without horizontal overflow for all Phase 1 roles. | Canonical routes, role guard, unauthorized state, responsive layouts. | `pnpm --filter cets-web test:e2e` with Chromium viewport projects, frontend unit tests, CI e2e gate. |
| AC-14 | Every new API/schema/worker/page behavior has matching tests before being considered complete. | Architecture test, CI production gate, reviewer checklist. | `go test ./services/api/...`, DB integration, web lint/test/build/e2e, Docker build, Compose config, k6, reviewer pass. |

## Edge Cases

| # | Scenario | Expected Behavior |
|---|----------|-------------------|
| E-1 | Event state transition skips an illegal state | Return `409`, do not mutate event, and do not create a new version. |
| E-2 | Event update changes capacity below confirmed registrations | Return `409` unless confirmed registrations are first cancelled or moved through an explicit admin action. |
| E-3 | Eligibility expression is malformed or matches zero employees | Return `400` for malformed expressions; allow zero-match save only with explicit admin confirmation and audit metadata. |
| E-4 | HR sync removes eligibility from existing ticket holders | Keep existing tickets usable until admin review resolves the impact row unless the event is configured for automatic revocation. |
| E-5 | Employee retries booking/cancellation with the same idempotency key | Return the original result without duplicate registration, ticket, or audit side effects. |
| E-6 | Cancellation creates capacity while waitlist exists | Promote exactly one earliest eligible waitlisted registration inside the same transaction or leave an auditable promotion failure. |
| E-7 | Lottery is run twice with the same seed and unchanged entries | Return the existing run/result instead of reallocating tickets. |
| E-8 | Ticket token is expired, revoked, tampered, or mismatched | Reject check-in and never create an accepted check-in record. |
| E-9 | Offline scan conflicts with an already redeemed ticket | Preserve both scan records, mark conflict, and keep the first committed redemption as final. |
| E-10 | Mailhog, Redis, MinIO, or worker queue is unavailable | Core booking and check-in do not fall back to local process state; side effects are retried or marked failed in PostgreSQL. |
| E-11 | Report export is requested by a non-HR/admin role | Return `403` and create no export file. |
| E-12 | Audit query requests a very large page | Enforce a configured maximum page size and return deterministic cursor ordering. |

## Non-Functional Requirements

| Category | Requirement | Metric |
|----------|-------------|--------|
| Phase Boundary | Keep Phase 1 as modular monolith plus optional same-binary worker. | No microservice, Kafka, Kubernetes, service mesh, or cross-region dependency. |
| Consistency | PostgreSQL is final truth for bookings, tickets, check-ins, reports, and audit logs. | Unique constraints and transactions cover oversell, duplicate booking, and duplicate check-in. |
| Hot Event Gate | Redis may reserve short-lived capacity, but DB commit confirms success. | Redis outage degrades safely to DB-only or returns a controlled service error. |
| Timeout | HTTP, DB, Redis, MinIO, and mail operations use context deadlines. | Default request deadline <= 5s; check-in target remains <= 200ms p99 in Phase 1 smoke tests. |
| Observability | State changes emit structured JSON logs to stdout with trace id and redacted identifiers. | No signed tokens, session secrets, or full PII in logs. |
| Worker Reliability | Outbox worker is idempotent and disposable. | Duplicate event consumption does not duplicate deliveries or read-model rows. |
| Accessibility | Forms use labels and controlled inputs; table layouts degrade for mobile. | Keyboard focus visible; no emoji icons; responsive at required breakpoints. |
| Testability | Tests map to acceptance criteria. | Every small task adds or updates matching unit/integration tests. |

## Minimal API Contract

All API responses continue to use the existing envelope:

```json
{ "success": true, "data": {}, "error": null }
```

Errors use:

```json
{ "success": false, "data": null, "error": "human-readable message" }
```

New or expanded endpoint groups:

```text
GET    /api/v1/admin/events
GET    /api/v1/events/{event_id}
PATCH  /api/v1/admin/events/{event_id}
POST   /api/v1/admin/events/{event_id}/state
POST   /api/v1/admin/events/{event_id}/duplicate
DELETE /api/v1/admin/events/{event_id}

POST   /api/v1/admin/events/{event_id}/eligibility/preview
PUT    /api/v1/admin/events/{event_id}/eligibility
GET    /api/v1/admin/eligibility-impact-reviews
POST   /api/v1/admin/eligibility-impact-reviews/{review_id}/resolve

GET    /api/v1/admin/events/{event_id}/registrations
POST   /api/v1/events/{event_id}/bookings/{registration_id}/cancel
POST   /api/v1/admin/events/{event_id}/registrations/{registration_id}/cancel
POST   /api/v1/admin/events/{event_id}/waitlist/promote
POST   /api/v1/admin/events/{event_id}/lottery-runs

GET    /api/v1/tickets/{ticket_id}
POST   /api/v1/admin/tickets/{ticket_id}/revoke
GET    /api/v1/checkins/events/{event_id}/offline-package
POST   /api/v1/checkins/offline-sync

GET    /api/v1/notifications/preferences
PUT    /api/v1/notifications/preferences
GET    /api/v1/admin/notifications/deliveries
POST   /api/v1/admin/notifications/deliveries/{delivery_id}/retry

GET    /api/v1/admin/reports
POST   /api/v1/admin/reports/exports
GET    /api/v1/admin/audit-logs?actor_id=&role=&action=&entity_type=&entity_id=&from=&to=&limit=&cursor=
```

## Production Pages

- Employee pages: `/user/events`, `/user/events/:event_id`, `/user/tickets`, `/user/notifications`.
- Activity admin pages: `/admin/events`, `/admin/events/new`, `/admin/events/:event_id/edit`, `/admin/events/:event_id/eligibility`, `/admin/events/:event_id/registrations`, `/admin/notifications`.
- Check-in pages: `/admin/checkin`, `/admin/checkin/offline`.
- HR/system admin pages: `/admin/reports`, `/admin/hr-sync`, `/admin/audit`, `/admin/settings`.

## RBAC Completion Matrix

`system_admin` is a Phase 1 alias for HR/admin audit capabilities, not a separate implementation role. A separate system-admin role requires a new spec.

| Surface | Employee | Activity admin | Check-in staff | HR/system admin | Unauthorized |
|---|---|---|---|---|---|
| Employee events, event detail, and own tickets | allow own data only | deny | deny | deny | login required |
| Event governance and eligibility management | deny | allow | deny | read-only event visibility where implemented | 401/403 |
| Registration governance, waitlist, lottery, ticket revoke | deny | allow | deny | read-only governance visibility where implemented | 401/403 |
| Online/offline check-in | deny | deny | allow | deny | 401/403 |
| Reports, exports, HR sync, audit logs | deny | deny unless explicitly listed | deny | allow | 401/403 |
| Notifications preferences | allow own preferences | deny | deny | deny | login required |
| Notification delivery admin | deny | allow | deny | allow | 401/403 |

Every protected API and page must have an allow/deny test for its owning role and at least one forbidden role. Sensitive denied actions create no business mutation.

## Phase 1 Product Boundaries

- Offline check-in packages are single-event, staff-bound, device-bound, signed, and valid for 4 hours. Offline scan results are provisional until sync; sync must return accepted, duplicate, or conflict per scan and preserve the first committed redemption.
- Required notification outbox events: booking confirmed, booking waitlisted, waitlist promoted, registration cancelled, lottery completed, ticket revoked, ticket expired, eligibility impact review created, report export ready, and report export failed.
- Report exports are HR-only aggregate CSVs. The allowed columns are `event_id`, `title`, `capacity`, `confirmed_count`, `waitlist_count`, `ticket_count`, `checkin_count`, `remaining_capacity`, and `starts_at`. Exports must not include employee rows, full names, signed tokens, QR payloads, session data, or raw audit metadata.
- Event asset upload, event image serving, ticket PDF generation, custom report builders, analytics warehouses, and read replicas are out of Phase 1 production unless a new spec adds them.

## 12-Factor Compliance Notes

- Config: New Redis, MinIO, mailer, worker, token TTL, export, and pagination settings must come from environment variables.
- Dependencies: Any new Go or web dependency must be declared in `go.mod` or `package.json` and locked.
- Backing Services: PostgreSQL, Redis, MinIO, and Mailhog are attached resources, injected by URLs/credentials.
- Build / Release / Run: App and worker use the same built artifact; runtime command selects web, worker, migrate, or seed.
- Processes: No durable state in memory or local files; uploads, exports, messages, and scans persist in backing services.
- Port Binding: The web process continues to bind from env-configured address/port.
- Concurrency: Multiple app or worker processes must be safe through DB constraints and idempotency.
- Disposability: Web and worker handle SIGTERM, drain current work, and exit without corrupting booking/check-in state.
- Logs: Structured logs go to stdout/stderr and redact signed tokens, sessions, and full employee PII.
- Admin Processes: Migrations, seeds, worker, lottery, HR sync, and data repair are same-codebase one-off commands.

## Required Production Gates

- Backend: `go test ./services/api/... -count=1` and `TEST_DATABASE_URL=... go test ./services/api/... -count=1`.
- Frontend: `pnpm --filter cets-web lint`, `pnpm --filter cets-web test`, `pnpm --filter cets-web build`, and `pnpm --filter cets-web test:e2e`.
- Playwright: Chromium projects for 375px, 768px, 1024px, and 1440px must cover employee, activity-admin, check-in, offline check-in, HR/reporting, audit, notifications, and unauthorized states with no horizontal overflow or severe console errors. Mocked route tests are useful only for viewport/layout coverage; at least one live app/Compose-backed Playwright workflow must run without API route mocking before frontend production completion.
- k6 smoke: `k6/phase1-production-gate.js` must run against local Compose and cover health/ready, login, browse/detail, book/cancel, ticket detail, online check-in, offline sync, reports/export, audit, and critical browser paths for employee tickets, admin check-in, and admin audit.
- k6 release: `k6/phase1-release-gate.js` must cover 36 app RPS, 5 booking TPS, check-in p99 `< 200ms`, idempotent retries, duplicate check-in, waitlist promotion, lottery, reports/export, and audit pagination. Release thresholds must fail with a non-zero exit code.
- Release: Docker build, Compose config, explicit migration, app/worker startup, production-shaped config validation, Playwright, k6 smoke, k6 release, and reviewer pass must all succeed before any AC is marked complete.


## Production Completion Gate

- Fresh PostgreSQL migration must be applied against an empty database in the integration gate; string-only schema tests are not sufficient.
- Route-exposure tests using fake services do not satisfy production behavior coverage. Each production endpoint must have a matching service or integration test for state, audit, idempotency, and error behavior.
- Worker, Mailhog, MinIO, and offline check-in behavior must be verified through adapter boundaries or DB-backed tests before the related acceptance criteria can be marked complete.
- Frontend pages must use typed API contracts for surfaced production behavior and include route/deep-link, live workflow, loading, error, empty, and role-denied tests.
- Reviewer approval is required for each completed workstream and must reject route-only, fake-service-only, docs-only, or untested changes.

## NestJS Decision

NestJS is not part of Phase 1 production. The Phase 1 backend remains a Go modular monolith with same-binary worker processes because the remaining risks are PostgreSQL correctness, worker reliability, 12-Factor configuration, and production gates. Adding NestJS would introduce a second server runtime, a second dependency-injection/module system, additional Docker and CI surface, and a new auth/session boundary without reducing the current Phase 1 risk.

NestJS may be reconsidered in Phase 2 or Phase 3 only if the team needs an independently deployed TypeScript BFF, GraphQL gateway, WebSocket-heavy admin surface, or a deliberate split of frontend platform responsibilities from the Go API. Any future NestJS adoption requires a new spec defining API ownership, auth/session handoff, deployment topology, observability, and test gates before code is added.

## Phase 1 Non-Goals

- Do not split modules into independently deployed services.
- Do not require Kafka, Kubernetes, service mesh, multi-region, or managed cloud services.
- Do not implement external payments.
- Do not implement full offline-first consensus; offline check-in uses bounded package download and first-commit-wins sync.
