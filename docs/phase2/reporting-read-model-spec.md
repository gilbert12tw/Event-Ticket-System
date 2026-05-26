# Phase 2 Reporting Read Model — Specification

**Issue:** PH2-40  
**Status:** Draft — pending reviewer sign-off. PH2-41 has already merged the
`reporting_event_summary` / `reporting_projection_offsets` tables with a
`last_processed_at`-only cursor; the composite-cursor column in §3 is a required
follow-up `ALTER TABLE` (see §3).  
**Reviewers:**
- Person A (WS1): privacy whitelist compliance, non-authoritative language, non-goals completeness
- Person B (WS2): freshness SLA measurability, load-test criteria, baseline alignment

---

## 1. Purpose and Scope

### Why projections exist in Phase 2

Phase 1 reporting queries run directly against OLTP tables (`events`, `registrations`,
`tickets`). Under the Phase 2 load profile (50 k employees, hot-event bursts) these queries
create read/write contention on the same rows that the booking hot path updates. Projection
tables isolate reporting I/O from transactional I/O.

### What projections replace

Direct `SELECT … FROM registrations JOIN events …` queries issued by reporting endpoints and
export jobs. After Phase 2, those endpoints read from `reporting_event_summary` instead.

### Non-authoritative declaration

> **Projections are derived, disposable, and rebuildable. They are not the source of truth for
> any booking, capacity, eligibility, or access-control decision.**
>
> If a projection row states that an employee is confirmed, that does not mean they hold a
> valid ticket. The PostgreSQL OLTP tables (`events`, `registrations`, `tickets`) remain the
> sole authoritative source of truth.

Projections must never be read by:
- Check-in validation or ticket redemption flows
- Authorization or role-check logic
- Eligibility evaluation
- Audit log generation

---

## 2. Source Events and Tables

The projection worker consumes two kinds of sources:

| Source | Kind | Fields Consumed |
|---|---|---|
| `booking.confirmed` | Outbox event (incremental) | `event_id`, `registration_id`, `employee_department`, `occurred_at` |
| `registration.cancelled` | Outbox event (incremental) | `event_id`, `registration_id`, `occurred_at` |
| `booking.waitlisted` | Outbox event (incremental) | `event_id`, `registration_id`, `occurred_at` |
| `waitlist.promoted` | Outbox event (incremental) | `event_id`, `registration_id`, `employee_department`, `occurred_at` |
| `events` | OLTP direct read (rebuild only) | `event_id`, `title`, `starts_at`, `capacity_type`, `capacity` |
| `registrations` | OLTP direct read (rebuild only) | `registration_id`, `event_id`, `status`, `employee_id`, `created_at` |
| `employees` | OLTP direct read (rebuild only) | `employee_id`, `department` (for department breakdown aggregation only) |

`total_capacity` has no incremental outbox source: the Phase 1/2 producers emit no
capacity-change event (a capacity edit writes only an audit entry, not an `outbox_events`
row). It is therefore maintained on the **rebuild path only** — read from `events` (§5 step 4).
A capacity edit between rebuilds is reflected in the projection only after the next rebuild.

**Incremental path (normal operation):** the worker polls `outbox_events` using a composite
cursor `(last_processed_at, last_processed_outbox_id)` stored in
`reporting_projection_offsets`. The resume predicate is:

```sql
WHERE (created_at, outbox_id) > (:last_processed_at, :last_processed_outbox_id)
ORDER BY created_at ASC, outbox_id ASC
```

After each batch is applied, both `last_processed_at` and `last_processed_outbox_id` are
updated atomically to the values of the last row in that batch. Using a composite tuple
comparison avoids the skip risk that a pure `created_at >` watermark has when multiple
events share the same timestamp: every row is identified uniquely by the `(created_at,
outbox_id)` pair, so no event is ever silently skipped.

**Rebuild path:** the worker reads OLTP tables directly, aggregates in-process, and bulk-inserts
into `reporting_event_summary`. See §5 for the full rebuild procedure.

### Fields never consumed

The following fields must never appear in a projection-bound query or payload:
- `full_name`, `email`, any employee PII
- `signed_token`, `qr_payload`, `provider_token`
- Raw `audit_logs` rows
- `signed_token_hash`

---

## 3. Projection Keys and Schema Sketch

> DDL belongs in PH2-41. This section defines the logical shape only.
>
> **PH2-41 drift note:** PH2-41 already merged these tables with a `last_processed_at`-only
> cursor (no `last_processed_outbox_id` column, and the offset row seeded with
> `(projection_name, last_processed_at)` only). The `last_processed_outbox_id` column below is
> part of the composite-cursor upgrade and must be added via a follow-up `ALTER TABLE` (with a
> backfill to `''`) before the PH2-42 projection worker is built.

### `reporting_event_summary`

