# Phase 2 PH2-20 — Redis Reservation Gate Spec

> Child issue: `COR-60` / `[PH2-20]`. Parent: `COR-41` / `[PH2-WS3]`.
> Status: specification only. Runtime implementation belongs to `PH2-22`, `PH2-23`, and `PH2-27`.
> Anchors: `docs/specs/phase2-ws3-registration-hot-path.md`, `docs/specs/phase2-scale-hardening.md`, `docs/agent-rules/correctness.md`.

## 1. Goal

Define the Redis pre-admission reservation gate for limited-event booking bursts without changing the source of truth: PostgreSQL confirms or rejects every booking inside the existing Registration transaction. Redis only decides whether a request may enter the expensive DB path during pressure.

## 2. Acceptance Criteria

| AC | Given | When | Then |
| --- | --- | --- | --- |
| PH2-20-AC-1 | A limited event has remaining capacity | A unique booking request passes validation and idempotency lookup | Redis atomically decrements the advisory counter, records one short-lived hold, and returns `granted`; the DB transaction still owns final confirmation. |
| PH2-20-AC-2 | A limited event is exhausted in Redis | A new booking request arrives | Redis returns `exhausted`; the application returns the existing full or waitlist outcome. Any waitlist row must still be created by PostgreSQL, and no confirmed registration may be created from Redis alone. |
| PH2-20-AC-3 | The same idempotency tuple replays while a hold is active | The request reaches Redis again | Redis returns the existing reservation token without another decrement; the booking idempotency record decides the final replay response once persisted. |
| PH2-20-AC-4 | Redis is unavailable, times out, or evicts required keys | A booking is attempted | `REDIS_OUTAGE_MODE=degrade` falls back to DB-only admission; `REDIS_OUTAGE_MODE=fail` returns controlled `503` with retry metadata. Neither mode can oversell because DB checks remain mandatory. |
| PH2-20-AC-5 | Redis grants a reservation but the DB transaction rolls back or returns a non-confirmed limited-event result | Release or compensation runs | The advisory slot is returned at most once, capped by DB-derived remaining capacity, and an audit-safe conflict outcome is observable. |
| PH2-20-AC-6 | The gate is disabled | Booking traffic continues | Requests use the Phase 1 DB-only path; stale Redis keys drain through TTL and compensation without changing committed bookings. |

## 3. Request Order

The booking application service must execute the gate in this order:

1. Authorize the actor and validate request shape.
2. Reject limited-event family-count requests before any reservation.
3. Look up the booking idempotency record scoped to `(operation, event_id, actor_id, idempotency_key_hash)`. This composite scope refines the existing Phase 1 `booking_idempotency_results` columns `(idempotency_key, event_id, employee_id)` without changing the key shape or breaking replay semantics; the hash ensures raw client keys never appear in Redis or logs.
4. Return the stored result if it exists, including prior conflict, waitlist, or throttle outcomes.
5. For unlimited events, bypass capacity reservation and continue to the existing DB transaction.
6. For limited events with `BOOKING_PREADMISSION=on`, call the Redis reservation script.
7. Run the PostgreSQL transaction, rechecking eligibility, event state, booking window, allocation policy, capacity, duplicate booking, and idempotency.
8. Finalize the Redis hold based on the DB result.

Passing Redis admission is never sufficient to issue a registration or ticket.

## 4. Redis Key Shape

All keys use an app-owned prefix and version so a later incompatible rollout can run side by side.

| Key | Type | TTL | Semantics |
| --- | --- | --- | --- |
| `cets:v1:resv:{event_id}:remaining` | integer string | none | Advisory remaining limited-event slots. It mirrors DB-derived remaining capacity minus active holds, but is not authoritative. |
| `cets:v1:resv:{event_id}:hold:{idempotency_hash}` | hash | `RESERVATION_TTL_SECONDS` | One active pre-admission hold for the scoped booking tuple. Duplicate calls return this hold without another decrement. |
| `cets:v1:resv:{event_id}:pending` | sorted set | none | Members are `idempotency_hash`; score is Unix expiry time. Compensation scans this set after TTL plus grace. |
| `cets:v1:resv:{event_id}:drift` | string | short TTL | Optional marker set by compensation when the counter was rebuilt or capped from PostgreSQL. Used for observability only. |

