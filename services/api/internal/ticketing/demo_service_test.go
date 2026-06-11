package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoCheckinStartBounds(t *testing.T) {
	loc := checkinBusinessLocation()
	cases := []struct {
		name       string
		now        time.Time
		wantToday  time.Time
		wantFuture time.Time
	}{
		{
			name:       "local morning",
			now:        time.Date(2026, 6, 8, 1, 30, 0, 0, loc),
			wantToday:  time.Date(2026, 6, 8, 0, 0, 0, 0, loc).UTC(),
			wantFuture: time.Date(2026, 6, 9, 0, 0, 0, 0, loc).UTC(),
		},
		{
			name:       "exact local midnight",
			now:        time.Date(2026, 6, 8, 0, 0, 0, 0, loc),
			wantToday:  time.Date(2026, 6, 8, 0, 0, 0, 0, loc).UTC(),
			wantFuture: time.Date(2026, 6, 9, 0, 0, 0, 0, loc).UTC(),
		},
		{
			name:       "late local evening",
			now:        time.Date(2026, 6, 8, 23, 59, 0, 0, loc),
			wantToday:  time.Date(2026, 6, 8, 0, 0, 0, 0, loc).UTC(),
			wantFuture: time.Date(2026, 6, 9, 0, 0, 0, 0, loc).UTC(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := tc.now.UTC()

			todayStarts := demoTodayCheckinStart(now)
			futureStarts := demoNextCheckinStart(todayStarts)

			assert.Equal(t, tc.wantToday, todayStarts)
			assert.Equal(t, tc.wantFuture, futureStarts)
			assert.False(t, todayStarts.After(now), "today check-in window must already be open")
			assert.True(t, now.Before(futureStarts), "future check-in window must start after now")
		})
	}
}

func TestSeedDemoDataUpsertsEmployeesIdempotently(t *testing.T) {
	service, ctx := newSeededIntegrationTest(t)

	// A second seed run must update in place, not duplicate employees.
	require.NoError(t, service.SeedDemoData(ctx))

	var total int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM employees`).Scan(&total))
	assert.Equal(t, len(DemoEmployeeIDs)+2, total, "demo employees plus E2001 and E3001")

	var site string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT site FROM employees WHERE employee_id = 'E3001'`).Scan(&site))
	assert.Equal(t, "Tainan HQ", site, "second site must survive for multi-site demos")
}

func TestSeedDemoEventTicketsCreatesTodayAndFutureEvents(t *testing.T) {
	service, ctx := newSeededIntegrationTest(t)

	seed, err := service.SeedDemoEventTickets(ctx)
	require.NoError(t, err)

	assert.Len(t, seed.TodayTicketIDs, len(DemoEmployeeIDs))
	assert.Len(t, seed.FutureTicketIDs, 2)
	require.NotEmpty(t, seed.TodayEventID)
	require.NotEmpty(t, seed.FutureEventID)
	assert.NotEqual(t, seed.TodayEventID, seed.FutureEventID)

	now := time.Now().UTC()
	var todayStarts, futureStarts time.Time
	var todayStatus string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT starts_at, status FROM events WHERE event_id = $1`, seed.TodayEventID).Scan(&todayStarts, &todayStatus))
	require.NoError(t, service.db.QueryRow(ctx, `SELECT starts_at FROM events WHERE event_id = $1`, seed.FutureEventID).Scan(&futureStarts))
	assert.Equal(t, string(EventStatusPublished), todayStatus)
	assert.True(t, todayStarts.Before(now), "today event must already be open for check-in")
	assert.True(t, futureStarts.After(now), "future event must start after now")

	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets t JOIN registrations r ON r.registration_id = t.registration_id WHERE r.event_id = $1`, seed.TodayEventID, len(DemoEmployeeIDs))
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets t JOIN registrations r ON r.registration_id = t.registration_id WHERE r.event_id = $1`, seed.FutureEventID, 2)
}

func TestSeedDemoEventTicketsRequiresSeededEmployees(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.SeedDemoEventTickets(ctx)
	require.Error(t, err, "booking must fail when demo employees are missing")
}
