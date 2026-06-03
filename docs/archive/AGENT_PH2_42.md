# AGENT.md: PH2-42 — Read Model Worker

## Before You Start

1. Create a new branch from `main`:
   ```bash
   git checkout main && git pull
   git checkout -b feat/ph2-42-read-model-worker
   ```
2. Confirm the following are already merged:
   - **PH2-40** — Reporting read model spec
   - **PH2-41** — `reporting_event_summary` and `reporting_projection_offsets` tables exist in DB
   - **PH2-30** — Outbox envelope v2 (`schema_version`, `idempotency_key`, `partition_key`)
   - **PH2-31** — Worker kind config (`WORKER_KINDS` env var, `projection` kind is registered)
3. Do not start implementation until you confirm the above tables exist in
   `services/api/internal/postgres/migrate.go`.

---

## Feature Summary

Implement a `projection` worker kind that consumes outbox events of type
`reporting.projection.update_required.v2` and updates `reporting_event_summary` idempotently.

The worker must:
- Process events exactly once (idempotent upsert using `last_event_offset`)
- Never double-count on replay or duplicate delivery
- Update `reporting_projection_offsets` after each successful batch
- Expose lag metrics compatible with PH2-14 / PH2-15 observability

**The projection is read-only truth for reporting. It must never be used to authorize
bookings, check-ins, eligibility, or ticket redemption.**

---

## Acceptance Criteria

| # | Criterion |
|---|---|
| AC-1 | Worker consumes `reporting.projection.update_required.v2` events and updates `reporting_event_summary` aggregate counts |
| AC-2 | Replaying the same event twice does not change projection counts (idempotent upsert) |
| AC-3 | Worker processes `booking.confirmed`, `booking.cancelled`, `booking.waitlisted`, and `checkin.completed` event payloads correctly |
| AC-4 | `reporting_projection_offsets.last_processed_outbox_id` advances after each successful event |
| AC-5 | Worker lag (time between outbox event created and projection updated) is measurable via metrics |
| AC-6 | Worker kind and concurrency are env-configured via `WORKER_KINDS` and `PROJECTION_WORKER_CONCURRENCY` |
| AC-7 | Projection worker does not process notification, export, or compensation events |
| AC-8 | On worker crash and restart, processing resumes from `last_processed_outbox_id` without data loss or double-count |

---

## Event Payload Contract

The worker consumes outbox events where `event_type = 'reporting.projection.update_required.v2'`.

The payload inside the outbox envelope contains the original domain event. The worker must
handle the following inner event types:

| Inner Event Type | Projection Effect |
|---|---|
| `booking.confirmed` | `confirmed_count += 1`; update `department_breakdown` |
| `booking.cancelled` | `confirmed_count -= 1`; `cancelled_count += 1` |
| `booking.waitlisted` | `waitlist_count += 1` |
| `booking.waitlist_cancelled` | `waitlist_count -= 1` |
| `checkin.completed` | no aggregate count change (checkin is not a projection field) |
| unknown type | log warning, skip, do NOT dead-letter — projection worker is tolerant |

**Counts must never go below 0.** Use `GREATEST(col - 1, 0)` in SQL for decrement operations.

---

## Idempotency Design

Each outbox event has a unique `id` (bigint). The projection row stores `last_event_offset`.

Upsert logic for `reporting_event_summary`:

```sql
INSERT INTO reporting_event_summary (
    event_id, confirmed_count, cancelled_count, waitlist_count,
    department_breakdown, last_event_offset, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (event_id) DO UPDATE SET
    confirmed_count    = CASE WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
                              THEN excluded.confirmed_count
                              ELSE reporting_event_summary.confirmed_count END,
    cancelled_count    = CASE WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
                              THEN excluded.cancelled_count
                              ELSE reporting_event_summary.cancelled_count END,
    waitlist_count     = CASE WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
                              THEN excluded.waitlist_count
                              ELSE reporting_event_summary.waitlist_count END,
    department_breakdown = CASE WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
                              THEN excluded.department_breakdown
                              ELSE reporting_event_summary.department_breakdown END,
    last_event_offset  = GREATEST(excluded.last_event_offset,
                                  reporting_event_summary.last_event_offset),
    updated_at         = CASE WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
                              THEN now()
                              ELSE reporting_event_summary.updated_at END
```

