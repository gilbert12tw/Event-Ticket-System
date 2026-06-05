package ticketing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"event-ticket-system/internal/postgres"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func newRebuildPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, cleanup := newTicketingTestPool(t, ctx, databaseURL)
	t.Cleanup(cleanup)
	require.NoError(t, postgres.Migrate(ctx, pool))
	return pool, ctx
}

func seedRebuildEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string) {
	t.Helper()
	_, err := pool.Exec(ctx, `INSERT INTO events
		(event_id, title, starts_at, registration_start, registration_close, capacity_type, capacity, status, created_by)
		VALUES ($1, $2, now() + interval '7 days', now(), now() + interval '1 day', 'limited', 100, 'published', 'admin-1')`,
		eventID, "Event "+eventID)
	require.NoError(t, err)
}

// seedRegistrations seeds n registrations of one status for an event, each with
// its own employee in the given department (avoids the active-employee unique
// index and lets department_breakdown be computed from confirmed rows).
func seedRegistrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID, status, department string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		empID := fmt.Sprintf("%s-%s-%s-%d", eventID, status, department, i)
		_, err := pool.Exec(ctx, `INSERT INTO employees
			(employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1, $2, $3, 'Taipei HQ', 5, 'active')`, empID, "Name "+empID, department)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO registrations
			(registration_id, event_id, employee_id, status, idempotency_key)
			VALUES ($1, $2, $3, $4, $5)`, "reg-"+empID, eventID, empID, status, "idem-"+empID)
		require.NoError(t, err)
	}
}

func seedOutbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, outboxID string) {
	t.Helper()
	_, err := pool.Exec(ctx, `INSERT INTO outbox_events (outbox_id, aggregate_id, event_type, payload)
		VALUES ($1, $1 || '-agg', 'test.event', '{}'::jsonb)`, outboxID)
	require.NoError(t, err)
}

func summaryRowCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reporting_event_summary WHERE event_id = $1`, eventID).Scan(&count))
	return count
}

func TestRebuildProjection_FreshBuild(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evtA")
	seedRebuildEvent(t, ctx, pool, "evtB")
	seedRegistrations(t, ctx, pool, "evtA", "confirmed", "Engineering", 5)
	seedRegistrations(t, ctx, pool, "evtA", "cancelled", "Engineering", 2)
	seedRegistrations(t, ctx, pool, "evtB", "waitlisted", "Sales", 3)

	res, err := RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, res.RowsInserted)

	a, err := getEventSummaryRow(ctx, pool, "evtA")
	require.NoError(t, err)
	require.Equal(t, 5, a.ConfirmedCount)
	require.Equal(t, 2, a.CancelledCount)
	require.Equal(t, 0, a.WaitlistCount)

	b, err := getEventSummaryRow(ctx, pool, "evtB")
	require.NoError(t, err)
	require.Equal(t, 0, b.ConfirmedCount)
	require.Equal(t, 3, b.WaitlistCount)
}

func TestRebuildProjection_Idempotent(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 4)
	seedRegistrations(t, ctx, pool, "evt", "cancelled", "Engineering", 1)

	_, err := RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)
	first, err := getEventSummaryRow(ctx, pool, "evt")
	require.NoError(t, err)

	_, err = RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)
	second, err := getEventSummaryRow(ctx, pool, "evt")
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, 4, second.ConfirmedCount)
}

func TestRebuildProjection_EmptyProjectionBeforeRebuild(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 2)
	require.Equal(t, 0, summaryRowCount(t, ctx, pool, "evt"))

	_, err := RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)

	row, err := getEventSummaryRow(ctx, pool, "evt")
	require.NoError(t, err)
	require.Equal(t, 2, row.ConfirmedCount)
}

func TestRebuildProjection_TruncatesStaleRows(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 1)
	_, err := pool.Exec(ctx, `INSERT INTO reporting_event_summary (event_id, confirmed_count)
		VALUES ('old_event', 9)`)
	require.NoError(t, err)

	_, err = RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)

	require.Equal(t, 0, summaryRowCount(t, ctx, pool, "old_event"))
	require.Equal(t, 1, summaryRowCount(t, ctx, pool, "evt"))
}

