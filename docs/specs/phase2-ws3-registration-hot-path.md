# Phase 2 WS3 — Registration Hot Path

> Workstream-level spec. Parent: `COR-41` / `[PH2-WS3]`. Owner: Person C.
> Anchors: `docs/specs/phase2-scale-hardening.md` (§1, §3.3, §4), Phase 1 baseline `docs/specs/phase1-production-upper-bound.md` (AC-5, E-15..E-18), correctness rules in `CLAUDE.md`.

## 1. Summary

WS3 owns the booking hot path under Phase 2 load: a Redis pre-admission gate that sheds load before PostgreSQL contention; event/actor rate limiting; Redis Lua reservation with TTL + compensation; idempotency result hardening (same key, same response, no duplicate side effects); hot-row contention reduction (advisory lock vs row lock vs queue vs counter sharding — chosen on evidence, not preference); a read-only capacity-pressure admin API for ops; and regression tests proving Redis-outage / Redis-timeout / TTL-expiry / DB-rollback / idempotent-retry cases never oversell, never duplicate, and always reach a definite outcome.

PostgreSQL remains the only source of truth for confirmed bookings. Redis is pre-admission only; final eligibility, capacity, window, and allocation policy recheck happens inside the DB transaction.

## 2. Scope

In scope (`PH2-20`..`PH2-27`):

- `PH2-20` Redis reservation gate spec — semantics, invariants, failure modes. Dedicated spec: `docs/specs/phase2-redis-reservation-gate.md`.
- `PH2-21` Event/actor rate-limit middleware (token bucket or sliding window; per `(event_id, employee_id)` and per `(event_id, *)`).
- `PH2-22` Redis Lua pre-admission reservation (atomic check + decrement + TTL set).
- `PH2-23` Reservation TTL and compensation (lapsed reservation returns capacity without admin action; reconciliation worker handles edge gaps).
- `PH2-24` Booking idempotency result hardening (idempotency key uniqueness; replay returns the exact prior result, including conflict outcomes).
- `PH2-25` Hot-row contention reduction (decision: advisory lock vs `SELECT FOR UPDATE` vs counter sharding vs queue; chosen on `PH2-16` evidence).
- `PH2-26` Capacity pressure admin API (read-only; consumed by WS5 ops UI).
- `PH2-27` Redis failure + oversell regression tests.

Out of scope:

- Worker / outbox behavior, retry, dead-letter, replay — WS4.
- Reporting projections, freshness, ops UI surface — WS5.
- Trace/metric instrumentation — WS2 (WS3 emits metrics WS2 defined; does not own metric pipeline).
- Check-in cache — gated by `PH2-47` decision gate (WS5).
- Frontend booking UI changes — only consume rate-limit / capacity-pressure responses; UI redesign deferred.

## 3. Acceptance Criteria