This ensures replaying an older event never overwrites a newer projection state.

---

## Affected Files

| File | Change |
|---|---|
| `services/api/internal/ticketing/projection_worker.go` | **New file.** Core worker loop: claim → decode → upsert → advance offset |
| `services/api/internal/ticketing/projection_worker_test.go` | **New file.** Integration tests (see Test Cases below) |
| `services/api/internal/ticketing/projection_models.go` | **New file.** `ProjectionEvent` struct for decoded inner payload |
| `services/api/internal/worker/worker.go` | Register `projection` worker kind; wire `ProjectionWorker` to outbox claim loop |
| `services/api/internal/postgres/queries_projection.go` | **New file.** `UpsertEventSummary`, `AdvanceProjectionOffset`, `GetProjectionOffset` queries |
| `services/api/internal/metrics/projection_metrics.go` | **New file.** `projection_worker_lag_seconds` histogram; `projection_events_processed_total` counter |
| `services/api/deploy/.env.example` | Add `PROJECTION_WORKER_CONCURRENCY=2` |

### New file size budget

- `projection_worker.go`: target ≤ 120 lines
- `projection_worker_test.go`: target ≤ 250 lines
- `queries_projection.go`: target ≤ 80 lines

---

## Detailed Logic

### 1. Worker loop (`projection_worker.go`)

```
claimOutboxEvent(kind = "projection")
  → filter: event_type = 'reporting.projection.update_required.v2'
  → decode outer envelope (schema_version check)
  → decode inner payload → ProjectionEvent{EventID, InnerType, Department, ...}
  → computeNewCounts(currentRow, ProjectionEvent)
  → upsertEventSummary(tx, eventID, newCounts, outboxID)
  → advanceProjectionOffset(tx, 'event_summary', outboxID)
  → recordLagMetric(outboxEvent.CreatedAt, now())
  → markOutboxPublished(outboxID)
```

### 2. Lag metric

```go
lagSeconds := time.Since(outboxEvent.CreatedAt).Seconds()
projectionLagHistogram.Observe(lagSeconds)
```

Metric name: `projection_worker_lag_seconds` (histogram)
Buckets: 1s, 5s, 15s, 30s, 60s, 120s, 180s

This integrates with PH2-14 (queue lag metrics) and PH2-15 (trace/log schema).

### 3. Department breakdown update

`department_breakdown` is a JSONB column storing `{ "department_label": count }`.

Update logic (in Go, before upsert):
```go
breakdown := currentRow.DepartmentBreakdown  // map[string]int
if event.InnerType == "booking.confirmed" {
    breakdown[event.Department]++
} else if event.InnerType == "booking.cancelled" {
    breakdown[event.Department] = max(0, breakdown[event.Department]-1)
}
```

Never store employee names or IDs in `department_breakdown`. Store department label only.

### 4. Env config

```
WORKER_KINDS=projection              # enables projection worker process
PROJECTION_WORKER_CONCURRENCY=2      # number of concurrent claim loops
```

Both must be read from env via the config package. Never hardcode.

---

## Test Cases

All integration tests require `TEST_DATABASE_URL`. Use isolated schemas via
`newMigrationTestPool`.

**Test 1: `TestProjectionWorker_ConfirmedBookingUpdatesCount`**
- Seed outbox event: `booking.confirmed`, event_id = `evt_1`, department = `Engineering`
- Run worker once
- Assert: `confirmed_count = 1`, `department_breakdown["Engineering"] = 1`

**Test 2: `TestProjectionWorker_CancelledBookingDecrementsCount`**
- Seed two confirmed events, then one cancelled event
- Run worker for all three
- Assert: `confirmed_count = 1`, `cancelled_count = 1`

