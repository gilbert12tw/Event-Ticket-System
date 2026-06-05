package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// systemAdmin is the actor used to drive rebuild in tests (AC-7).
var systemAdmin = Actor{ID: "rebuild-test", Role: RoleSystemAdmin}

// seedRebuildEvent inserts a minimal published, limited-capacity event so that
// registrations may reference it (registrations.event_id has an FK to events).
func seedRebuildEvent(t *testing.T, s *Service, ctx context.Context, eventID string) {
	t.Helper()
	_, err := s.db.Exec(ctx, `
		INSERT INTO events (event_id, title, starts_at, registration_start, registration_close,
		                    capacity_type, capacity, status, created_by)
		VALUES ($1, $1, now()+interval '7 days', now()-interval '1 day', now()+interval '6 days',
		        'limited', 1000, 'published', 'admin-1')
		ON CONFLICT (event_id) DO NOTHING`, eventID)
	require.NoError(t, err)
}

// seedRebuildEmployee inserts an employee with a given department label.
func seedRebuildEmployee(t *testing.T, s *Service, ctx context.Context, employeeID, department string) {
	t.Helper()
	_, err := s.db.Exec(ctx, `
		INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
		VALUES ($1, $1, $2, 'Taipei HQ', 5, 'active')
		ON CONFLICT (employee_id) DO UPDATE SET department = EXCLUDED.department`, employeeID, department)
	require.NoError(t, err)
}

// seedRegistration inserts one registration row with the given status. A unique
// idempotency_key is derived from the registration id.
func seedRegistration(t *testing.T, s *Service, ctx context.Context, regID, eventID, employeeID, status string) {
	t.Helper()
	_, err := s.db.Exec(ctx, `
		INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ($1, $2, $3, $4, $1)`, regID, eventID, employeeID, status)
	require.NoError(t, err)
}

// seedRegN inserts n registrations of `status` into eventID, each under its own
// freshly-created employee in `department`. A distinct employee per row keeps
// the registrations_unique_active_employee partial index (one non-cancelled
// registration per event+employee) satisfied.
func seedRegN(t *testing.T, s *Service, ctx context.Context, eventID, department, status string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		emp := fmt.Sprintf("emp_%s_%s_%s_%d", eventID, department, status, i)
		seedRebuildEmployee(t, s, ctx, emp, department)
		seedRegistration(t, s, ctx, fmt.Sprintf("reg_%s_%s_%s_%d", eventID, department, status, i),
			eventID, emp, status)
	}
}

func readSummary(t *testing.T, s *Service, ctx context.Context, eventID string) eventSummaryRow {
	t.Helper()
	row, err := getEventSummaryRow(ctx, s.db, eventID)
	require.NoError(t, err)
	return row
}

func readOffset(t *testing.T, s *Service, ctx context.Context) string {
	t.Helper()
	offset, err := getProjectionOffset(ctx, s.db, projectionProjectionName)
	require.NoError(t, err)
	return offset
}

