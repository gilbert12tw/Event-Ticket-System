package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultsBookingContentionStrategyPhase1(t *testing.T) {
	t.Setenv("BOOKING_CONTENTION_STRATEGY", "")

	cfg := Load()

	assert.Equal(t, "phase1", cfg.BookingContentionStrategy)
}

func TestLoadedConfigRejectsMalformedBookingContentionStrategy(t *testing.T) {
	t.Setenv("BOOKING_CONTENTION_STRATEGY", "queue")

	err := Load().ValidateForServe()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "BOOKING_CONTENTION_STRATEGY")
}
