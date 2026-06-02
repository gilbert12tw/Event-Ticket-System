# Feature: Phase 1 Ticketing MVP

## Summary

Build a Go modular monolith that can run locally against PostgreSQL and demonstrate the Phase 1 corporate event ticketing flow: admins create limited or unlimited events and eligibility rules, employees authenticated by an external provider browse and book eligible events, the system records family counts only for unlimited events, issues non-transferable signed electronic tickets, lets check-in staff redeem tickets once with holder verification, and lets HR admins inspect participation reports and immutable audit logs.

## Small Task Breakdown

1. MVP spec and project scaffold.
2. Database-backed Go API with migrations, health checks, structured logs, and graceful shutdown.
3. Event, eligibility, registration, ticket, check-in, reporting, and audit use cases.
4. React browser UI served by the Go app.
5. Docker Compose app integration, verification, code review, and local commits.

## Acceptance Criteria

- [ ] AC-1: Given an activity admin, When they create a published event with `capacity_type`, city/site, capacity, family rule, and eligibility rules, Then the event is persisted in PostgreSQL and an audit log entry is recorded.
- [ ] AC-2: Given an employee whose provider/HR claims match the event rules, When they browse events, Then they see eligibility, limited/unlimited status, family availability, remaining capacity when applicable, and any cross-city warning.
- [ ] AC-3: Given an eligible employee and available limited capacity, When they book with an idempotency key, Then a confirmed registration and signed non-transferable ticket are created in PostgreSQL, the response is retry-safe, the employee receives at most one ticket for the event, and the event is not oversold.
- [ ] AC-4: Given an eligible employee and no remaining limited capacity, When they book, Then a waitlist registration is created without issuing a ticket.
- [ ] AC-4a: Given an eligible employee booking an unlimited event, When they submit `family_count`, Then a confirmed registration is created without inventory decrement or waitlist, and no transferable companion ticket is created.
- [ ] AC-5: Given an ineligible employee, When they book, Then the request is rejected with a reason and no registration or ticket is created.
- [ ] AC-6: Given a confirmed ticket, When check-in staff redeem the signed token online, Then exactly one successful check-in record is created, duplicate scans return the first redemption details, and the response shows holder name/department/city and family count for on-site verification.
- [ ] AC-7: Given an HR/system admin, When they inspect reports and audit logs, Then participation counts, family totals, city distribution, ticket counts, check-in counts, no-show/cancellation events, and sensitive actions are visible without exposing unnecessary PII.
- [ ] AC-8: Given the app starts locally, When `/healthz` and `/readyz` are called, Then health returns app status and readiness verifies PostgreSQL connectivity.
- [ ] AC-9: Given the React browser UI is opened, When the demo control panel is used, Then an operator can step through event creation, eligibility display, booking, ticket display, check-in, reporting, and audit inspection without manual database edits.

## Edge Cases

| #    | Scenario                                                           | Expected Behavior                                                                                                   |
| ---- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| E-1  | Empty or malformed JSON request                                    | Return `400` with a JSON error response and do not mutate state.                                                    |
| E-2  | Missing, expired, unmapped, or incomplete external provider claims | Return `401` or `403`, degrade only where explicitly allowed, and record no sensitive data.                         |
| E-3  | Duplicate booking request with same idempotency key                | Return the original registration/ticket result without duplicate records.                                           |
| E-4  | Same employee retries with a different idempotency key             | Return the existing registration result without duplicate confirmed bookings.                                       |
| E-5  | Limited capacity is exhausted                                      | Create a waitlist registration and no ticket.                                                                       |
| E-6  | Concurrent bookings compete for the last seat                      | PostgreSQL transaction/locking and constraints prevent oversell.                                                    |
| E-7  | Tampered ticket token                                              | Reject check-in with `400` and record no successful check-in.                                                       |
| E-8  | Duplicate ticket scan                                              | Return `409` with first redemption details.                                                                         |
| E-9  | PostgreSQL unavailable                                             | `/readyz` fails and mutating endpoints return `503` or `500` without local fallback state.                          |
| E-10 | Unlimited event booking includes family members                    | Persist `family_count`, do not decrement inventory, do not create waitlist, and do not issue companion tickets.     |
| E-11 | Employee city differs from event city                              | Return a non-blocking warning on detail/eligibility/booking confirmation while allowing eligible booking.           |
| E-12 | Employee cancels after the registration window                     | Reject employee self-cancellation with `409`; admin exception requires reason and audit log.                        |
| E-13 | Employee reaches no-show cooldown threshold                        | Block limited-event booking during cooldown and allow unlimited-event booking unless a later policy says otherwise. |
| E-14 | Ticket holder mismatch or transfer attempt                         | Reject or flag check-in, show holder information, and record staff/device/reason audit data.                        |

