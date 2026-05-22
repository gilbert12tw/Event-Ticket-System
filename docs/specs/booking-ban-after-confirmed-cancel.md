# AGENT.md: Cancellation Penalty — Booking Ban After Confirmed-Cancel

## Feature Summary

Once an employee cancels a **confirmed** booking, that `(event_id, employee_id)` pair is
permanently blocked from re-booking the same event. The block is per-event (not global) and
must be lifted by an admin before the employee can book again.

Waitlist cancellations do NOT trigger a ban. Cancellations that pre-date this feature (Phase 1
data) are NOT retroactively penalised.

---

## Acceptance Criteria

| # | Criterion |
|---|---|
| AC-1 | Employee cancels a confirmed booking → a ban record is created for `(event_id, employee_id)` |
| AC-2 | Employee attempts to re-book the banned event → HTTP 422 with `"error_code": "BOOKING_BANNED"` |
| AC-3 | Waitlist cancellation → no ban record created |
| AC-4 | Admin calls lift-ban endpoint → ban record removed; employee can re-book |
| AC-5 | Audit log entries on ban creation, ban lift |
| AC-6 | Phase 1 data (pre-existing confirmed-then-cancelled rows) → no retroactive bans |
| AC-7 | Idempotent ban creation: cancelling the same already-banned registration a second time (idempotent cancel) does not create a duplicate ban row |

---

## DB Changes

### New table: `booking_bans`

Add to `SchemaStatements` in `services/api/internal/postgres/migrate.go`:

```sql
CREATE TABLE IF NOT EXISTS booking_bans (
    ban_id          TEXT        NOT NULL PRIMARY KEY,
    event_id        TEXT        NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    employee_id     TEXT        NOT NULL REFERENCES employees(employee_id) ON DELETE CASCADE,
    registration_id TEXT        NOT NULL REFERENCES registrations(registration_id) ON DELETE CASCADE,
    reason          TEXT        NOT NULL DEFAULT '',
    banned_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    lifted_at       TIMESTAMPTZ,
    lifted_by       TEXT,
    CONSTRAINT booking_bans_unique_active UNIQUE (event_id, employee_id)
);

CREATE INDEX IF NOT EXISTS idx_booking_bans_employee
    ON booking_bans (employee_id);
```

**Design notes:**
- `UNIQUE (event_id, employee_id)` enforces at most one active ban per pair — the DB is the
  final guarantee, not application logic.
- `lifted_at` / `lifted_by` preserve history for audit rather than deleting the row.
  A ban is **active** when `lifted_at IS NULL`.
- `registration_id` links back to the originating cancellation for auditability.
- No `CASCADE` from employees — employees should not be silently deleted in Phase 1.
  The FK prevents orphaned bans.

### No OLTP table is altered

No `ALTER TABLE` on any Phase 1 table. The new table stands alone.

---

## Affected Files

| File | Change |
|---|---|
| `services/api/internal/postgres/migrate.go` | Append `booking_bans` DDL + index to `SchemaStatements` |
| `services/api/internal/postgres/migrate_test.go` | Assert `booking_bans` exists after migration; assert `UNIQUE (event_id, employee_id)` is in schema; add PII-column absence test for `booking_bans`; add `dropSchema` entry |
| `services/api/internal/ticketing/errors.go` | Add `AppError.Code string` field + `bookedBanned()` constructor returning 422 + `"BOOKING_BANNED"` |
| `services/api/internal/httpapi/response.go` | Update `envelope` struct to include `"error_code"` field; propagate code from `AppError` in `writeError` / `writeServiceError` |
| `services/api/internal/ticketing/registration_models.go` | Add `BookingBan` struct |
| `services/api/internal/ticketing/registrations_service.go` | In `Book()`: after idempotency check, before eligibility check, call `checkBookingBanTx(ctx, tx, eventID, employeeID)` |
| `services/api/internal/ticketing/registrations_admin_service.go` | In `CancelRegistration()`: after status update, when `wasConfirmed`, call `createBookingBanTx(ctx, tx, ...)` |
| `services/api/internal/ticketing/booking_ban_service.go` | **New file.** Contains `checkBookingBanTx`, `createBookingBanTx`, `LiftBookingBan` |
| `services/api/internal/httpapi/handlers_registrations.go` | Add `DELETE /events/{event_id}/bans/{employee_id}` handler calling `LiftBookingBan` |
| `services/api/internal/httpapi/routes.go` | Register the new lift-ban route |
| `services/api/internal/ticketing/booking_ban_service_test.go` | **New file.** Integration tests (see Test Cases below) |