`idempotency_hash` is an HMAC-SHA256 or equivalent keyed digest of the booking operation, event id, actor id, and client idempotency key. Raw client idempotency keys must not appear in Redis keys, logs, metrics, traces, or admin payloads.

## 5. Hold Value Semantics

The hold hash stores only redacted, operational fields:

| Field | Required | Meaning |
| --- | --- | --- |
| `schema_version` | yes | `1` for this spec. |
| `reservation_id` | yes | Server-generated opaque id returned by Redis. Safe to log. |
| `event_id` | yes | Event identifier. Not a booking authority. |
| `actor_hash` | yes | Redacted actor digest for replay/debug correlation. |
| `idempotency_hash` | yes | Redacted scoped idempotency digest. |
| `capacity_version` | yes | Event capacity/version observed when the counter was initialized. |
| `seats` | yes | Always `1` for limited events in Phase 2 WS3. |
| `created_at_unix` | yes | Server Unix timestamp. |
| `expires_at_unix` | yes | Server Unix timestamp used as the pending-set score. |
| `state` | yes | `reserved`; committed and released states remove the pending member instead of relying on this field as truth. |

Redis values must not contain full employee names, email addresses, provider tokens, signed ticket tokens, raw idempotency keys, or request bodies.

## 6. Lua Outcomes

The `PH2-22` Lua script must be atomic for one event and return exactly one of these outcomes:

| Outcome | Meaning | Application behavior |
| --- | --- | --- |
| `granted` | Counter was positive, decremented by one, hold was created, pending member was added. | Continue to DB transaction. |
| `duplicate` | A hold already exists for the same `idempotency_hash`. | Continue with the existing `reservation_id`; do not decrement again. |
| `exhausted` | Counter is zero or missing after initialization rules are applied. | Return full or waitlist response per existing allocation rules; do not treat as confirmed. |
| `misconfigured` | Required TTL, key, or capacity-version input is invalid. | Return controlled server error and emit redacted operational log. |

Counter initialization and rebuild must use PostgreSQL-derived remaining capacity and subtract active pending holds. Redis must never increase the advisory counter above DB-derived remaining capacity.

## 7. Configuration

`PH2-22` and `PH2-23` must add these environment variables when runtime code lands:

| Env var | Default | Purpose |
| --- | ---: | --- |
| `BOOKING_PREADMISSION` | `off` | `off` bypasses Redis and preserves the Phase 1 path; `on` enables reservation attempts for limited events. |
| `REDIS_OUTAGE_MODE` | `degrade` | `degrade` means fail open to DB-only admission; `fail` means fail closed with controlled `503` and retry metadata. |
| `RESERVATION_TTL_SECONDS` | `20` | Maximum time a Redis hold may exist before compensation may reclaim it. Must exceed normal booking DB p99 plus network jitter. |
| `RESERVATION_TTL_GRACE_SECONDS` | `10` | Extra time before compensation scans an expired pending hold. |
| `RESERVATION_COMPENSATION_INTERVAL_SECONDS` | `30` | Worker interval for scanning expired pending holds and counter drift. |
| `REDIS_OPERATION_TIMEOUT_MS` | `150` | Per Redis gate operation deadline, bounded by the HTTP request context. |
| `BOOKING_RESERVATION_HASH_SECRET` | unset | Secret used to hash idempotency and actor identifiers. Required when `BOOKING_PREADMISSION=on`. |

All values are deploy config. No default may rely on mutable in-process state.