## Non-Functional Requirements

| Category         | Requirement                                                                                                               | Metric                                                                          |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| Timeout          | HTTP server and DB operations use context deadlines.                                                                      | Default request timeout <= 5s.                                                  |
| Observability    | State-changing operations emit structured logs to stdout.                                                                 | JSON log includes action, status, and trace id.                                 |
| Failure Handling | Database failure does not fall back to in-memory state.                                                                   | Readiness and API errors expose failure safely.                                 |
| Idempotency      | Booking is retry-safe.                                                                                                    | Unique keys for booking idempotency and event/employee booking.                 |
| Consistency      | PostgreSQL is the source of truth.                                                                                        | Capacity checked in a transaction with row locks/constraints.                   |
| Security         | Product APIs validate external provider tokens and required claims; local/demo auth is non-production compatibility only. | Unauthorized sensitive actions rejected; product OpenAPI excludes login/logout. |
| Disposability    | App starts quickly and handles SIGTERM.                                                                                   | Graceful shutdown path implemented.                                             |

## Minimal API Contract

All responses use:

```json
{ "success": true, "data": {}, "error": null }
```

Error responses use:

```json
{ "success": false, "data": null, "error": "message" }
```

Provider-claims auth boundary:

```text
GET /api/v1/auth/me
```

Product OpenAPI does not expose `/api/v1/auth/login` or `/api/v1/auth/logout`. Local/demo compatibility uses mock provider metadata profiles that issue provider-format bearer tokens and must not be treated as product scope.

Endpoints:

```text
GET /healthz
GET /readyz
POST /api/v1/admin/events
GET /api/v1/events
GET /api/v1/events/{event_id}
GET /api/v1/events/{event_id}/eligibility
POST /api/v1/events/{event_id}/bookings
GET /api/v1/me/tickets
POST /api/v1/me/registrations/{registration_id}/cancel
POST /api/v1/checkins
GET /api/v1/admin/reports
GET /api/v1/admin/audit-logs
GET /
GET /user/events
GET /user/tickets
GET /employee/events
GET /employee/tickets
GET /admin/events
GET /admin/checkin
GET /admin/reports
GET /admin/audit
GET /admin/demo
GET /checkin
GET /hr/reports
GET /demo
```

The `/user/*` and `/admin/*` routes are the primary React SPA information architecture. `/employee/*`, `/checkin`, `/hr/reports`, and `/demo` remain legacy aliases for refresh and demo compatibility.

## 12-Factor Compliance Notes

- Config: `APP_ADDR`, `DATABASE_URL`, `APP_ENV`, and token signing secret are environment variables.
- Dependencies: Go dependencies are declared in `services/api/go.mod` and locked in `services/api/go.sum`.
- Backing Services: PostgreSQL is injected through `DATABASE_URL`; Redis, MinIO, and Mailhog remain attached in Compose for Phase 1 parity.
- Build / Release / Run: `services/api/Dockerfile` builds the Go binary; runtime config comes only from env.
- Processes: App is stateless; registrations, tickets, check-ins, reports, and audit logs are in PostgreSQL.
- Port Binding: HTTP binds to `APP_ADDR`.
- Disposability: Server handles SIGTERM/SIGINT and shuts down gracefully.
- Logs: Structured logs go to stdout/stderr without full PII or secrets.
- Admin Processes: Migrations and seed data run through the same binary with `migrate` and `seed` commands.

