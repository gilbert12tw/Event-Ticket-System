package ratelimit

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	counts map[string]int64
	err    error
}

func (s *fakeStore) Increment(_ context.Context, key string, _ time.Duration) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	if s.counts == nil {
		s.counts = map[string]int64{}
	}
	s.counts[key]++
	return s.counts[key], nil
}

func TestLimiterThrottlesActorScopeBeforeEventScope(t *testing.T) {
	store := &fakeStore{}
	limiter := NewRedisLimiter(store, Config{
		Enabled:             true,
		ActorLimitPerSecond: 1,
		EventLimitPerSecond: 10,
		OutageMode:          OutageModeFail,
		OperationTimeout:    time.Second,
	}, nil)
	limiter.now = func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) }

	first, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")
	require.NoError(t, err)
	require.True(t, first.Allowed)
	second, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")

	require.NoError(t, err)
	assert.False(t, second.Allowed)
	assert.Equal(t, ScopeActor, second.Scope)
	assert.Equal(t, time.Second, second.RetryAfter)
}

func TestLimiterThrottlesEventScope(t *testing.T) {
	store := &fakeStore{}
	limiter := NewRedisLimiter(store, Config{
		Enabled:             true,
		EventLimitPerSecond: 1,
		OutageMode:          OutageModeFail,
		OperationTimeout:    time.Second,
	}, nil)
	limiter.now = func() time.Time { return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC) }

	first, err := limiter.Allow(context.Background(), "evt_1", "actor_1")
	require.NoError(t, err)
	require.True(t, first.Allowed)
	second, err := limiter.Allow(context.Background(), "evt_1", "actor_2")

	require.NoError(t, err)
	assert.False(t, second.Allowed)
	assert.Equal(t, ScopeEvent, second.Scope)
}

func TestLimiterDegradesOpenOnStoreFailure(t *testing.T) {
	limiter := NewRedisLimiter(&fakeStore{err: errors.New("redis down")}, Config{
		Enabled:             true,
		ActorLimitPerSecond: 1,
		OutageMode:          OutageModeDegrade,
		OperationTimeout:    time.Second,
	}, nil)

	decision, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
}

func TestLimiterFailsClosedOnStoreFailure(t *testing.T) {
	limiter := NewRedisLimiter(&fakeStore{err: errors.New("redis down")}, Config{
		Enabled:             true,
		ActorLimitPerSecond: 1,
		OutageMode:          OutageModeFail,
		OperationTimeout:    time.Second,
	}, nil)

	_, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")

	require.ErrorIs(t, err, ErrUnavailable)
}

func TestNoopLimiterAlwaysAllows(t *testing.T) {
	limiter := NoopLimiter{}

	assert.False(t, limiter.Enabled())
	decision, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
}

func TestLimiterEnabledRequiresConfiguredLimit(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{name: "disabled flag", cfg: Config{Enabled: false, ActorLimitPerSecond: 1}, want: false},
		{name: "enabled without limits", cfg: Config{Enabled: true}, want: false},
		{name: "actor limit only", cfg: Config{Enabled: true, ActorLimitPerSecond: 1}, want: true},
		{name: "event limit only", cfg: Config{Enabled: true, EventLimitPerSecond: 1}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NewRedisLimiter(&fakeStore{}, tc.cfg, nil).Enabled())
		})
	}

	var nilLimiter *RedisLimiter
	assert.False(t, nilLimiter.Enabled())
}

func TestLimiterAllowsWithoutCheckWhenDisabled(t *testing.T) {
	store := &fakeStore{}
	limiter := NewRedisLimiter(store, Config{Enabled: false}, nil)

	decision, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Empty(t, store.counts, "disabled limiter must not touch the store")
}

func TestLimiterActorOnlySkipsEventScope(t *testing.T) {
	store := &fakeStore{}
	limiter := NewRedisLimiter(store, Config{
		Enabled:             true,
		ActorLimitPerSecond: 5,
		OutageMode:          OutageModeFail,
		OperationTimeout:    time.Second,
	}, nil)

	decision, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")

	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	assert.Len(t, store.counts, 1, "only the actor key must be incremented")
}

