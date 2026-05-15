package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancelRegistrationPromotesWaitlistInSingleTransaction(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Promotion Transactional Test",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	confirmed, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "book-1"})
	require.NoError(t, err)
	waitlist, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "book-2"})
	require.NoError(t, err)

	cancelled, err := service.CancelRegistration(ctx, admin, event.EventID, confirmed.Registration.RegistrationID, CancelRegistrationRequest{
		IdempotencyKey: "cancel-1",
		Reason:         "speaker change",
	})
	require.NoError(t, err)
	assert.Equal(t, RegistrationCancelled, cancelled.Registration.Status)
	assert.Equal(t, 0, cancelled.RemainingCapacity)
	if cancelled.Ticket != nil {
		assert.Empty(t, cancelled.Ticket.SignedToken, "admin cancellation response should not expose reusable token")
		assert.Empty(t, cancelled.Ticket.QRPayload, "admin cancellation response should not expose reusable token")
	}

	var promotedStatus string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status FROM registrations WHERE registration_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotedStatus))
	assert.Equal(t, RegistrationConfirmed, promotedStatus)
	var promotedTicketCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotedTicketCount))
	assert.Equal(t, 1, promotedTicketCount)

	var canceledOutboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'registration.cancelled' AND aggregate_id = $1`, confirmed.Registration.RegistrationID).Scan(&canceledOutboxCount))
	assert.Equal(t, 1, canceledOutboxCount)
	var promotedOutboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'waitlist.promoted' AND aggregate_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotedOutboxCount))
	assert.Equal(t, 1, promotedOutboxCount)
}

func TestCancelRegistrationRetryIsSafeByCancelIdempotencyKey(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Idempotent Cancel Test",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	confirmed, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-book-1"})
	require.NoError(t, err)
	waitlist, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "idem-book-2"})
	require.NoError(t, err)

	first, err := service.CancelRegistration(ctx, admin, event.EventID, confirmed.Registration.RegistrationID, CancelRegistrationRequest{
		IdempotencyKey: "cancel-idem-1",
		Reason:         "full cancellation",
	})
	require.NoError(t, err)
	assert.Equal(t, RegistrationCancelled, first.Registration.Status)

	second, err := service.CancelRegistration(ctx, admin, event.EventID, confirmed.Registration.RegistrationID, CancelRegistrationRequest{
		IdempotencyKey: "cancel-idem-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "registration already cancelled", second.Message)

	var cancellationAuditCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'registration.cancelled' AND entity_id = $1`, confirmed.Registration.RegistrationID).Scan(&cancellationAuditCount))
	assert.Equal(t, 1, cancellationAuditCount)

	var promotionAuditCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'waitlist.promoted' AND entity_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotionAuditCount))
	assert.Equal(t, 1, promotionAuditCount)
	var promotionOutboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'waitlist.promoted' AND aggregate_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotionOutboxCount))
	assert.Equal(t, 1, promotionOutboxCount)
	var cancelledOutboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'registration.cancelled' AND aggregate_id = $1`, confirmed.Registration.RegistrationID).Scan(&cancelledOutboxCount))
	assert.Equal(t, 1, cancelledOutboxCount)
}

func TestPromoteWaitlistDoesNotExposeTicketTokenToAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Sanitized Promotion",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "sanitize-book-1"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "sanitize-book-2"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE events SET capacity = 2 WHERE event_id = $1`, event.EventID)
	require.NoError(t, err)

	promoted, err := service.PromoteWaitlist(ctx, admin, event.EventID)
	require.NoError(t, err)
	require.NotNil(t, promoted.Promoted, "expected promoted registration")
	require.NotNil(t, promoted.Promoted.Ticket, "expected promoted registration with ticket")
	assert.Empty(t, promoted.Promoted.Ticket.SignedToken, "admin promotion response should not expose reusable token")
	assert.Empty(t, promoted.Promoted.Ticket.QRPayload, "admin promotion response should not expose reusable token")
}
