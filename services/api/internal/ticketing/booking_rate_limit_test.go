package ticketing

import (
	"context"
	"testing"
	"time"

	"event-ticket-system/internal/ratelimit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBookingLimiter struct {
	enabled  bool
	decision ratelimit.Decision
	err      error
	calls    int
}

func (l *fakeBookingLimiter) Enabled() bool { return l.enabled }

func (l *fakeBookingLimiter) Allow(_ context.Context, _, _ string) (ratelimit.Decision, error) {
	l.calls++
	return l.decision, l.err
}

func TestBookingRateLimitReturns429BeforeBusinessMutation(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event := createPublishedEvent(t, service, ctx, engineeringEventRequest("Rate limited", 10, 1))
	limiter := &fakeBookingLimiter{
		enabled: true,
		decision: ratelimit.Decision{
			Allowed:    false,
			Scope:      ratelimit.ScopeActor,
			RetryAfter: time.Second,
		},
	}
	service.WithBookingRateLimiter(limiter, []byte("rate-limit-secret"))

	_, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "rate-limited-1",
	})

	require.Error(t, err)
	assert.Equal(t, 429, ErrorStatus(err))
	assert.Equal(t, "BOOKING_RATE_LIMITED", ErrorCode(err))
	retryAfter, ok := ErrorRetryAfterSeconds(err)
	require.True(t, ok)
	assert.Equal(t, 1, retryAfter)
	assert.Equal(t, 1, limiter.calls)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1`, event.EventID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE event_id = $1`, event.EventID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events
		WHERE event_type LIKE 'booking.%'
			AND aggregate_id IN (SELECT registration_id FROM registrations WHERE event_id = $1)`, event.EventID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'booking.rate_limited' AND entity_id = $1`, event.EventID, 1)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs WHERE action = 'booking.rate_limited' AND entity_id = $1`, event.EventID)
	assert.Equal(t, event.EventID, audit["event_id"])
	assert.Equal(t, string(ratelimit.ScopeActor), audit["scope"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "rate-limited-1")
}

func TestCompletedBookingReplayBypassesRateLimit(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event := createPublishedEvent(t, service, ctx, engineeringEventRequest("Replay bypasses limiter", 10, 1))
	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "rate-replay-1",
	})
	require.NoError(t, err)
	limiter := &fakeBookingLimiter{
		enabled: true,
		decision: ratelimit.Decision{
			Allowed:    false,
			Scope:      ratelimit.ScopeActor,
			RetryAfter: time.Second,
		},
	}
	service.WithBookingRateLimiter(limiter, []byte("rate-limit-secret"))

	replay, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "rate-replay-1",
	})

	require.NoError(t, err)
	assert.Equal(t, first.Registration.RegistrationID, replay.Registration.RegistrationID)
	assert.True(t, replay.Duplicate)
	assert.Equal(t, 0, limiter.calls)
}