| AC | Given | When | Then |
| --- | --- | --- | --- |
| WS3-AC-1 | A limited event at capacity | Two concurrent booking attempts arrive | At most one confirms; the other returns a structured "full" or "waitlisted" response; no oversell row exists. |
| WS3-AC-2 | Redis pre-admission gate is enabled | A booking passes Redis admission | DB transaction still rechecks eligibility, event state, capacity, booking window, allocation policy before commit; passing Redis is not sufficient. |
| WS3-AC-3 | Redis is unavailable (connection refused, timeout, key eviction) | A booking is attempted | Either (a) the system degrades to DB-only admission with documented latency cost, or (b) the request returns a controlled 503 with retry-after; under no path does Redis outage cause oversell or duplicate booking. Choice is configurable per `PH2-20`. |
| WS3-AC-4 | An idempotency key has been used | The same key replays (network retry, client retry) | Response payload + side effects match the original exactly, including conflict / waitlisted / cooldown outcomes; no second registration row, no second ticket, no second audit row. |
| WS3-AC-5 | A reservation is created in Redis but the DB tx is rolled back | TTL elapses or compensation runs | The reserved slot returns to the pool; subsequent bookings can consume it; no orphaned reservation accumulates. |
| WS3-AC-6 | Rate limit threshold for `(event_id, employee_id)` is exceeded | Further requests arrive in the window | 429 with retry-after; no DB row created; audit row records the throttle for the actor (no PII beyond existing audit redaction). |
| WS3-AC-7 | Hot-row contention reduction is shipped | `PH2-16` baseline is re-run on the same fixture | Booking tx duration p95 decreases (or DB pool wait p95 decreases) by a margin recorded in the PR; if neither moves, the change is reverted. |
| WS3-AC-8 | An ops admin calls `GET /api/v1/admin/ops/capacity-pressure` | Hot events are running | Response returns per-event remaining capacity, reservation count, queue depth, rate-limit drop rate, idempotency-replay rate — all derived, never authoritative; no booking commit reads this surface. |
| WS3-AC-9 | Regression suite runs | Redis-outage, Redis-timeout, TTL-expiry, DB-rollback, idempotent-retry, duplicate-booking, oversell scenarios are exercised | All assertions pass; new cases are added before any hot-path code change is merged. |

## 4. Edge Cases

| # | Scenario | Expected behavior |
| --- | --- | --- |
| WS3-E-1 | Redis returns "OK" then crashes before the DB tx commits | DB tx rollback → reservation TTL expires → capacity returns. Compensation worker reconciles stuck reservations beyond TTL grace. |
| WS3-E-2 | Two requests with the same idempotency key arrive in parallel (cold key) | Unique constraint on `idempotency_key` serializes; first commits, second reads the prior result and returns it; no duplicate registration. |
| WS3-E-3 | Reservation TTL elapses while DB tx is mid-flight | DB tx still recheck-protected by capacity constraint; if capacity exhausted, tx rolls back and returns "full" rather than overselling. |
| WS3-E-4 | Rate limit middleware is bypassed by direct internal call (admin script) | Booking service-level guards still enforce idempotency, eligibility, capacity — middleware is defense-in-depth, not the only gate. |
| WS3-E-5 | Family-count booking against unlimited event arrives during pressure | Pre-admission gate does not decrement (no inventory); only `(event_id, employee_id)` idempotency applies; Phase 1 E-16 semantics preserved. |
| WS3-E-6 | Limited event booking includes family members | Validation error before reservation; no Redis decrement, no DB row (Phase 1 E-15 preserved). |
| WS3-E-7 | Hot-row contention change introduces a latent deadlock | Statement timeout + retry middleware surfaces it as a non-2xx; regression test from `PH2-27` includes deadlock injection. |
| WS3-E-8 | Capacity pressure API is queried while a booking commits | Snapshot may be slightly stale; response includes `as_of` timestamp; ops UI shows freshness; never used as commit authority. |
| WS3-E-9 | Idempotency key reuse across different `(event_id, employee_id)` (client bug) | Unique constraint is scoped to the booking operation key shape (key + event + actor); collision returns the original result for the original tuple, never confuses identities. |

## 5. Non-Functional Requirements

| Category | Requirement | Metric |
| --- | --- | --- |
| Correctness invariant | Confirmed bookings ≤ event capacity, always. | DB unique + check constraints; regression suite proves under load. |
| Correctness invariant | At most one confirmed booking per `(event_id, employee_id)`. | Unique constraint on `(event_id, employee_id, status='confirmed')` or equivalent. |
| Latency | Booking HTTP p95 < 750ms, p99 < 1500ms; server-side booking tx p99 < 500ms (per acceptance matrix §1.3). | k6 hot-event profile (`PH2-11`). |
| Degradation | Redis outage never causes oversell or duplicate; either degrades to DB-only or 503. | `PH2-27` Redis-outage tests. |
| Idempotency cost | Replay path adds no extra DB writes beyond a lookup. | Code review + integration test. |
| Audit | Throttle and rejection outcomes are auditable. | Audit rows for reject/throttle; redaction preserved. |
| Backpressure | Rate-limit drop rate is observable. | Metric `cets_rate_limit_drop_total` (defined by WS2). |

