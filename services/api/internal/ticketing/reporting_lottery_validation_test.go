package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLotteryRejectsFCFSEventWithoutSideEffects(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event := createLotteryValidationEvent(t, service, ctx, admin)

	assertLotteryRejectedWithoutAllocation(t, service, ctx, event.EventID, "fcfs-seed", "lottery allocation is not enabled for this event")
}

func TestRunLotteryRejectsClosedEventWithoutSideEffects(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event := createLotteryValidationEvent(t, service, ctx, admin)
	setLotteryAllocationMode(t, service, ctx, event.EventID)
	_, err := service.db.Exec(ctx, `UPDATE events SET status = $1 WHERE event_id = $2`, EventStatusClosed, event.EventID)
	require.NoError(t, err)

	assertLotteryRejectedWithoutAllocation(t, service, ctx, event.EventID, "closed-seed", "lottery can only run for published events")
}

func TestRunLotteryRejectsBeforeRegistrationCloseWithoutSideEffects(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event := createLotteryValidationEvent(t, service, ctx, admin)
	setLotteryAllocationMode(t, service, ctx, event.EventID)

	received, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "lottery-before-close-received",
	})
	require.NoError(t, err)
	assert.Equal(t, RegistrationReceived, received.Registration.Status)

	assertLotteryRejectedWithoutAllocation(t, service, ctx, event.EventID, "before-close-seed", "lottery can only run after registration window closes")
}

func TestLotteryBookingCreatesReceivedRegistrationWithoutTicket(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event := createLotteryValidationEvent(t, service, ctx, admin)
	setLotteryAllocationMode(t, service, ctx, event.EventID)

	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "lottery-received-booking",
	})
	require.NoError(t, err)
	assert.Equal(t, RegistrationReceived, booking.Registration.Status)
	assert.Nil(t, booking.Ticket)
	require.NotNil(t, event.Capacity)
	assert.Equal(t, *event.Capacity, booking.RemainingCapacity)
	assert.Equal(t, "lottery registration received", booking.Message)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, booking.Registration.RegistrationID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'booking.confirmed' AND aggregate_id = $1`, booking.Registration.RegistrationID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'booking.waitlisted' AND aggregate_id = $1`, booking.Registration.RegistrationID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'booking.received' AND aggregate_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'booking.received' AND entity_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_idempotency_results WHERE idempotency_key = $1 AND registration_status = 'received'`, "lottery-received-booking", 1)
}

func createLotteryValidationEvent(t *testing.T, service *Service, ctx context.Context, admin Actor) EventSummary {
	t.Helper()
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Guard",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	return event
}

func assertLotteryRejectedWithoutAllocation(t *testing.T, service *Service, ctx context.Context, eventID string, seed string, message string) {
	t.Helper()
	before := lotterySideEffectCounts(t, service, ctx, eventID)

	_, err := service.RunLottery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, eventID, LotteryRunRequest{Seed: seed})

	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	assert.Equal(t, message, ErrorMessage(err))
	assert.Equal(t, before, lotterySideEffectCounts(t, service, ctx, eventID))
}

func lotterySideEffectCounts(t *testing.T, service *Service, ctx context.Context, eventID string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	queries := map[string]string{
		"received":        `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'received'`,
		"confirmed":       `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`,
		"waitlisted":      `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'waitlisted'`,
		"tickets":         `SELECT count(*) FROM tickets WHERE event_id = $1`,
		"lottery_runs":    `SELECT count(*) FROM lottery_runs WHERE event_id = $1`,
		"lottery_results": `SELECT count(*) FROM lottery_results WHERE event_id = $1`,
		"lottery_audits":  `SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'lottery.completed'`,
		"ticket_audits":   `SELECT count(*) FROM audit_logs WHERE action = 'ticket.issued' AND entity_id IN (SELECT ticket_id FROM tickets WHERE event_id = $1)`,
		"lottery_outbox":  `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND (payload->'payload'->>'event_id') = $1`,
		"ticket_outbox":   `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.issued' AND (payload->'payload'->>'event_id') = $1`,
		"redeemed_outbox": `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.redeemed' AND (payload->'payload'->>'event_id') = $1`,
	}
	for key, query := range queries {
		var count int
		require.NoError(t, service.db.QueryRow(ctx, query, eventID).Scan(&count), key)
		counts[key] = count
	}
	return counts
}
