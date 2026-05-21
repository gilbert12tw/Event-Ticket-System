package ticketing

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookingIdempotencyReplayReturnsOriginalCapacityResult(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Replay Capacity Snapshot",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "snapshot-1"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, first.Registration.Status)
	require.NotNil(t, first.Ticket)
	assert.Equal(t, 1, first.RemainingCapacity)

	second, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "snapshot-2"})
	require.NoError(t, err)
	assert.Equal(t, RegistrationConfirmed, second.Registration.Status)
	assert.Equal(t, 0, second.RemainingCapacity)

	replay, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "snapshot-1"})
	require.NoError(t, err)
	require.NotNil(t, replay.Ticket)
	assert.True(t, replay.Duplicate)
	assert.Equal(t, first.Registration.RegistrationID, replay.Registration.RegistrationID)
	assert.Equal(t, first.Ticket.TicketID, replay.Ticket.TicketID)
	assert.Equal(t, first.RemainingCapacity, replay.RemainingCapacity)
	assert.Equal(t, first.Message, replay.Message)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_idempotency_results WHERE idempotency_key = $1`, "snapshot-1", 1)
}

func TestBookingIdempotencyConcurrentSameKeyCreatesOneSideEffectSet(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Same Key Race",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	const workers = 8
	start := make(chan struct{})
	results := make(chan BookingResponse, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "same-key-race"})
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	var registrationID string
	var ticketID string
	var freshCount int
	var replayCount int
	for result := range results {
		require.Equal(t, RegistrationConfirmed, result.Registration.Status)
		require.NotNil(t, result.Ticket)
		if registrationID == "" {
			registrationID = result.Registration.RegistrationID
			ticketID = result.Ticket.TicketID
		}
		assert.Equal(t, registrationID, result.Registration.RegistrationID)
		assert.Equal(t, ticketID, result.Ticket.TicketID)
		if result.Duplicate {
			replayCount++
		} else {
			freshCount++
		}
	}
	assert.Equal(t, 1, freshCount)
	assert.Equal(t, workers-1, replayCount)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E1001'`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, registrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'booking.confirmed' AND aggregate_id = $1`, registrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'booking.confirmed' AND entity_id = $1`, registrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.issued' AND entity_id = $1`, ticketID, 1)
}

func TestBookingDifferentIdempotencyKeyForExistingBookingDoesNotCreateSideEffects(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Different Key Existing",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "existing-original"})
	require.NoError(t, err)
	require.NotNil(t, first.Ticket)

	duplicate, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "existing-new-key"})
	require.NoError(t, err)
	require.NotNil(t, duplicate.Ticket)
	assert.True(t, duplicate.Duplicate)
	assert.Equal(t, first.Registration.RegistrationID, duplicate.Registration.RegistrationID)
	assert.Equal(t, first.Ticket.TicketID, duplicate.Ticket.TicketID)

	replay, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "existing-new-key"})
	require.NoError(t, err)
	assert.True(t, replay.Duplicate)
	assert.Equal(t, duplicate.Registration.RegistrationID, replay.Registration.RegistrationID)
	assert.Equal(t, duplicate.RemainingCapacity, replay.RemainingCapacity)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E1001'`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, first.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'booking.confirmed' AND aggregate_id = $1`, first.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'booking.confirmed' AND entity_id = $1`, first.Registration.RegistrationID, 1)
}

func TestBookingIdempotencyRolledBackPartialAttemptLeavesRetryClean(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Rollback Retry",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO booking_idempotency_results
		(idempotency_key, event_id, employee_id, family_count)
		VALUES ('rolled-back-key', $1, 'E1001', 0)`, event.EventID)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(ctx))

	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "rolled-back-key"})
	require.NoError(t, err)
	require.NotNil(t, booking.Ticket)
	assert.Equal(t, RegistrationConfirmed, booking.Registration.Status)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_idempotency_results WHERE idempotency_key = $1`, "rolled-back-key", 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE idempotency_key = $1`, "rolled-back-key", 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'booking.confirmed' AND entity_id = $1`, booking.Registration.RegistrationID, 1)
}