| Column | Type | Notes |
|---|---|---|
| `event_id` | TEXT (PK) | References OLTP `events.event_id`; no FK constraint (projection is disposable) |
| `total_capacity` | INTEGER ≥ 0 | 0 for unlimited events; maintained on rebuild only (no incremental capacity event — see §2) |
| `confirmed_count` | INTEGER ≥ 0 | Count of registrations with status `confirmed` |
| `cancelled_count` | INTEGER ≥ 0 | Count of registrations with status `cancelled` |
| `waitlist_count` | INTEGER ≥ 0 | Count of registrations with status `waitlisted` |
| `department_breakdown` | JSONB | Map of `{ "department_label": integer_count }` — no names or IDs |
| `last_processed_at` | TIMESTAMPTZ | `created_at` of the last outbox event applied to this row |
| `last_processed_outbox_id` | TEXT | `outbox_id` of the last outbox event applied (tiebreaker for same-second events) |
| `updated_at` | TIMESTAMPTZ | Wall-clock time of last projection write |

`confirmed_count <= total_capacity` is intentionally not enforced at the projection layer.
OLTP enforces capacity. The projection reflects a derived count; capacity enforcement is the
OLTP's responsibility.

### `reporting_projection_offsets`

| Column | Type | Notes |
|---|---|---|
| `projection_name` | TEXT (PK) | Logical name of the projection, e.g. `event_summary` |
| `last_processed_at` | TIMESTAMPTZ | `created_at` of the last `outbox_events` row successfully applied |
| `last_processed_outbox_id` | TEXT | `outbox_id` of the last `outbox_events` row successfully applied; tiebreaker for the composite cursor |
| `updated_at` | TIMESTAMPTZ | Wall-clock time of last offset commit |

One row per projection. The shipped PH2-41 migration seeds `last_processed_at = '-infinity'`.
Once `last_processed_outbox_id` is added (see drift note above), it must be backfilled to `''`
so the first poll's tuple comparison `(created_at, outbox_id) > ('-infinity', '')` picks up all
existing outbox events.

---

## 4. Staleness SLA

### Targets

| Metric | Target |
|---|---|
| Outbox event lag p95 | < 60 s |
| Outbox event lag max | < 180 s |
| Reporting projection freshness p95 | < 60 s |

Freshness is defined as `now() - updated_at` on the relevant `reporting_event_summary` row,
measured at query time.

### How freshness is measured

