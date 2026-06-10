package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PH2-45: computeNewCounts maintains the export aggregate columns per the
// inner-event matrix (floors at 0 throughout).
func TestComputeNewCountsExportAggregateMatrix(t *testing.T) {
	base := eventSummaryRow{
		ConfirmedCount: 2, CancelledCount: 1, WaitlistCount: 1,
		EmployeeCount: 2, FamilyCount: 3, TicketCount: 2, CheckinCount: 1,
		DepartmentBreakdown: map[string]int{"Engineering": 2},
	}
	cases := []struct {
		name        string
		current     eventSummaryRow
		event       ProjectionEvent
		confirmed   int
		cancelled   int
		waitlist    int
		employee    int
		family      int
		ticket      int
		checkin     int
		engineering int
	}{
		{
			name:      "booking.confirmed increments confirmed/employee/family/ticket",
			current:   base,
			event:     ProjectionEvent{InnerType: projectionInnerTypeBookingConfirmed, Department: "Engineering", FamilyCount: 2},
			confirmed: 3, cancelled: 1, waitlist: 1, employee: 3, family: 5, ticket: 3, checkin: 1, engineering: 3,
		},
		{
			name:      "booking.cancelled decrements confirmed/employee/family, keeps ticket",
			current:   base,
			event:     ProjectionEvent{InnerType: projectionInnerTypeBookingCancelled, Department: "Engineering", FamilyCount: 2},
			confirmed: 1, cancelled: 2, waitlist: 1, employee: 1, family: 1, ticket: 2, checkin: 1, engineering: 1,
		},
		{
			name:      "booking.waitlisted leaves export columns unchanged",
			current:   base,
			event:     ProjectionEvent{InnerType: projectionInnerTypeBookingWaitlisted},
			confirmed: 2, cancelled: 1, waitlist: 2, employee: 2, family: 3, ticket: 2, checkin: 1, engineering: 2,
		},
		{
			name:      "checkin.completed increments checkin_count only",
			current:   base,
			event:     ProjectionEvent{InnerType: projectionInnerTypeCheckinCompleted},
			confirmed: 2, cancelled: 1, waitlist: 1, employee: 2, family: 3, ticket: 2, checkin: 2, engineering: 2,
		},
		{
			name:      "cancel on empty row floors counts at zero",
			current:   eventSummaryRow{DepartmentBreakdown: map[string]int{}},
			event:     ProjectionEvent{InnerType: projectionInnerTypeBookingCancelled, FamilyCount: 4},
			confirmed: 0, cancelled: 1, waitlist: 0, employee: 0, family: 0, ticket: 0, checkin: 0, engineering: 0,
		},
		{
			name:      "negative family delta in payload is ignored",
			current:   base,
			event:     ProjectionEvent{InnerType: projectionInnerTypeBookingConfirmed, FamilyCount: -3},
			confirmed: 3, cancelled: 1, waitlist: 1, employee: 3, family: 3, ticket: 3, checkin: 1, engineering: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := computeNewCounts(tc.current, tc.event)
			assert.Equal(t, tc.confirmed, next.ConfirmedCount, "confirmed_count")
			assert.Equal(t, tc.cancelled, next.CancelledCount, "cancelled_count")
			assert.Equal(t, tc.waitlist, next.WaitlistCount, "waitlist_count")
			assert.Equal(t, tc.employee, next.EmployeeCount, "employee_count")
			assert.Equal(t, tc.family, next.FamilyCount, "family_count")
			assert.Equal(t, tc.ticket, next.TicketCount, "ticket_count")
			assert.Equal(t, tc.checkin, next.CheckinCount, "checkin_count")
			assert.Equal(t, tc.engineering, next.DepartmentBreakdown["Engineering"], "department_breakdown")
		})
	}
}

// PH2-45: decodeProjectionEvent carries the optional family_count field in
// both the v1 flat payload and the v2 envelope payload.
func TestDecodeProjectionEventFamilyCount(t *testing.T) {
	createdAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

	v1 := outboxClaim{
		outboxID:      "ob-1",
		eventType:     outboxEventReportingProjectionUpdateRequiredV2,
		payloadText:   `{"aggregate_id":"evt_1","inner_event_type":"booking.confirmed","family_count":3}`,
		schemaVersion: 1,
		createdAt:     createdAt,
	}
	proj, ok := decodeProjectionEvent(v1)
	require.True(t, ok)
	assert.Equal(t, 3, proj.FamilyCount)

	v2 := outboxClaim{
		outboxID:      "ob-2",
		eventType:     outboxEventReportingProjectionUpdateRequiredV2,
		payloadText:   `{"payload":{"aggregate_id":"evt_1","trigger_event_id":"trig-1","family_count":2}}`,
		schemaVersion: 2,
		createdAt:     createdAt,
	}
	proj, ok = decodeProjectionEvent(v2)
	require.True(t, ok)
	assert.Equal(t, 2, proj.FamilyCount)

	missing := outboxClaim{
		outboxID:      "ob-3",
		eventType:     outboxEventReportingProjectionUpdateRequiredV2,
		payloadText:   `{"aggregate_id":"evt_1","inner_event_type":"booking.confirmed"}`,
		schemaVersion: 1,
		createdAt:     createdAt,
	}
	proj, ok = decodeProjectionEvent(missing)
	require.True(t, ok)
	assert.Equal(t, 0, proj.FamilyCount, "absent family_count defaults to zero delta")
}

// PH2-45: a checkin.completed projection event increments checkin_count and
// advances the watermark; replaying a stale checkin event does not
// double-count.
func TestProjectionWorker_CheckinCompletedIncrementsCheckinCount(t *testing.T) {
	service, ctx := newWorkerTest(t)
	baseTime := time.Now().UTC().Truncate(time.Microsecond).Add(-10 * time.Second)
	insertProjectionOutboxAt(t, service, ctx, "ob-chk-1", "evt_1", projectionInnerTypeCheckinCompleted, baseTime)

	processed, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 1, row.CheckinCount)
	assert.Equal(t, 0, row.ConfirmedCount)

	offset := readProjectionOffset(t, service, ctx, projectionProjectionName)
	assert.Regexp(t, `\|ob-chk-1$`, offset, "checkin event must advance the watermark")

	// Stale replay with an older offset must not double-count.
	insertProjectionOutboxAt(t, service, ctx, "ob-chk-0", "evt_1", projectionInnerTypeCheckinCompleted, baseTime.Add(-time.Minute))
	_, err = runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	row = readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 1, row.CheckinCount, "stale checkin replay must not double-count")
}