### New file size budget

- `booking_ban_service.go`: target ≤ 80 lines (3 functions)
- `booking_ban_service_test.go`: target ≤ 200 lines

---

## Detailed Logic

### 1. Ban creation (inside `CancelRegistration`)

Location: `registrations_admin_service.go`, after line 113 (`reg.Status = RegistrationCancelled`),
inside the same transaction, only when `wasConfirmed == true`:

```
banID  ← newID("ban")
INSERT INTO booking_bans (ban_id, event_id, employee_id, registration_id, reason, banned_at)
VALUES (...)
ON CONFLICT (event_id, employee_id) DO NOTHING  -- idempotent: already banned
```

Insert audit log: action `"booking.ban_created"`, entity_type `"booking_ban"`, entity_id `ban_id`.

### 2. Ban check (inside `Book`)

Location: `registrations_service.go`, after the idempotency-key check (line ~48), before
eligibility evaluation:

```
SELECT ban_id FROM booking_bans
WHERE event_id = $1 AND employee_id = $2 AND lifted_at IS NULL
```

If a row is found → return `bookingBanned("you are banned from booking this event")`.

**Position rationale:** must be inside the transaction so that it sees the same snapshot as the
capacity check. Must come before the `UNIQUE (event_id, employee_id)` duplicate-registration
check — banning is a stronger gate than duplicate detection.

### 3. Ban lift (`LiftBookingBan`)

```
UPDATE booking_bans SET lifted_at = now(), lifted_by = $actor_id
WHERE event_id = $1 AND employee_id = $2 AND lifted_at IS NULL
```

Role required: `activity_admin` or `system_admin`.

Insert audit log: action `"booking.ban_lifted"`, entity_type `"booking_ban"`.

Return 404 if no active ban found.

### 4. Error code surface

`AppError` must carry a `Code string` field. The HTTP layer must emit:

```json
{
  "success": false,
  "error": "you are banned from booking this event",
  "error_code": "BOOKING_BANNED",
  "data": null
}
```

Status: **422 Unprocessable Entity** (the request is well-formed but violates a business rule,
distinct from 409 Conflict used for window-closed and capacity errors).

Add `func bookingBanned(message string) AppError` to `errors.go`:

```go
func bookingBanned(message string) AppError {
    return AppError{Status: 422, Code: "BOOKING_BANNED", Message: message}
}
```

---

## Edge Cases

| Edge Case | Expected Behaviour |
|---|---|
| Employee cancels a **waitlisted** booking | No ban. `wasConfirmed = false`; skip ban insertion. |
| Admin cancels a **waitlisted** booking | No ban. Same guard. |
| Employee who is banned tries to join waitlist | Same `Book()` path; ban check fires before capacity check; 422 returned. Bans block both confirmed seats and waitlist entries for the same event. |
| Idempotent cancel (same `cancel_idempotency_key`) on already-cancelled registration | Early-return path (line ~93) exits before ban insertion. No duplicate ban. |
| Registration row deleted (cascades) | `ON DELETE CASCADE` on `booking_bans.registration_id` removes the ban automatically. This is intentional — if the registration disappears, the ban anchor is gone. Document this in service comment. |
| Admin lifts a ban that does not exist | Return 404. |
| Admin calls lift-ban twice | Second call: no row with `lifted_at IS NULL` found → 404. |
| Phase 1 data: employee has an old confirmed-then-cancelled registration | The `booking_bans` table is empty on migration. No retroactive ban. The new code path only fires when `CancelRegistration` executes post-deploy. |
| `Book()` is called concurrently for the same `(event_id, employee_id)` | `SELECT … FOR UPDATE` via `lockEventWithRule` serialises within the event. Ban check inside the same tx sees consistent state. |
| Ban exists, employee changes department, event eligibility rule updated | Ban is not affected. Eligibility is checked before the ban check; if the employee is now ineligible for a different reason, they see that error first. If eligible but banned, they see `BOOKING_BANNED`. |

---

## Test Cases

### Unit / Static

1. `TestSchemaIncludesBookingBans` — asserts `SchemaStatements` contains `booking_bans`,
   `UNIQUE (event_id, employee_id)`, and `idx_booking_bans_employee`.

### Integration (`TEST_DATABASE_URL` required)

All tests use isolated schemas via `newMigrationTestPool`.

