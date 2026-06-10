# Evolution Boundaries

## Summary

This document keeps the minimum active contract for Phase 2/3 evolution after
historical workstream specs were removed. It is normative for worker kinds,
Redis pre-admission, event contracts, reporting projections, and deferred
infrastructure claims.

## Process-First Scaling

- Keep one Go codebase and one deployable `cets` command binary.
- Scale with more `app` processes and same-binary workers selected by
  `WORKER_KINDS`.
- Valid worker kinds are `notification`, `projection`, `compensation`, and
  `export`.
- PostgreSQL outbox remains the async boundary unless a new accepted spec
  replaces it.
- PostgreSQL remains the final truth for booking, ticket, check-in, reporting,
  and audit state.
- Kafka, RabbitMQ, NATS, SQS, service mesh, full microservices, and cross-region
  HA are deferred decision gates, not default deliverables.

## Redis Reservation Gate

Redis pre-admission may reduce hot-event database contention for limited events.
It is advisory only:

- `BOOKING_PREADMISSION=off` preserves the DB-only path.
- `BOOKING_PREADMISSION=on` requires Redis and hashed actor/idempotency inputs.
- PostgreSQL commit decides confirmed booking success.
- Redis outage must degrade to the DB-only path or return a controlled service
  error according to typed runtime config.
- Compensation reconciles leaked or stale holds from PostgreSQL truth.

## Reporting Projections

Reporting projections are derived and rebuildable. They may serve HR/admin
report reads and freshness signals, but must never authorize booking,
eligibility, ticket redemption, check-in, audit, or RBAC decisions.

## Event Type Registry

Event type registry (normative):

```text
registration.confirmed.v2
registration.cancelled.v2
registration.waitlisted.v2
registration.received.v2
registration.promoted.v2
ticket.issued.v2
ticket.revoked.v2
ticket.expired.v2
checkin.recorded.v2
notification.requested.v2
reservation.compensation.release_required.v2
report.export.requested.v2
report.export.completed.v2
report.export.failed.v2
hr_sync.batch.completed.v2
eligibility.impact_review.created.v2
reporting.projection.update_required.v2
```

Every v2 envelope carries `event_id`, `event_type`, `schema_version`,
`occurred_at`, `idempotency_key`, `partition_key`, and `payload`. Payloads must
not contain raw PII, signed ticket tokens, provider tokens, session tokens,
passwords, private keys, or signed download URLs.

## Phase 3 Boundary

Phase 3 local or bare-metal work may verify availability, observability,
capacity, release, and failure drills. Such assets still deploy the current
modular monolith and process-first worker model unless a new architecture spec
explicitly changes that boundary.
