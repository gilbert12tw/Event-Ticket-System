package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultsRateLimitsDisabled(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "")
	t.Setenv("BOOKING_RATE_LIMIT_RPS_PER_ACTOR", "")
	t.Setenv("BOOKING_RATE_LIMIT_RPS_PER_EVENT", "")
	t.Setenv("RATE_LIMIT_OUTAGE_MODE", "")
	t.Setenv("BOOKING_RATE_LIMIT_HASH_SECRET", "")

	cfg := Load()

	assert.False(t, cfg.RateLimitEnabled)
	assert.Equal(t, 0, cfg.BookingRateLimitPerActor)
	assert.Equal(t, 0, cfg.BookingRateLimitPerEvent)
	assert.Equal(t, "degrade", cfg.RateLimitOutageMode)
	assert.Empty(t, cfg.BookingRateLimitHashSecret)
}

func TestLoadedConfigRejectsMalformedRateLimits(t *testing.T) {
	t.Setenv("BOOKING_RATE_LIMIT_RPS_PER_ACTOR", "-1")
	t.Setenv("BOOKING_RATE_LIMIT_RPS_PER_EVENT", "fast")

	err := Load().ValidateForServe()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "BOOKING_RATE_LIMIT_RPS_PER_ACTOR")
	assert.Contains(t, err.Error(), "BOOKING_RATE_LIMIT_RPS_PER_EVENT")
}