## 6. Minimal API / Data Contract

Reservation gate (Redis side; detailed normative contract lives in `docs/specs/phase2-redis-reservation-gate.md`):

```text
Counter:      cets:v1:resv:{event_id}:remaining     # remaining slots mirror, NOT authority
Hold:         cets:v1:resv:{event_id}:hold:{idempotency_hash}
Pending:      cets:v1:resv:{event_id}:pending
Lua script:   atomic CHECK count > 0 → DECR → SET hold TTL=N → ZADD pending → return reservation_id
Outcome:      "granted" | "exhausted" | "duplicate" | "misconfigured"
TTL:          configurable via RESERVATION_TTL_SECONDS (default defined by PH2-20)
Compensation: scheduled worker reconciles ghost reservations beyond TTL + grace
```

Booking endpoint (existing `POST /api/v1/events/{event_id}/bookings`, behavior delta only):

```text
Request:    Idempotency-Key header REQUIRED (matches Phase 1; unchanged shape)
Response:   { success, data: { registration_id, status: "confirmed"|"waitlisted"|"rejected", reason?, retry_after_seconds? }, error }
Status:     200 (confirmed/waitlisted), 409 (duplicate non-replay), 429 (rate-limited), 503 (Redis-degrade with retry-after, only if configured)
Replay:     same idempotency key + tuple → 200 with original response shape; never a new side effect
```

Capacity pressure admin API (additive, `PH2-26`; payload shape lives here, OpenAPI added by WS1 `PH2-02`):

```text
GET /api/v1/admin/ops/capacity-pressure
Auth:    activity_admin or hr_admin per existing RBAC
Response:
{
  "success": true,
  "data": {
    "as_of": "2026-05-19T12:34:56Z",
    "events": [
      {
        "event_id": "...",
        "capacity_type": "limited|unlimited",
        "remaining_capacity": 12,
        "reservation_count": 5,
        "rate_limit_drop_per_min": 3,
        "idempotency_replay_per_min": 7
      }
    ]
  },
  "meta": { "source": "derived", "freshness_seconds": 5 },
  "error": null
}
```

This endpoint is read-only, derived, never the booking commit authority.

## 7. 12-Factor Notes

- **Config**: New env vars include `BOOKING_PREADMISSION`, `RESERVATION_TTL_SECONDS`, `RESERVATION_TTL_GRACE_SECONDS`, `RESERVATION_COMPENSATION_INTERVAL_SECONDS`, `REDIS_OPERATION_TIMEOUT_MS`, `BOOKING_RESERVATION_HASH_SECRET`, `BOOKING_RATE_LIMIT_RPS_PER_ACTOR`, `BOOKING_RATE_LIMIT_RPS_PER_EVENT`, and `REDIS_OUTAGE_MODE=degrade|fail`. Defaults and rollout rules are defined by `PH2-20`; runtime PRs must add them to `services/api/deploy/.env.example`.
- **Backing services**: Redis is already an attached resource; this WS upgrades its role from optional cache to required pre-admission gate when `REDIS_OUTAGE_MODE=fail`. Connection injected via `REDIS_URL` (unchanged).
- **Build / release / run**: Same Go binary; no new process types. Compensation runs inside the existing same-binary worker as a new kind (`worker_kind=compensation`) — WS4 owns kind config (`PH2-31`).
- **Processes**: Stateless; reservation state lives in Redis + DB. No in-memory authoritative state.
- **Logs**: Structured JSON to stdout; fields per WS2 §6. No idempotency key values, no Redis keys containing PII, no signed tokens.
- **Admin processes**: New one-off `cets ops reservation-reconcile --event-id=...` for manual compensation; documented but rarely needed.
- **Disposability**: Booking handler honors request context cancellation; partial Redis reservation without DB commit is reclaimed by TTL / compensation.
- **Dev/prod parity**: Same Redis Lua script in dev / CI / production; CI uses real Redis (Compose).

