package ticketing

import (
	"context"
	"strconv"
	"testing"
	"time"

	"event-ticket-system/internal/reservation"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PH2-23 end-to-end: drive the production Compensator with the real
// Service-backed BookingLookup against live PG + Redis to confirm the
// reconciliation contract holds when the wiring is wrong-end-up (not just
// the fake lookup used in internal/reservation/compensation_test.go).

func newE2ECompensator(t *testing.T, service *Service, client redis.UniversalClient) *reservation.Compensator {
	t.Helper()
	return reservation.NewCompensator(client, reservation.CompensationConfig{
		GraceTTL:         50 * time.Millisecond,
		BatchSize:        16,
		MaxEvents:        16,
		DriftMarkerTTL:   5 * time.Second,
		OperationTimeout: 500 * time.Millisecond,
	}, service, nil, nil)
}

// expirePendingMember backdates the pending zset score so the
// compensator's `now - grace` filter actually picks it up — mirrors the
// helper used in internal/reservation/compensation_test.go, replicated
// here because production code never rewrites pending scores.
func expirePendingMember(t *testing.T, client redis.UniversalClient, eventID, idempotencyHash string) {
	t.Helper()
	past := time.Now().Add(-1 * time.Hour).Unix()
	require.NoError(t, client.ZAdd(context.Background(), "cets:v1:resv:"+eventID+":pending",
		redis.Z{Score: float64(past), Member: idempotencyHash}).Err())
}

func TestCompensatorReleasesOrphanHoldThroughLiveLookup(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "E2E Orphan",
		Capacity: 3,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	client := redisClientFromEnv(t)
	defer func() { _ = client.Close() }()
	comp := newE2ECompensator(t, service, client)

	// Seed an orphan hold: counter decremented to 2, hold + pending member
	// in place, but no booking_idempotency_results row. Mirrors the "Redis
	// granted, DB tx never committed" failure mode (process crash mid-tx).
	orphanHash := "orphan-" + strconv.FormatInt(time.Now().UnixNano(), 16)
	require.NoError(t, client.Set(ctx, "cets:v1:resv:"+event.EventID+":remaining", 2, 0).Err())
	require.NoError(t, client.HSet(ctx, "cets:v1:resv:"+event.EventID+":hold:"+orphanHash,
		"reservation_id", "resv_e2e",
		"event_id", event.EventID,
		"idempotency_hash", orphanHash,
		"seats", "1",
		"state", "reserved",
	).Err())
	expirePendingMember(t, client, event.EventID, orphanHash)

	require.NoError(t, comp.Sweep(ctx))

	remaining, err := client.Get(ctx, "cets:v1:resv:"+event.EventID+":remaining").Int()
	require.NoError(t, err)
	assert.Equal(t, 3, remaining, "compensator must return the orphan slot")
	exists, err := client.Exists(ctx, "cets:v1:resv:"+event.EventID+":hold:"+orphanHash).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "orphan hold must be removed")
}

func TestCompensatorDropsHoldWhenLiveBookingConfirmed(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "E2E Confirmed Drop",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	client := redisClientFromEnv(t)
	defer func() { _ = client.Close() }()

	// Drive a real booking through the gate so the hash matches what's in
	// PostgreSQL; PR3 e2e contract is "real adapter, real Redis, real PG".
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "e2e-confirm",
	})
	require.NoError(t, err)
	hash := reservation.Hash([]byte("reservation-test-secret"), "registration.book", event.EventID, "E1001", "e2e-confirm")

	// The booking flow already called Confirm — re-seed the hold and the
	// pending member to simulate a stale state the worker must drop.
	require.NoError(t, client.HSet(ctx, "cets:v1:resv:"+event.EventID+":hold:"+hash,
		"reservation_id", "resv_stale",
		"idempotency_hash", hash,
		"state", "reserved",
	).Err())
	expirePendingMember(t, client, event.EventID, hash)

	comp := newE2ECompensator(t, service, client)
	require.NoError(t, comp.Sweep(ctx))

	// DB-derived remaining is 1 (capacity 2 minus 1 confirmed). Drop must
	// NOT increment, so the counter ends at what it was (whatever cap the
	// gate seeded) or capped down to the DB truth — never above.
	remaining, err := client.Get(ctx, "cets:v1:resv:"+event.EventID+":remaining").Int()
	require.NoError(t, err)
	assert.LessOrEqual(t, remaining, 1, "drop on confirmed booking must NOT increment the advisory counter")
	exists, err := client.Exists(ctx, "cets:v1:resv:"+event.EventID+":hold:"+hash).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestCompensatorCapsDriftedCounterAgainstLiveCapacityProbe(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "E2E Drift Cap",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	client := redisClientFromEnv(t)
	defer func() { _ = client.Close() }()
	comp := newE2ECompensator(t, service, client)

	require.NoError(t, client.Set(ctx, "cets:v1:resv:"+event.EventID+":remaining", 999, 0).Err())
	require.NoError(t, comp.SweepEvent(ctx, event.EventID))

	remaining, err := client.Get(ctx, "cets:v1:resv:"+event.EventID+":remaining").Int()
	require.NoError(t, err)
	assert.Equal(t, 5, remaining, "drift cap must clamp to DB-derived remaining capacity (5 - 0 confirmed)")
}