func TestRebuildProjection_OffsetResetToMaxOutbox(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedOutbox(t, ctx, pool, "ob-001")
	seedOutbox(t, ctx, pool, "ob-002")
	seedOutbox(t, ctx, pool, "ob-042")

	res, err := RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)
	require.Equal(t, "ob-042", res.OffsetResetTo)

	offset, err := getProjectionOffset(ctx, pool, projectionProjectionName)
	require.NoError(t, err)
	require.Equal(t, "ob-042", offset)
}

func TestRebuildProjection_OffsetResetToEmptyWhenNoOutbox(t *testing.T) {
	pool, ctx := newRebuildPool(t)

	res, err := RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)
	require.Equal(t, "", res.OffsetResetTo)

	offset, err := getProjectionOffset(ctx, pool, projectionProjectionName)
	require.NoError(t, err)
	require.Equal(t, "", offset)
}

func TestRebuildProjection_ValidationPassesOnCorrectData(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 3)

	res, err := RebuildProjection(ctx, pool, RebuildOptions{SampleValidate: true})
	require.NoError(t, err)
	require.True(t, res.Validated)
}

func TestRebuildProjection_ValidationFailsAndRollsBack(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 2)
	_, err := pool.Exec(ctx, `INSERT INTO reporting_event_summary (event_id, confirmed_count)
		VALUES ('sentinel', 7)`)
	require.NoError(t, err)

	_, err = rebuildProjection(ctx, pool, RebuildOptions{SampleValidate: true},
		func(context.Context, pgx.Tx) error { return fmt.Errorf("forced mismatch") })
	require.Error(t, err)

	// Truncate + insert rolled back: pre-rebuild sentinel row is intact.
	var stored int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT confirmed_count FROM reporting_event_summary WHERE event_id = 'sentinel'`).Scan(&stored))
	require.Equal(t, 7, stored)
	require.Equal(t, 0, summaryRowCount(t, ctx, pool, "evt"))
}

func TestRebuildProjection_DryRunDoesNotWrite(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 3)
	seedOutbox(t, ctx, pool, "ob-100")
	_, err := pool.Exec(ctx,
		`UPDATE reporting_projection_offsets SET last_processed_outbox_id = 'baseline' WHERE projection_name = $1`,
		projectionProjectionName)
	require.NoError(t, err)

	res, err := RebuildProjection(ctx, pool, RebuildOptions{DryRun: true})
	require.NoError(t, err)
	require.True(t, res.DryRun)
	require.Equal(t, 1, res.RowsInserted)

	require.Equal(t, 0, summaryRowCount(t, ctx, pool, "evt"))
	offset, err := getProjectionOffset(ctx, pool, projectionProjectionName)
	require.NoError(t, err)
	require.Equal(t, "baseline", offset)
}

func TestRebuildProjection_DepartmentBreakdownCorrect(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 2)
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "HR", 1)

	_, err := RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)

	row, err := getEventSummaryRow(ctx, pool, "evt")
	require.NoError(t, err)
	require.Equal(t, map[string]int{"Engineering": 2, "HR": 1}, row.DepartmentBreakdown)
}

func TestRebuildProjection_CountsNeverNegative(t *testing.T) {
	pool, ctx := newRebuildPool(t)
	seedRebuildEvent(t, ctx, pool, "evt")
	seedRegistrations(t, ctx, pool, "evt", "confirmed", "Engineering", 1)
	seedRegistrations(t, ctx, pool, "evt", "cancelled", "Engineering", 3)

	_, err := RebuildProjection(ctx, pool, RebuildOptions{})
	require.NoError(t, err)

	row, err := getEventSummaryRow(ctx, pool, "evt")
	require.NoError(t, err)
	require.GreaterOrEqual(t, row.ConfirmedCount, 0)
	require.GreaterOrEqual(t, row.CancelledCount, 0)
	require.GreaterOrEqual(t, row.WaitlistCount, 0)
	require.Equal(t, 1, row.ConfirmedCount)
	require.Equal(t, 3, row.CancelledCount)
}