## 8. Limited and Unlimited Event Paths

Limited events:

- Validate family-count rules before reservation. Limited events reserve one employee seat only.
- Call Redis only after an idempotency replay miss.
- Recheck DB capacity and duplicate booking inside the transaction even after `granted`.
- If DB confirms the booking, remove the pending member and hold without incrementing the counter.
- If DB rejects, waitlists, rolls back, or times out before commit, release the hold and increment the counter at most once.

Unlimited events:

- Do not create Redis reservation keys and do not decrement advisory counters.
- Preserve `family_count` semantics from Phase 1.
- Still require booking idempotency and duplicate booking checks.
- Still emit audit and outbox effects from PostgreSQL only.

## 9. DB Rollback and Ghost Reservation Reconciliation

The application should release a hold immediately after any non-confirmed DB outcome. Compensation exists for gaps such as process crash, context cancellation, Redis success followed by DB connection failure, or release timeout.

Compensation worker behavior:

1. Scan `pending` members with score older than `now - RESERVATION_TTL_GRACE_SECONDS`.
2. For each member, query PostgreSQL by `(event_id, idempotency_hash)` or the equivalent booking idempotency table.
3. If a confirmed limited-event booking exists, remove the pending member and hold without incrementing the counter.
4. If no confirmed booking exists, run a release script that increments the counter only if the pending member is still present.
5. Cap the counter to DB-derived remaining capacity after the release.
6. Emit a redacted metric/log entry with `released`, `confirmed_elsewhere`, `missing_hold`, or `counter_capped`.

Manual reconciliation may run as a same-binary one-off admin process, but it must use the same DB-derived cap and must not mutate committed booking rows.

## 10. Metrics, Logs, and Audit Safety

Metrics:

- `cets_reservation_attempt_total{outcome,capacity_type,outage_mode}`
- `cets_reservation_active{capacity_type}`
- `cets_reservation_compensation_total{action,result}`
- `cets_reservation_counter_drift_total{result}`
- `cets_booking_preadmission_latency_ms{outcome}`

Structured logs:

- Allowed fields: `trace_id`, `event_id`, `reservation_id`, `actor_hash`, `idempotency_hash`, `capacity_type`, `outcome`, `outage_mode`, `duration_ms`, `error_class`.
- Forbidden fields: raw idempotency key, provider token, signed ticket token, full employee PII, Redis URL credentials, request body.

Audit:

- Booking accept/reject/waitlist/conflict outcomes remain PostgreSQL audit facts.
- Redis-only operational events are logs/metrics unless they affect a user-visible throttle or rejection result.

## 11. Rollback and Disable Path

- Set `BOOKING_PREADMISSION=off` to route all booking traffic through the Phase 1 DB-only path.
- Keep the compiled Redis code path and tests in place until a follow-up removal spec exists.
- Leave existing Redis keys to expire or let the `compensation` worker drain them; do not delete keys manually during active booking windows.
- If Redis causes operational instability while the gate is on, set `REDIS_OUTAGE_MODE=degrade` before disabling worker compensation.
- Omit `compensation` from `WORKER_KINDS` only after active holds have drained or manual reconciliation has verified zero pending holds.
- Rollback must not require schema rollback or destructive data changes.

## 12. Non-Goals

- No middleware, Lua script, database migration, worker implementation, OpenAPI change, or frontend behavior is part of `PH2-20`.
- Redis reservation success is not booking success.
- Redis is not a source of truth for tickets, check-in, reporting, eligibility, or audit.
- This spec does not introduce an independently deployed Registration service.
- This spec does not change Phase 1 family-count, cooldown, cutoff, lottery, or waitlist semantics.

## 13. Verification for This Spec

- `git diff --check`
- `cd services/api && go test ./internal/architecture -run TestPhase2DocsDoNotClaimDeferredInfraIsRequired -count=1`
- Confirm the diff is limited to PH2-20 documentation and references.
