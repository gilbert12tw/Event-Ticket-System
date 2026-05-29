package ticketing

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"event-ticket-system/internal/reservation"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withRedisGate attaches a real RedisGate to the service and returns a cleanup
// that flushes the gate-owned keys and closes the connection. The test skips
// when REDIS_URL is not set so `go test` without Compose still passes.
func withRedisGate(t *testing.T, service *Service) func() {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set; skipping gate-enabled booking test")
	}
	opts, err := redis.ParseURL(url)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	require.NoError(t, client.Ping(context.Background()).Err())
	flushKeys(t, client)
	gate := reservation.NewRedisGate(client, reservation.Config{
		Enabled:          true,
		OutageMode:       reservation.OutageModeDegrade,
		HashSecret:       []byte("reservation-test-secret"),
		TTL:              10 * time.Second,
		OperationTimeout: 500 * time.Millisecond,
	}, nil)
	service.WithReservationGate(gate, []byte("reservation-test-secret"))
	return func() {
		flushKeys(t, client)
		_ = client.Close()
	}
}

func flushKeys(t *testing.T, client *redis.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	keys, err := client.Keys(ctx, "cets:v1:resv:*").Result()
	if err == nil && len(keys) > 0 {
		_ = client.Del(ctx, keys...).Err()
	}
}

func TestBookingWithGateConfirmsLimitedSeatAndDecrementsCounter(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Redis Gate Confirm",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "gate-confirm-1"})
	require.NoError(t, err)
	assert.Equal(t, RegistrationConfirmed, first.Registration.Status)

	// After a confirmed booking, the hold is gone and the counter reflects
	// remaining capacity (capacity=2, remaining after one confirm = 1).
	client := redisClientFromEnv(t)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	remaining, err := client.Get(ctx, "cets:v1:resv:"+event.EventID+":remaining").Int()
	require.NoError(t, err)
	assert.Equal(t, 1, remaining)
}

func TestBookingWithGateFallsBackToWaitlistOnExhausted(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Redis Gate Exhausted",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	// Consume the only seat.
	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "exhaust-1"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, first.Registration.Status)

	// Second booking should hit Redis 'exhausted' and land on waitlist; DB
	// remains the source of truth so the row IS persisted.
	second, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "exhaust-2"})
	require.NoError(t, err)
	assert.Equal(t, RegistrationWaitlisted, second.Registration.Status)
	assert.Nil(t, second.Ticket, "waitlisted bookings must not carry tickets")
}