## 8. Tests / Verification

- `go test ./internal/ticketing -run TestBookingHotPath -count=1` — covers AC-1..AC-6, AC-9 with real PostgreSQL + Redis (Compose-backed).
- `go test ./internal/ticketing -run TestReservationCompensation -count=1` — TTL expiry + compensation worker correctness.
- `go test ./internal/ticketing -run TestIdempotencyReplay -count=1` — identical response on replay, no duplicate side effects.
- `go test ./internal/httpapi -run TestCapacityPressureAPI -count=1` — RBAC, read-only shape, freshness metadata.
- k6 hot-event regression (`k6/phase2-hot-event.js`, owned by WS2) re-run before/after `PH2-25` to satisfy AC-7.
- Redis-outage scenario test: integration test toggles Redis off mid-run; asserts no oversell, no duplicate, no orphan reservation.
- `ruby scripts/check-openapi-contract.rb` after `PH2-02` lands the capacity-pressure path.

## 9. Rollback / Disable

Every WS3 runtime change ships behind a disable path; reviewer rejects a PR that lacks one.

- Redis pre-admission gate: `BOOKING_PREADMISSION=off` short-circuits to Phase 1 DB-only path. Code path stays compiled (no removal) until baseline shows the gate is stable.
- Rate limit middleware: `BOOKING_RATE_LIMIT_RPS_PER_ACTOR=0` and `..._PER_EVENT=0` disable enforcement; middleware logs would-be drops at debug level for observation.
- Hot-row contention change (`PH2-25`): chosen approach (advisory lock / sharded counter / queue) is selected via `BOOKING_CONTENTION_STRATEGY=phase1|advisory|sharded|queue`; default during rollout is `phase1`; flip after evidence.
- Capacity pressure admin API: disabled by removing the route registration behind `OPS_API_ENABLED=true`; default off until WS5 ops UI consumes it.
- Compensation worker kind: WS4-managed kind flag (`WORKER_KINDS=...`); omit the kind to disable.
- Migrations: any new index (e.g. on `idempotency_key`) is additive; rollback is `DROP INDEX CONCURRENTLY` via a follow-up migration, never a destructive change of existing tables.

## 10. Non-Goals

- Do not treat Redis reservation success as booking success. Final truth is the DB tx outcome.
- Do not implement check-in caching as part of WS3 — it is a separate decision gate (`PH2-47`, WS5).
- Do not introduce a separately deployed booking microservice. Phase 2 stays modular monolith.
- Do not change Phase 1 family-count / cooldown / cutoff semantics; only harden them under pressure.
- Do not change OpenAPI shapes outside `PH2-02` scope; payload deltas in §6 are normative input to `PH2-02`, not a substitute.
- Do not log idempotency keys, Redis keys with PII, or signed tokens.

## 11. Cross-Stream Dependencies

| Direction | Stream | Contract consumed / produced |
| --- | --- | --- |
| Consumes | WS1 | OpenAPI delta for capacity-pressure path (`PH2-02`); reviewer checklist (`PH2-07`). |
| Consumes | WS2 | Baseline report (`PH2-16`) gating `PH2-22`/`PH2-23`/`PH2-25`; pool/lock/tx metrics; trace schema. |
| Consumes | WS4 | Worker kind config (`PH2-31`) for compensation worker; outbox envelope (`PH2-30`) if compensation emits events. |
| Produces | WS5 | Capacity pressure API payload (§6) for ops UI (`PH2-46`). |
| Produces | WS2 | New metric labels (`cets_rate_limit_drop_total`, `cets_reservation_outcome_total`) — registered by WS3, exported via WS2 wiring. |
