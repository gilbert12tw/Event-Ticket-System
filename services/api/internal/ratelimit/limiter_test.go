package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

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