## Test Mapping

- AC-1: API handler/service tests for event creation and audit logging.
- AC-2 and AC-5: Eligibility unit tests and event listing tests.
- AC-3, AC-4, E-3, E-4, E-6: Registration service tests with database transactions.
- AC-6, E-7, E-8: Ticket signing and check-in service tests.
- AC-7: Reporting and audit query tests.
- AC-8: Health/readiness handler tests.
- AC-9: Browser MCP verification against the running local app.

## Verification Traceability

This table is the Phase 1 MVP verification contract. Unit and handler tests run
with `go test ./services/api/...`; PostgreSQL-backed correctness tests require a real
database and `TEST_DATABASE_URL`; browser demo verification requires the local
Compose app to be running.

| Requirement      | Verification                                                                                                                                     | Command / Evidence                                                                                                              |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| AC-1             | Event creation persists the event, eligibility rule, and audit log.                                                                              | `TEST_DATABASE_URL=postgresql://cets:cets_dev_password@localhost:15432/cets go test ./services/api/internal/ticketing -count=1` |
| AC-2             | Eligible employees see published events with eligibility and remaining capacity.                                                                 | `go test ./services/api/internal/ticketing`; PostgreSQL integration test above                                                  |
| AC-3             | Confirmed booking creates one registration, one signed ticket response, outbox event, and no oversell.                                           | PostgreSQL integration test above                                                                                               |
| AC-4 / E-5       | Full capacity creates a waitlist registration and does not issue a ticket.                                                                       | PostgreSQL integration test above                                                                                               |
| AC-5             | Ineligible booking is rejected and does not create registration or ticket rows.                                                                  | PostgreSQL integration test above                                                                                               |
| AC-6 / E-7 / E-8 | Signed ticket check-in succeeds once; tampered and duplicate tokens fail safely.                                                                 | `go test ./services/api/internal/ticketing`; PostgreSQL integration test above                                                  |
| AC-7             | Reports and audit logs expose participation and sensitive actions to HR admins.                                                                  | PostgreSQL integration test above                                                                                               |
| AC-8 / E-9       | Health returns app status; readiness verifies PostgreSQL connectivity and fails closed.                                                          | `go test ./services/api/internal/httpapi ./services/api/cmd/cets`; Compose readiness check                                      |
| AC-9             | React browser UI completes seed, event creation, eligibility, booking, ticket display, check-in, reports, and audit inspection without DB edits. | Run the Compose app and verify `http://localhost:8080/admin/demo` with Browser Use                                              |

## Phase 1 Non-Goals

This MVP spec is a demo baseline, not the Phase 1 production completion contract. `docs/specs/phase1-production-upper-bound.md` is the source of truth for production readiness and may require bounded offline check-in sync, Mailhog notification delivery, MinIO report exports, Playwright, and k6 gates that were intentionally outside the original MVP hardening pass.

- Do not split the Phase 1 app into microservices.
- Do not require Kafka, Kubernetes, cross-region HA, service mesh, or managed cloud services.
- Do not implement a product login/logout flow, password lifecycle, refresh token rotation, offline check-in sync, Redis reservation, object storage adapters, or mail delivery adapters in this MVP hardening pass.
- Do not deliver activity asset upload/serving, report Excel/PDF export, ticket PDF generation, cross-AZ HA, RTO/RPO, or managed-cloud failover in Phase 1 unless a new spec explicitly adds them.
- Redis, MinIO, and Mailhog remain Compose-attached backing services until their adapters are implemented in a later small task.

## Browser Demo Verification Record

Verified on 2026-05-05 with the Compose app running at `http://localhost:8080`.
The browser completed the `/admin/demo` flow without manual database edits:
seed employees, create event, browse eligibility, confirmed booking, waitlist
booking, ineligible rejection, ticket display, first check-in, duplicate scan
rejection, reporting, and audit inspection. The same pass checked
`/user/events`, `/admin/audit`, legacy aliases, and 375px / 768px / 1024px /
1440px layouts with no horizontal page overflow. Only expected negative-flow
403 / 409 browser records were observed.
