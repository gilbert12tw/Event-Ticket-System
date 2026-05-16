package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTicketTokenVisibleOnlyToOwningEmployee(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Token Visibility Event",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	owner := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, owner, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "ticket-vis-1"})
	require.NoError(t, err)
	owned, err := service.GetTicket(ctx, owner, booking.Ticket.TicketID)
	require.NoError(t, err)
	assert.NotEmpty(t, owned.SignedToken, "owning employee should receive token payload")
	assert.NotEmpty(t, owned.QRPayload, "owning employee should receive token payload")

	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	hrView, err := service.GetTicket(ctx, hr, booking.Ticket.TicketID)
	require.NoError(t, err)
	assert.Empty(t, hrView.SignedToken, "hr should receive sanitized ticket")
	assert.Empty(t, hrView.QRPayload, "hr should receive sanitized ticket")

	checkin := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	checkinView, err := service.GetTicket(ctx, checkin, booking.Ticket.TicketID)
	require.NoError(t, err)
	assert.Empty(t, checkinView.SignedToken, "checkin staff should receive sanitized ticket")
	assert.Empty(t, checkinView.QRPayload, "checkin staff should receive sanitized ticket")

	activityAdmin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	adminView, err := service.GetTicket(ctx, activityAdmin, booking.Ticket.TicketID)
	require.NoError(t, err)
	assert.Empty(t, adminView.SignedToken, "activity admin should receive sanitized ticket")
	assert.Empty(t, adminView.QRPayload, "activity admin should receive sanitized ticket")

	_, err = service.GetTicket(ctx, Actor{ID: "E1002", Role: RoleEmployee}, booking.Ticket.TicketID)
	require.Error(t, err, "non-owner employee should be forbidden")
	assert.Equal(t, 403, ErrorStatus(err))
}

func TestRevokeTicketWritesOutbox(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Revocation Event",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "revoke-ticket-1"})
	require.NoError(t, err)
	_, err = service.RevokeTicket(ctx, admin, booking.Ticket.TicketID, RevokeTicketRequest{Reason: "fraud detected"})
	require.NoError(t, err)

	var outboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.revoked' AND aggregate_id = $1`, booking.Ticket.TicketID).Scan(&outboxCount))
	assert.Equal(t, 1, outboxCount)
}
