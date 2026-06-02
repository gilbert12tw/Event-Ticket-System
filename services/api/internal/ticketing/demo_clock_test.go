package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoClockSwitchesBetweenRealAndFixedTime(t *testing.T) {
	realNow := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	clock := NewDemoClockWithRealNow(func() time.Time { return realNow })

	assert.Equal(t, realNow, clock.Now())
	assert.Equal(t, DemoClockModeReal, clock.Snapshot().Mode)

	fixed := time.Date(2026, 6, 1, 9, 30, 0, 0, time.FixedZone("TST", 8*60*60))
	snapshot := clock.UseFixed(fixed, "cutoff demo")

	assert.Equal(t, DemoClockModeFixed, snapshot.Mode)
	assert.Equal(t, fixed.UTC(), clock.Now())
	assert.Equal(t, "cutoff demo", snapshot.Reason)
	require.NotNil(t, snapshot.UpdatedAt)
	assert.Equal(t, realNow, *snapshot.UpdatedAt)

	realNow = realNow.Add(time.Hour)
	snapshot = clock.UseReal("reset")

	assert.Equal(t, DemoClockModeReal, snapshot.Mode)
	assert.Equal(t, realNow, clock.Now())
	assert.Equal(t, "reset", snapshot.Reason)
}

func TestServiceWithClockUsesDemoBusinessTime(t *testing.T) {
	clock := NewDemoClockWithRealNow(func() time.Time {
		return time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	})
	service := NewService(nil, NewSigner("secret"), nil).WithClock(clock.Now)

	assert.Equal(t, time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC), service.now())

	clock.UseFixed(time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC), "cutoff")

	assert.Equal(t, time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC), service.now())
}