func TestBookingWithGateConcurrentBookingsRespectCapacity(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	const capacity = 3
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Redis Gate Race",
		Capacity: capacity,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	// Insert enough HR-synced employees beyond demo seed.
	for i := 0; i < 10; i++ {
		_, err := service.db.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1, $2, 'Engineering', 'Taipei HQ', 6, 'active')
			ON CONFLICT (employee_id) DO NOTHING`, "EHOT"+strings.Repeat("0", 3-len(itoa(i)))+itoa(i), "Hot "+itoa(i))
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	results := make([]string, 10)
	errors := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := "EHOT" + strings.Repeat("0", 3-len(itoa(idx))) + itoa(idx)
			r, err := service.Book(ctx, Actor{ID: id, Role: RoleEmployee}, event.EventID,
				BookingRequest{EmployeeID: id, IdempotencyKey: "race-" + itoa(idx)})
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = r.Registration.Status
		}(i)
	}
	wg.Wait()

	confirmed := 0
	waitlisted := 0
	for i, status := range results {
		if errors[i] != nil {
			continue
		}
		switch status {
		case RegistrationConfirmed:
			confirmed++
		case RegistrationWaitlisted:
			waitlisted++
		}
	}
	assert.Equal(t, capacity, confirmed, "confirmed bookings must equal capacity (oversell-safe)")
	assert.Greater(t, waitlisted, 0, "remaining bookings must be waitlisted")

	// PostgreSQL is final truth — the confirmed-count constraint never breaks
	// regardless of what Redis decided.
	var dbConfirmed int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID).Scan(&dbConfirmed))
	assert.Equal(t, capacity, dbConfirmed)
}

func TestBookingWithGateUnlimitedEventBypassesRedis(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Unlimited Bypass",
		CapacityType: CapacityTypeUnlimited,
		AllowsFamily: true,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unlimited-1"})
	require.NoError(t, err)
	assert.Equal(t, RegistrationConfirmed, first.Registration.Status)

	// No Redis keys should be created for unlimited events.
	client := redisClientFromEnv(t)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	keys, err := client.Keys(ctx, "cets:v1:resv:"+event.EventID+":*").Result()
	require.NoError(t, err)
	assert.Empty(t, keys, "unlimited events must not touch Redis")
}

func TestBookingWithGateIdempotentReplayDoesNotDoubleDecrement(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	flushRedis := withRedisGate(t, service)
	defer flushRedis()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Idempotent Replay",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "replay-1"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, first.Registration.Status)
	// Replay same key — must return the same registration AND must not
	// further decrement the Redis counter.
	replay, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "replay-1"})
	require.NoError(t, err)
	assert.True(t, replay.Duplicate)
	assert.Equal(t, first.Registration.RegistrationID, replay.Registration.RegistrationID)

	client := redisClientFromEnv(t)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	remaining, err := client.Get(ctx, "cets:v1:resv:"+event.EventID+":remaining").Int()
	require.NoError(t, err)
	assert.Equal(t, 4, remaining, "replay must not decrement again (5 - 1 confirmed = 4)")
}

func TestBookingWithGateReplayDoesNotReserveAgain(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	gate := &recordingReservationGate{outcome: reservation.OutcomeGranted}
	service.WithReservationGate(gate, []byte("reservation-test-secret"))
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Replay Skips Gate",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "replay-skip-gate"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, first.Registration.Status)
	require.Equal(t, 1, gate.reserveCalls)
	require.Equal(t, 1, gate.confirmCalls)

	replay, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "replay-skip-gate"})
	require.NoError(t, err)
	assert.True(t, replay.Duplicate)
	assert.Equal(t, first.Registration.RegistrationID, replay.Registration.RegistrationID)
	assert.Equal(t, 1, gate.reserveCalls, "completed idempotency replay must return before Redis reservation")
}

func TestBookingWithGateRejectsLimitedFamilyBeforeReserve(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	gate := &recordingReservationGate{outcome: reservation.OutcomeGranted}
	service.WithReservationGate(gate, []byte("reservation-test-secret"))
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Limited Family Gate",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "limited-family-gate",
		FamilyCount:    1,
	})
	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
	assert.Equal(t, 0, gate.reserveCalls, "invalid limited family request must not create a Redis hold")
}

func TestBookingWithGateFailClosedUnavailableReturns503(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	gate := &recordingReservationGate{err: reservation.ErrUnavailable}
	service.WithReservationGate(gate, []byte("reservation-test-secret"))
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Gate Unavailable",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "gate-unavailable"})
	require.Error(t, err)
	assert.Equal(t, 503, ErrorStatus(err))
	assert.Equal(t, "RESERVATION_GATE_UNAVAILABLE", ErrorCode(err))
}

func TestBookingWithGateExhaustedPathWaitsForEventStateLock(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	gate := &recordingReservationGate{outcome: reservation.OutcomeExhausted}
	service.WithReservationGate(gate, []byte("reservation-test-secret"))
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Exhausted Path State Lock",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	lockTx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	_, err = lockTx.Exec(ctx, `SELECT event_id FROM events WHERE event_id = $1 FOR UPDATE`, event.EventID)
	require.NoError(t, err)

	done := make(chan BookingResponse, 1)
	errs := make(chan error, 1)
	go func() {
		res, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID,
			BookingRequest{EmployeeID: "E1001", IdempotencyKey: "exhausted-state-lock"})
		if err != nil {
			errs <- err
			return
		}
		done <- res
	}()

	select {
	case res := <-done:
		require.Failf(t, "booking completed while event row update lock was held", "status=%s", res.Registration.Status)
	case err := <-errs:
		require.Failf(t, "booking errored while event row update lock was held", "err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, lockTx.Rollback(ctx))
	select {
	case res := <-done:
		assert.Equal(t, RegistrationWaitlisted, res.Registration.Status)
	case err := <-errs:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		require.Fail(t, "booking did not finish after releasing event row lock")
	}
}

type recordingReservationGate struct {
	outcome      reservation.Outcome
	err          error
	reserveCalls int
	confirmCalls int
	releaseCalls int
}

func (g *recordingReservationGate) Enabled() bool { return true }

func (g *recordingReservationGate) Reserve(_ context.Context, _, idempotencyHash, _ string, _ reservation.CapacityProbe) (reservation.Hold, error) {
	g.reserveCalls++
	if g.err != nil {
		return reservation.Hold{}, g.err
	}
	outcome := g.outcome
	if outcome == "" {
		outcome = reservation.OutcomeGranted
	}
	return reservation.Hold{Outcome: outcome, IdempotencyHash: idempotencyHash}, nil
}

func (g *recordingReservationGate) Confirm(context.Context, string, string) error {
	g.confirmCalls++
	return nil
}

func (g *recordingReservationGate) Release(context.Context, string, string) error {
	g.releaseCalls++
	return nil
}

func redisClientFromEnv(t *testing.T) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	require.NoError(t, err)
	return redis.NewClient(opts)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
