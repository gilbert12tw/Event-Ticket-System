package ticketing

import (
	"context"
	"testing"

	"event-ticket-system/internal/reservation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PH2-23 storage prereq: the booking idempotency row must carry the HMAC
// idempotency_hash whenever the Redis gate ran, so the compensation worker
// can look up (event_id, idempotency_hash) and decide whether an orphan
// Redis hold should be released. Phase 1 bookings (gate=off) must continue
// to write a row with idempotency_hash IS NULL — never blocked, never
// retroactively backfilled.

func TestBookingIdempotencyHashStoredWhenGateEnabled(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Hash Storage",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "hash-store-1",
	})
	require.NoError(t, err)

	expected := reservation.Hash([]byte("reservation-test-secret"), "registration.book", event.EventID, "E1001", "hash-store-1")
	var stored *string
	require.NoError(t, service.db.QueryRow(ctx,
		`SELECT idempotency_hash FROM booking_idempotency_results WHERE idempotency_key = $1`,
		"hash-store-1",
	).Scan(&stored))
	require.NotNil(t, stored, "idempotency_hash must be populated when gate is enabled")
	assert.Equal(t, expected, *stored)
}

func TestBookingIdempotencyHashNullWhenGateDisabled(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Hash Null Phase 1",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	// gate=off (NoopGate) — preadmitBooking returns idempotencyHash="".
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "hash-null-1",
	})
	require.NoError(t, err)

	var stored *string
	require.NoError(t, service.db.QueryRow(ctx,
		`SELECT idempotency_hash FROM booking_idempotency_results WHERE idempotency_key = $1`,
		"hash-null-1",
	).Scan(&stored))
	assert.Nil(t, stored, "idempotency_hash must be NULL when gate is disabled (Phase 1 path)")
}

func TestBookingIdempotencyHashPartialUniquePreventsDuplicatePerEvent(t *testing.T) {
	// The partial unique index `idx_booking_idempotency_results_event_hash`
	// must reject two non-null hashes that collide on the same event. Same
	// hash for a different event is fine (different scope).
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Hash Unique",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.db.Exec(ctx,
		`INSERT INTO booking_idempotency_results (idempotency_key, event_id, employee_id, family_count, idempotency_hash)
		 VALUES ($1, $2, $3, 0, $4)`,
		"unique-key-1", event.EventID, "E1001", "abcdef0123456789")
	require.NoError(t, err)

	// Same (event_id, idempotency_hash) but different idempotency_key — must be rejected.
	_, err = service.db.Exec(ctx,
		`INSERT INTO booking_idempotency_results (idempotency_key, event_id, employee_id, family_count, idempotency_hash)
		 VALUES ($1, $2, $3, 0, $4)`,
		"unique-key-2", event.EventID, "E1002", "abcdef0123456789")
	require.Error(t, err, "partial unique index must reject duplicate (event_id, idempotency_hash)")

	// Two NULL hashes on the same event must be allowed (partial index ignores NULLs).
	_, err = service.db.Exec(ctx,
		`INSERT INTO booking_idempotency_results (idempotency_key, event_id, employee_id, family_count, idempotency_hash)
		 VALUES ($1, $2, $3, 0, NULL)`,
		"null-key-1", event.EventID, "E1003")
	require.NoError(t, err)
	_, err = service.db.Exec(ctx,
		`INSERT INTO booking_idempotency_results (idempotency_key, event_id, employee_id, family_count, idempotency_hash)
		 VALUES ($1, $2, $3, 0, NULL)`,
		"null-key-2", event.EventID, "E1004")
	require.NoError(t, err, "two NULL idempotency_hash rows must coexist on the same event")
}