func summaryRowExists(t *testing.T, s *Service, ctx context.Context, eventID string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM reporting_event_summary WHERE event_id = $1)`, eventID).Scan(&exists))
	return exists
}

// Test 1 + 3: fresh rebuild from empty projection populates counts from OLTP.
func TestRebuildProjection_FreshBuild(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRebuildEvent(t, service, ctx, "evtB")
	seedRegN(t, service, ctx, "evtA", "Engineering", "confirmed", 5)
	seedRegN(t, service, ctx, "evtA", "Engineering", "cancelled", 2)
	seedRegN(t, service, ctx, "evtB", "Engineering", "waitlisted", 3)

	result, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, result.RowsInserted)

	a := readSummary(t, service, ctx, "evtA")
	assert.Equal(t, 5, a.ConfirmedCount)
	assert.Equal(t, 2, a.CancelledCount)
	assert.Equal(t, 0, a.WaitlistCount)
	b := readSummary(t, service, ctx, "evtB")
	assert.Equal(t, 0, b.ConfirmedCount)
	assert.Equal(t, 3, b.WaitlistCount)
}

// Test 2: running twice yields identical counts (idempotent).
func TestRebuildProjection_Idempotent(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRegN(t, service, ctx, "evtA", "Engineering", "confirmed", 4)

	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	first := readSummary(t, service, ctx, "evtA")

	_, err = service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	second := readSummary(t, service, ctx, "evtA")

	assert.Equal(t, first.ConfirmedCount, second.ConfirmedCount)
	assert.Equal(t, first.CancelledCount, second.CancelledCount)
	assert.Equal(t, first.WaitlistCount, second.WaitlistCount)
	assert.Equal(t, first.DepartmentBreakdown, second.DepartmentBreakdown)
}

// Test 4: a stale projection row with no OLTP registrations is removed by the
// truncate-and-reinsert rebuild.
func TestRebuildProjection_TruncatesStaleRows(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.db.Exec(ctx, `
		INSERT INTO reporting_event_summary (event_id, confirmed_count) VALUES ('old_event', 9)`)
	require.NoError(t, err)

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRegN(t, service, ctx, "evtA", "Engineering", "confirmed", 1)

	_, err = service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	assert.False(t, summaryRowExists(t, service, ctx, "old_event"))
	assert.True(t, summaryRowExists(t, service, ctx, "evtA"))
}

// Test 5: offset is reset to the lexicographic max outbox_id.
func TestRebuildProjection_OffsetResetToMaxOutbox(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	for _, id := range []string{"out_a", "out_c", "out_b"} {
		_, err := service.db.Exec(ctx, `
			INSERT INTO outbox_events (outbox_id, aggregate_id, event_type, payload)
			VALUES ($1, 'evtA', 'booking.confirmed', '{}'::jsonb)`, id)
		require.NoError(t, err)
	}

	result, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	assert.Equal(t, "out_c", result.OffsetResetTo)
	assert.Equal(t, "out_c", readOffset(t, service, ctx))
}

// Test 6: offset resets to ” when the outbox is empty (TEXT equivalent of 0).
func TestRebuildProjection_OffsetResetToZeroWhenNoOutbox(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	result, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	assert.Equal(t, "", result.OffsetResetTo)
	assert.Equal(t, "", readOffset(t, service, ctx))
}

// Test 7: validation passes on correct data.
func TestRebuildProjection_ValidationPassesOnCorrectData(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRegN(t, service, ctx, "evtA", "Engineering", "confirmed", 3)

	result, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{SampleValidate: true})
	require.NoError(t, err)
	assert.True(t, result.Validated)
	assert.Equal(t, 3, readSummary(t, service, ctx, "evtA").ConfirmedCount)
}

// Test 8: a validation mismatch rolls back, leaving the pre-rebuild state.
func TestRebuildProjection_ValidationFailsAndRollsBack(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	// Pre-existing projection state that must survive the failed rebuild.
	_, err := service.db.Exec(ctx, `
		INSERT INTO reporting_event_summary (event_id, confirmed_count) VALUES ('preexisting', 7)`)
	require.NoError(t, err)

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRegN(t, service, ctx, "evtA", "Engineering", "confirmed", 2)

	// Force the OLTP side of validation to disagree with the inserted row.
	service.rebuildConfirmedCounter = func(context.Context, pgx.Tx, string) (int, error) {
		return 999, nil
	}

	_, err = service.RebuildProjection(ctx, systemAdmin, RebuildOptions{SampleValidate: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation mismatch")

	// Rollback restored the previous state: stale row back, rebuilt row absent.
	assert.True(t, summaryRowExists(t, service, ctx, "preexisting"))
	assert.False(t, summaryRowExists(t, service, ctx, "evtA"))
}

// Test 9: dry-run writes nothing.
func TestRebuildProjection_DryRunDoesNotWrite(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRegN(t, service, ctx, "evtA", "Engineering", "confirmed", 2)

	result, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{DryRun: true})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.RowsInserted) // would-insert count

	assert.False(t, summaryRowExists(t, service, ctx, "evtA"))
	assert.Equal(t, "", readOffset(t, service, ctx)) // offset untouched (migration default '')
}

// Test 10: department_breakdown is confirmed-only, by label, with no PII.
func TestRebuildProjection_DepartmentBreakdownCorrect(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEmployee(t, service, ctx, "ENG1", "Engineering")
	seedRebuildEmployee(t, service, ctx, "ENG2", "Engineering")
	seedRebuildEmployee(t, service, ctx, "HR1", "HR")
	seedRebuildEvent(t, service, ctx, "evtA")
	seedRegistration(t, service, ctx, "r1", "evtA", "ENG1", "confirmed")
	seedRegistration(t, service, ctx, "r2", "evtA", "ENG2", "confirmed")
	seedRegistration(t, service, ctx, "r3", "evtA", "HR1", "confirmed")

	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	row := readSummary(t, service, ctx, "evtA")
	assert.Equal(t, map[string]int{"Engineering": 2, "HR": 1}, row.DepartmentBreakdown)

	raw, err := json.Marshal(row.DepartmentBreakdown)
	require.NoError(t, err)
	for _, leaked := range []string{"ENG1", "ENG2", "HR1"} {
		assert.NotContains(t, string(raw), leaked)
	}
}

// Test 11: counts are never negative even with anomalous data.
func TestRebuildProjection_CountsNeverNegative(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRegN(t, service, ctx, "evtA", "Engineering", "confirmed", 1)
	seedRegN(t, service, ctx, "evtA", "Engineering", "cancelled", 4)

	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	row := readSummary(t, service, ctx, "evtA")
	assert.GreaterOrEqual(t, row.ConfirmedCount, 0)
	assert.GreaterOrEqual(t, row.CancelledCount, 0)
	assert.GreaterOrEqual(t, row.WaitlistCount, 0)
}

// AC-7: a non-admin actor is rejected before any work happens.
func TestRebuildProjection_RequiresSystemAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.RebuildProjection(ctx, Actor{ID: "E1", Role: RoleEmployee}, RebuildOptions{})
	require.Error(t, err)
	var appErr AppError
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, 403, appErr.Status)
}