**Test 3: `TestProjectionWorker_CountNeverGoesBelowZero`**
- Seed one cancelled event with no prior confirmed
- Run worker
- Assert: `confirmed_count = 0` (not -1)

**Test 4: `TestProjectionWorker_IdempotentReplay`**
- Seed one `booking.confirmed` outbox event (outbox_id = 5)
- Run worker twice (simulate duplicate delivery)
- Assert: `confirmed_count = 1` (not 2)

**Test 5: `TestProjectionWorker_OlderEventDoesNotOverwriteNewer`**
- Process event with outbox_id = 10 first (confirmed_count becomes 5)
- Then replay event with outbox_id = 3
- Assert: `confirmed_count` remains 5; `last_event_offset` remains 10

**Test 6: `TestProjectionWorker_OffsetAdvancesAfterProcessing`**
- Process 3 events with outbox_ids 1, 2, 3
- Assert: `reporting_projection_offsets.last_processed_outbox_id = 3`

**Test 7: `TestProjectionWorker_UnknownInnerEventTypeIsSkipped`**
- Seed outbox event with unknown inner type `booking.unknown_v99`
- Run worker
- Assert: no error, no dead-letter, `confirmed_count` unchanged

**Test 8: `TestProjectionWorker_CrashRecovery`**
- Process events 1 and 2; simulate crash before offset advances
- Restart worker; process from last committed offset
- Assert: no double-count; final counts correct

**Test 9: `TestProjectionWorker_LagMetricRecorded`**
- Seed outbox event with `created_at = now() - 45s`
- Run worker
- Assert: `projection_worker_lag_seconds` histogram has an observation between 44 and 46

**Test 10: `TestProjectionWorker_DoesNotProcessNotificationEvents`**
- Seed outbox event with `event_type = 'booking.confirmed'` (notification kind, not projection)
- Run projection worker
- Assert: event is not claimed; `reporting_event_summary` unchanged

---

## Implementation Order

Each step is independently compilable. Do not skip steps.

1. **`projection_models.go`** — define `ProjectionEvent` struct and inner event type constants
2. **`queries_projection.go`** — `UpsertEventSummary`, `AdvanceProjectionOffset`,
   `GetProjectionOffset` SQL queries
3. **`projection_metrics.go`** — register `projection_worker_lag_seconds` histogram and
   `projection_events_processed_total` counter
4. **`projection_worker.go`** — implement worker loop using queries and metrics from steps 2–3
5. **`worker.go`** — register `projection` kind; wire to existing outbox claim loop
6. **`.env.example`** — add `PROJECTION_WORKER_CONCURRENCY=2` with comment
7. **`projection_worker_test.go`** — integration tests 1–10; all must pass before PR

---

## Non-Goals

- Do NOT use projection data to authorize bookings, check-ins, or ticket redemption.
- Do NOT store employee IDs, names, or any PII in `reporting_event_summary`.
- Do NOT implement the rebuild admin process here (belongs to PH2-43).
- Do NOT modify notification, export, or compensation worker logic.
- Do NOT create a new outbox table — use the existing `outbox_events` table.
- Do NOT dead-letter unknown inner event types — skip and log a warning only.

---

## Reviewer Checklist

- [ ] `ON CONFLICT ... DO UPDATE` uses `last_event_offset` guard — older events never overwrite newer state.
- [ ] `GREATEST(col - 1, 0)` used for all decrement operations — counts never go below zero.
- [ ] `department_breakdown` stores only department labels and counts, no employee identifiers.
- [ ] `WORKER_KINDS` and `PROJECTION_WORKER_CONCURRENCY` read from env config, not hardcoded.
- [ ] Projection worker only claims `reporting.projection.update_required.v2` events.
- [ ] Lag metric `projection_worker_lag_seconds` is recorded per event.
- [ ] All 10 test cases present and passing.
- [ ] No projection data used for booking or check-in authorization anywhere in the diff.
- [ ] Branch is `feat/ph2-42-read-model-worker` and PR targets `main`.