2. `TestBookingBanCreatedAfterConfirmedCancellation`
   - Seed event (limited, 1 seat), employee A.
   - Book → confirmed.
   - Cancel (employee or admin).
   - Assert: `SELECT COUNT(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL` = 1.

3. `TestBookingBanNotCreatedAfterWaitlistCancellation`
   - Seed event (limited, 1 seat), employee A (books and fills capacity), employee B (waitlisted).
   - Cancel B's waitlisted registration.
   - Assert: no ban row for B.

4. `TestBannedEmployeeCannotRebook`
   - Create ban via confirmed-cancel flow (as in test 2).
   - Call `Book()` with a new idempotency key.
   - Assert: error is `AppError{Status: 422, Code: "BOOKING_BANNED"}`.

5. `TestBannedEmployeeCannotJoinWaitlist`
   - Same setup; ensure capacity is 0 (another confirmed booking fills it).
   - Call `Book()`.
   - Assert: 422 `BOOKING_BANNED` (not 409 waitlisted or 403 capacity).

6. `TestLiftBanAllowsRebook`
   - Create ban (test 2 flow).
   - Call `LiftBookingBan(ctx, activityAdmin, eventID, employeeID)`.
   - Assert: `lifted_at IS NOT NULL`.
   - Call `Book()` again → succeeds (confirmed or waitlisted depending on capacity).

7. `TestLiftBanNotFoundReturns404`
   - Call `LiftBookingBan` with an `(event_id, employee_id)` pair that has no active ban.
   - Assert: 404.

8. `TestBanCreationIdempotent`
   - Cancel confirmed registration (ban created).
   - Call `CancelRegistration` again with the same idempotency key.
   - Assert: still exactly 1 ban row (idempotent cancel path, `ON CONFLICT DO NOTHING`).

9. `TestPhase1DataNotRetroactivelyBanned`
   - Directly INSERT a registration with `status = 'cancelled'` and create no corresponding
     ban row (simulating Phase 1 data).
   - Call `Book()` for the same `(event_id, employee_id)`.
   - Assert: booking succeeds or fails for the expected reason (capacity / window), not 422.

10. `TestAuditLogWrittenOnBanAndLift`
    - After confirmed-cancel, assert `audit_logs` contains a row with
      `action = 'booking.ban_created'`.
    - After lift, assert `action = 'booking.ban_lifted'`.

---

## Non-Goals

- Do **not** add a "view my bans" endpoint for employees in this task.
- Do **not** send a notification email when a ban is created (may be added later).
- Do **not** display bans in the employee event list UI (separate frontend task).
- Do **not** retroactively scan Phase 1 data to create bans.
- Do **not** add a ban expiry timer; bans are permanent until admin-lifted.

---

## Implementation Order

1. **`migrate.go`** — add `booking_bans` DDL + index.
2. **`migrate_test.go`** — static schema assertion + `dropSchema` update.
3. **`errors.go`** — add `Code` field to `AppError`; add `bookingBanned()`.
4. **`response.go`** — propagate `error_code` in JSON envelope.
5. **`registration_models.go`** — add `BookingBan` struct.
6. **`booking_ban_service.go`** — `checkBookingBanTx`, `createBookingBanTx`, `LiftBookingBan`.
7. **`registrations_service.go`** — call `checkBookingBanTx` inside `Book()`.
8. **`registrations_admin_service.go`** — call `createBookingBanTx` inside `CancelRegistration()`.
9. **`handlers_registrations.go` + `routes.go`** — `DELETE /events/{id}/bans/{employee_id}`.
10. **`booking_ban_service_test.go`** — integration tests 2–10.
11. **`migrate_test.go`** — integration tests for `booking_bans` table (test 1).

Each step is independently compilable and committable. Steps 1–5 carry zero behaviour change.

---

## Reviewer Checklist

- [ ] Ban is only created when `wasConfirmed == true` (not waitlisted).
- [ ] Ban check is inside the same transaction as the booking attempt.
- [ ] `ON CONFLICT DO NOTHING` prevents duplicate bans on idempotent cancel.
- [ ] `error_code: "BOOKING_BANNED"` present in 422 response body.
- [ ] Audit log entries for both `booking.ban_created` and `booking.ban_lifted`.
- [ ] `LiftBookingBan` requires `activity_admin` or `system_admin` role.
- [ ] Phase 1 data retroactive ban test passes.
- [ ] No OLTP table altered by the migration.
- [ ] All 10 test cases covered.