1. **API layer:** the reporting endpoint reads `MAX(updated_at)` from
   `reporting_event_summary` (or the most recently queried row's `updated_at`) and sets
   `X-Report-Freshness: <ISO-8601 timestamp>` on every response.
2. **Ops UI:** a staleness banner is shown when `now() - freshness_timestamp > 120s` (2×
   the p95 target, giving one SLA window of grace before alerting the operator).
3. **Internal health check:** the worker exposes a `/healthz/projection` endpoint that
   returns HTTP 200 when lag < 60 s and HTTP 503 when lag ≥ 180 s (max SLA breach).

### Behaviour when stale beyond threshold

- **p95 breach (> 60 s):** ops UI shows a yellow staleness warning. Report responses
  continue to be served with the stale data and include the `X-Report-Freshness` header so
  callers can decide whether to retry.
- **Max breach (> 180 s):** worker health check returns HTTP 503. An ops alert fires (PH2-44
  alert contract). Report endpoints remain available but prepend a stale-data disclaimer to
  the response body and set `X-Report-Freshness-Status: stale`.
- **Booking hot path is never blocked** — staleness of the projection does not prevent
  bookings, cancellations, or check-ins.

---

## 5. Rebuild Path

A full rebuild is initiated by an admin and replaces all projection data from OLTP source
tables. Reads served during a rebuild see stale data (stale-read tolerance); the API is never
locked.

### Step-by-step

1. **Trigger:** system admin calls `POST /admin/ops/reporting/rebuild` (PH2-44 API) or runs
   `cets admin rebuild-reporting` (CLI).  
   Role required: `system_admin`.

2. **Reset offset:** the worker updates `reporting_projection_offsets` SET
   `last_processed_at = '-infinity'`, `last_processed_outbox_id = ''`, `updated_at = now()`
   WHERE `projection_name = 'event_summary'`. This causes the incremental poll loop to resume
   from the beginning of the outbox after rebuild completes.

3. **Truncate projection:** the worker executes `TRUNCATE reporting_event_summary`.  
   No OLTP table is touched.

4. **Replay from OLTP:**
   - Read all `registrations` rows, joined to `employees` for department only.
   - Aggregate `confirmed_count`, `cancelled_count`, `waitlist_count`,
     `department_breakdown` per `event_id` in-process.
   - Read `events` for `total_capacity`.
   - Bulk-insert into `reporting_event_summary` using `INSERT … ON CONFLICT DO UPDATE`.

5. **Advance offset:** after replay completes, set `last_processed_at` to
   `SELECT MAX(created_at) FROM outbox_events` and `last_processed_outbox_id` to the
   `outbox_id` of that row, so incremental processing resumes from the right position
   and does not redundantly re-apply events already reflected in the OLTP snapshot.
   If `outbox_events` is empty, leave both at their seed values (`'-infinity'`, `''`).

6. **Resume incremental processing:** the worker's normal poll loop continues from the new
   offset.

### In-progress reads during rebuild

Reporting reads are served from the projection table throughout the rebuild. Between steps 3
and 4 the table is empty; the API returns `X-Report-Freshness-Status: rebuilding` and an
empty data set rather than erroring. The booking hot path is unaffected.

---

## 6. Failure Modes

| Failure | Expected Behaviour |
|---|---|
| Outbox worker crashes mid-event | Projection falls behind. Freshness SLA breach detected via `X-Report-Freshness` and health check. On restart, worker reads the composite cursor `(last_processed_at, last_processed_outbox_id)` from `reporting_projection_offsets` and resumes from that exact position — no event is lost or double-counted because the cursor identifies each event uniquely. |
| Duplicate outbox event delivered | The composite cursor predicate `(created_at, outbox_id) > (last_processed_at, last_processed_outbox_id)` excludes already-applied events exactly. For any event that does slip through (e.g. worker restart mid-batch before the cursor was committed), the upsert uses `ON CONFLICT DO UPDATE` with no net state change, making the apply step idempotent. |
| Projection table corrupted or schema mismatch | OLTP tables are entirely unaffected. Admin triggers a full rebuild (§5). The projection is restored from OLTP source truth. |
| Stale read model served to report API | Response includes `X-Report-Freshness` timestamp. UI shows staleness banner when age > 120 s. Booking, check-in, and eligibility flows are never gated on projection freshness. |
| Projection worker starved by notification worker | Prevented by worker-kind split (PH2-31). The reporting projection worker runs as a separate process (`cets worker --kind reporting`), isolated from the notification worker (`cets worker --kind notification`). Both consume the same outbox but neither can starve the other. |

---

## 7. RBAC and PII Redaction

### Storage rules

- `reporting_event_summary` stores **only aggregate counts and JSONB maps of label → count**.
- No employee-level rows, no `employee_id`, no `full_name`, no `email`, no `token` of any
  kind are present in any projection table.
- `department_breakdown` stores department label strings (e.g. `"Engineering"`) mapped to
  integer counts. Individual employee identity is never recoverable from a projection row.

### Access rules

- Read endpoints for projection data: `activity_admin`, `system_admin`.
- Rebuild and offset-reset operations: `system_admin` only.
- Export endpoints (PH2-45): enforce the same privacy whitelist — only aggregate fields are
  included in any export payload or file.

### Enforcement guarantee

Because projection tables contain no PII, a full table dump of `reporting_event_summary` or
`reporting_projection_offsets` does not constitute a PII breach. This is an intentional design
property; it must be preserved through all future schema changes.

---

## 8. Load-Test Acceptance Criteria

Projection reads must be validated under the Phase 2 load profile before PH2-42 is marked
complete:

| Criterion | Target |
|---|---|
| Seed size | 50 k employees, events at Phase 2 scale |
| Load profile | Hot-event k6 script (PH2-11) — concurrent booking burst |
| Reporting freshness p95 under load | < 60 s |
| Projection query p99 latency | < 50 ms |
| Booking hot-path p99 latency delta | < 5 ms added versus baseline (projections must not slow bookings) |

The load test must run with both the projection worker active (incremental mode) and
immediately after a rebuild, to verify that freshness recovers within SLA after a TRUNCATE.

---

## 9. Non-Goals

This spec explicitly does not cover the following; they belong in later issues:

| Topic | Covered by |
|---|---|
| Schema DDL and migrations | PH2-41 |
| Projection worker implementation | PH2-42 |
| Rebuild process implementation | PH2-43 |
| API freshness header and staleness contract | PH2-44 |
| Export endpoints | PH2-45 |
| Ops UI staleness banner | PH2-46 |
| Check-in offline cache decision | PH2-47 (decision gate) |

**Projections must never be used for authorization, eligibility checks, audit logs, or any
decision that affects booking or check-in correctness.** Any future PR that reads a projection
table in a non-reporting code path must be rejected in review.

---

## Verification Checklist (PH2-40)

- [x] All 9 sections present and complete
- [x] Spec explicitly states projections are non-authoritative
- [x] Privacy whitelist documented (no employee rows, no tokens, no PII)
- [x] Rebuild path is step-by-step and covers offset reset
- [x] All five failure modes addressed
- [x] Load-test acceptance criteria reference Phase 2 targets
- [x] Non-goals section explicitly excludes schema, worker, API, UI, and check-in cache
