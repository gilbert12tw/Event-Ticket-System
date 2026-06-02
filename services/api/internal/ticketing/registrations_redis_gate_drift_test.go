package ticketing

import (
	"context"
	"testing"

	"event-ticket-system/internal/reservation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookingWithGateGrantedHoldStillWaitlistsWhenDBCapacityFull(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	gate := &recordingReservationGate{outcome: reservation.OutcomeGranted}
	service.WithReservationGate(gate, []byte("reservation-test-secret"))
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Redis Granted Drift",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "granted-drift-1",
	})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, first.Registration.Status)

	second, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1002",
		IdempotencyKey: "granted-drift-2",
	})
	require.NoError(t, err)
	assert.Equal(t, RegistrationWaitlisted, second.Registration.Status)
	assert.Nil(t, second.Ticket)
	assert.Equal(t, 2, gate.reserveCalls)
	assert.Equal(t, 1, gate.confirmCalls)
	assert.Equal(t, 1, gate.releaseCalls, "waitlist outcome must return the Redis-granted hold")

	var confirmed int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID).Scan(&confirmed))
	assert.Equal(t, 1, confirmed)
}
