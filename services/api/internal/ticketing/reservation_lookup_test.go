package ticketing

import (
	"context"
	"testing"

	"event-ticket-system/internal/reservation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The compensation worker depends on BookingByHash returning a
// well-defined status for each row state the gate can leave behind. These
// tests pin those states down so the SQL + status mapping does not silently
// drift away from the four cases the compensator branches on:
//
//   missing                    -> Found=false, Completed=false, Confirmed=false (release)
//   in-flight (no completed_at)-> Found=true,  Completed=false                  (release: row exists but never finished)
//   completed waitlisted/cancel-> Found=true,  Completed=true,  Confirmed=false (release: no capacity consumed)
//   completed confirmed        -> Found=true,  Completed=true,  Confirmed=true  (drop: capacity owned)

func TestBookingByHashReturnsEmptyWhenNoRow(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	status, err := service.BookingByHash(ctx, "evt_missing", "deadbeef")
	require.NoError(t, err)
	assert.False(t, status.Found)
	assert.False(t, status.Completed)
	assert.False(t, status.Confirmed)
}

func TestBookingByHashReportsConfirmedAfterBooking(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lookup Confirmed",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "lookup-confirmed-1",
	})
	require.NoError(t, err)

	hash := reservation.Hash([]byte("reservation-test-secret"), "registration.book", event.EventID, "E1001", "lookup-confirmed-1")
	status, err := service.BookingByHash(ctx, event.EventID, hash)
	require.NoError(t, err)
	assert.True(t, status.Found)
	assert.True(t, status.Completed)
	assert.True(t, status.Confirmed)
}

func TestBookingByHashReportsWaitlistedAsSettledNonConfirmed(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lookup Waitlist",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	// Consume the only seat.
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lookup-waitlist-confirm"})
	require.NoError(t, err)
	// Second booking lands on waitlist.
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "lookup-waitlist-row"})
	require.NoError(t, err)

	hash := reservation.Hash([]byte("reservation-test-secret"), "registration.book", event.EventID, "E1002", "lookup-waitlist-row")
	status, err := service.BookingByHash(ctx, event.EventID, hash)
	require.NoError(t, err)
	assert.True(t, status.Found)
	assert.True(t, status.Completed)
	assert.False(t, status.Confirmed, "waitlisted bookings did not consume capacity")
}

func TestBookingByHashReportsInFlightAsFoundButNotCompleted(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lookup Inflight",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	// Simulate the half-written idempotency row that the gate produces
	// between INSERT and the booking completion UPDATE.
	_, err = service.db.Exec(ctx,
		`INSERT INTO booking_idempotency_results (idempotency_key, event_id, employee_id, family_count, idempotency_hash)
		 VALUES ($1, $2, 'E1001', 0, $3)`,
		"inflight-key", event.EventID, "fakehash-inflight")
	require.NoError(t, err)

	status, err := service.BookingByHash(ctx, event.EventID, "fakehash-inflight")
	require.NoError(t, err)
	assert.True(t, status.Found, "in-flight row must be visible to the compensator")
	assert.False(t, status.Completed, "in-flight row has completed_at IS NULL")
	assert.False(t, status.Confirmed)
}

func TestRemainingCapacityReflectsConfirmedBookings(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Capacity Lookup",
		Capacity: 3,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	remaining, err := service.RemainingCapacity(ctx, event.EventID)
	require.NoError(t, err)
	assert.Equal(t, 3, remaining)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "cap-1"})
	require.NoError(t, err)

	remaining, err = service.RemainingCapacity(ctx, event.EventID)
	require.NoError(t, err)
	assert.Equal(t, 2, remaining)
}

func TestRemainingCapacityIsZeroForUnlimitedAndMissingEvents(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Unlimited Capacity",
		CapacityType: CapacityTypeUnlimited,
		AllowsFamily: true,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	remaining, err := service.RemainingCapacity(ctx, event.EventID)
	require.NoError(t, err)
	assert.Equal(t, 0, remaining, "unlimited events have no advisory counter")

	remaining, err = service.RemainingCapacity(ctx, "evt_missing")
	require.NoError(t, err)
	assert.Equal(t, 0, remaining, "missing event returns zero, not an error — compensator never blocks on it")
}
