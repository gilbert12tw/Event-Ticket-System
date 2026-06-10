package ticketing

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func upsertEventSummaryForTest(t *testing.T, service *Service, ctx context.Context, eventID string, counts eventSummaryRow, offset string) {
	t.Helper()
	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer func(tx pgx.Tx) { _ = tx.Rollback(ctx) }(tx)
	require.NoError(t, upsertEventSummary(ctx, tx, eventID, counts, offset))
	require.NoError(t, tx.Commit(ctx))
}

// PH2-45: the export aggregate columns round-trip through upsert and read.
func TestEventSummaryUpsertRoundTripsExportAggregateColumns(t *testing.T) {
	service, ctx := newWorkerTest(t)
	counts := eventSummaryRow{
		ConfirmedCount:      3,
		CancelledCount:      1,
		WaitlistCount:       2,
		EmployeeCount:       3,
		FamilyCount:         5,
		TicketCount:         3,
		CheckinCount:        2,
		DepartmentBreakdown: map[string]int{"Engineering": 3},
	}
	upsertEventSummaryForTest(t, service, ctx, "evt_rt", counts, "00000000000000000010|ob-10")

	row := readEventSummary(t, service, ctx, "evt_rt")
	assert.Equal(t, 3, row.ConfirmedCount)
	assert.Equal(t, 1, row.CancelledCount)
	assert.Equal(t, 2, row.WaitlistCount)
	assert.Equal(t, 3, row.EmployeeCount)
	assert.Equal(t, 5, row.FamilyCount)
	assert.Equal(t, 3, row.TicketCount)
	assert.Equal(t, 2, row.CheckinCount)
	assert.Equal(t, "00000000000000000010|ob-10", row.LastEventOffset)
}

// PH2-45: a stale (older-offset) upsert must not overwrite any of the export
// aggregate columns written by a newer event.
func TestEventSummaryUpsertStaleOffsetDoesNotOverwriteExportColumns(t *testing.T) {
	service, ctx := newWorkerTest(t)
	newer := eventSummaryRow{
		ConfirmedCount: 4, EmployeeCount: 4, FamilyCount: 6, TicketCount: 4, CheckinCount: 3,
		DepartmentBreakdown: map[string]int{},
	}
	upsertEventSummaryForTest(t, service, ctx, "evt_stale", newer, "00000000000000000020|ob-20")

	stale := eventSummaryRow{
		ConfirmedCount: 1, EmployeeCount: 1, FamilyCount: 1, TicketCount: 1, CheckinCount: 1,
		DepartmentBreakdown: map[string]int{},
	}
	upsertEventSummaryForTest(t, service, ctx, "evt_stale", stale, "00000000000000000005|ob-05")

	row := readEventSummary(t, service, ctx, "evt_stale")
	assert.Equal(t, 4, row.ConfirmedCount)
	assert.Equal(t, 4, row.EmployeeCount)
	assert.Equal(t, 6, row.FamilyCount)
	assert.Equal(t, 4, row.TicketCount)
	assert.Equal(t, 3, row.CheckinCount)
	assert.Equal(t, "00000000000000000020|ob-20", row.LastEventOffset)
}

// PH2-45: incremental updates that do not touch the export columns must carry
// them forward instead of zeroing rebuild-populated values.
func TestComputeNewCountsCarriesExportColumnsForward(t *testing.T) {
	current := eventSummaryRow{
		ConfirmedCount: 2, EmployeeCount: 2, FamilyCount: 3, TicketCount: 2, CheckinCount: 1,
		DepartmentBreakdown: map[string]int{},
	}
	next := computeNewCounts(current, ProjectionEvent{
		EventID:   "evt_1",
		OutboxID:  "00000000000000000030|ob-30",
		InnerType: projectionInnerTypeBookingWaitlisted,
	})
	assert.Equal(t, 2, next.EmployeeCount)
	assert.Equal(t, 3, next.FamilyCount)
	assert.Equal(t, 2, next.TicketCount)
	assert.Equal(t, 1, next.CheckinCount)
	assert.Equal(t, 1, next.WaitlistCount)
}