func TestNewRedisLimiterAppliesDefaults(t *testing.T) {
	limiter := NewRedisLimiter(&fakeStore{}, Config{Enabled: true, ActorLimitPerSecond: 1}, nil)

	assert.Equal(t, OutageModeDegrade, limiter.cfg.OutageMode)
	assert.Equal(t, 150*time.Millisecond, limiter.cfg.OperationTimeout)
}

func TestLimiterClassifiesTimeoutOutage(t *testing.T) {
	limiter := NewRedisLimiter(&fakeStore{err: context.DeadlineExceeded}, Config{
		Enabled:             true,
		ActorLimitPerSecond: 1,
		OutageMode:          OutageModeFail,
		OperationTimeout:    time.Second,
	}, nil)

	_, err := limiter.Allow(context.Background(), "evt_1", "actor_hash")

	require.ErrorIs(t, err, ErrUnavailable)
	assert.Equal(t, "timeout", classify(context.DeadlineExceeded))
	assert.Equal(t, "redis_error", classify(errors.New("boom")))
}

func TestParseOutageMode(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  OutageMode
	}{
		{name: "blank defaults to degrade", value: "", want: OutageModeDegrade},
		{name: "trims and lowercases degrade", value: "  DeGrAdE ", want: OutageModeDegrade},
		{name: "trims and lowercases fail", value: " FAIL ", want: OutageModeFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOutageMode(tt.value)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := ParseOutageMode("panic")
	require.ErrorContains(t, err, "RATE_LIMIT_OUTAGE_MODE")
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "disabled is always valid", cfg: Config{Enabled: false, ActorLimitPerSecond: -1, EventLimitPerSecond: -1}},
		{name: "enabled without limits is valid", cfg: Config{Enabled: true}},
		{name: "enabled without limits ignores operational timeout", cfg: Config{Enabled: true, OperationTimeout: -time.Second}},
		{name: "negative actor limit", cfg: Config{Enabled: true, ActorLimitPerSecond: -1, OutageMode: OutageModeDegrade, OperationTimeout: time.Second}, wantErr: "BOOKING_RATE_LIMIT_RPS_PER_ACTOR"},
		{name: "negative event limit", cfg: Config{Enabled: true, EventLimitPerSecond: -1, OutageMode: OutageModeDegrade, OperationTimeout: time.Second}, wantErr: "BOOKING_RATE_LIMIT_RPS_PER_EVENT"},
		{name: "missing timeout", cfg: Config{Enabled: true, ActorLimitPerSecond: 1, OutageMode: OutageModeDegrade}, wantErr: "REDIS_OPERATION_TIMEOUT_MS"},
		{name: "bad outage mode", cfg: Config{Enabled: true, ActorLimitPerSecond: 1, OperationTimeout: time.Second, OutageMode: "explode"}, wantErr: "RATE_LIMIT_OUTAGE_MODE"},
		{name: "valid", cfg: Config{Enabled: true, ActorLimitPerSecond: 1, OperationTimeout: time.Second, OutageMode: OutageModeFail}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestRedisStoreIncrementUsesAtomicCounterWithTTL(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not set; skipping Redis-backed rate limit store test")
	}
	opts, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	key := keyPrefix + "test:" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_ = client.Del(context.Background(), key).Err()
	})

	store := NewRedisStore(client)
	first, err := store.Increment(ctx, key, time.Minute)
	require.NoError(t, err)
	second, err := store.Increment(ctx, key, time.Minute)
	require.NoError(t, err)
	ttl, err := client.TTL(ctx, key).Result()
	require.NoError(t, err)

	assert.Equal(t, int64(1), first)
	assert.Equal(t, int64(2), second)
	assert.Positive(t, ttl)
}
