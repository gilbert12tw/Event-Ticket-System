package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceRejectsCheckinBeforeEventStart(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	service.WithClock(func() time.Time { return now })
	require.NoError(t, service.SeedDemoData(ctx))

	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:             "Future Check-in Rejected",
		StartsAt:          now.Add(24 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(time.Hour),
		Capacity:          1,
		Status:            EventStatusPublished,
		Rule:              engineeringRule(5),
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "future-checkin-booking"})
	require.NoError(t, err)
	require.NotNil(t, booking.Ticket)

	rejected, err := service.CheckIn(ctx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{
		SignedToken: booking.Ticket.SignedToken,
		EventID:     event.EventID,
		DeviceID:    "gate-future",
	})

	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	assert.Equal(t, "rejected", rejected.Status)
	assert.Equal(t, checkinNotStartedReason, rejected.ReasonCode)
	assert.Equal(t, checkinNotStartedReason, rejected.ConflictReason)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, booking.Ticket.TicketID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_rejections WHERE ticket_id = $1 AND reason = 'event_not_started'`, booking.Ticket.TicketID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE ticket_id = $1 AND status = 'active'`, booking.Ticket.TicketID, 1)
}

func TestServiceAcceptsCheckinAfterEventStartAcrossLocalMidnight(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	bookingNow := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	eventStarts := time.Date(2026, 6, 4, 15, 0, 0, 0, time.UTC)
	service.WithClock(func() time.Time { return bookingNow })
	require.NoError(t, service.SeedDemoData(ctx))

	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:             "Late Night Check-in",
		StartsAt:          eventStarts,
		RegistrationStart: bookingNow.Add(-time.Hour),
		RegistrationClose: bookingNow.Add(time.Hour),
		Capacity:          1,
		Status:            EventStatusPublished,
		Rule:              engineeringRule(5),
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "late-night-checkin-booking"})
	require.NoError(t, err)
	require.NotNil(t, booking.Ticket)
	service.WithClock(func() time.Time {
		return time.Date(2026, 6, 4, 17, 30, 0, 0, time.UTC)
	})

	accepted, err := service.CheckIn(ctx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{
		SignedToken: booking.Ticket.SignedToken,
		EventID:     event.EventID,
		DeviceID:    "gate-late",
	})

	require.NoError(t, err)
	assert.Equal(t, "accepted", accepted.Status)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, booking.Ticket.TicketID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_rejections WHERE ticket_id = $1`, booking.Ticket.TicketID, 0)
}

func TestSyncOfflineCheckinsRejectsScanBeforeEventStart(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	service.WithClock(func() time.Time { return now })
	require.NoError(t, service.SeedDemoData(ctx))

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Offline Future Rejected", "E1001", "offline-future-checkin")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-future")
	require.NoError(t, err)

	response, err := service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-future",
		PackageSignature: pkg.PackageSignature,
		Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: event.StartsAt.AddDate(0, 0, -1)}},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, response.Accepted)
	assert.Equal(t, 1, response.Conflict)
	require.Len(t, response.Results, 1)
	assert.Equal(t, offlineScanStatusConflict, response.Results[0].Status)
	assert.Equal(t, checkinNotStartedReason, response.Results[0].ConflictReason)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 0)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'conflict' AND conflict_reason = 'event_not_started'`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 1)
}

func checkinReadyEventRequest(service *Service, title string, capacity int) CreateEventRequest {
	now := service.now()
	return CreateEventRequest{
		Title:             title,
		Location:          "Taipei HQ",
		StartsAt:          now,
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(time.Hour),
		Capacity:          capacity,
		Status:            EventStatusPublished,
		Rule:              engineeringRule(5),
	}
}
