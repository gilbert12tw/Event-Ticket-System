package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDemoCheckinStartBounds(t *testing.T) {
	loc := checkinBusinessLocation()
	cases := []struct {
		name       string
		now        time.Time
		wantToday  time.Time
		wantFuture time.Time
	}{
		{
			name:       "local morning",
			now:        time.Date(2026, 6, 8, 1, 30, 0, 0, loc),
			wantToday:  time.Date(2026, 6, 8, 0, 0, 0, 0, loc).UTC(),
			wantFuture: time.Date(2026, 6, 9, 0, 0, 0, 0, loc).UTC(),
		},
		{
			name:       "exact local midnight",
			now:        time.Date(2026, 6, 8, 0, 0, 0, 0, loc),
			wantToday:  time.Date(2026, 6, 8, 0, 0, 0, 0, loc).UTC(),
			wantFuture: time.Date(2026, 6, 9, 0, 0, 0, 0, loc).UTC(),
		},
		{
			name:       "late local evening",
			now:        time.Date(2026, 6, 8, 23, 59, 0, 0, loc),
			wantToday:  time.Date(2026, 6, 8, 0, 0, 0, 0, loc).UTC(),
			wantFuture: time.Date(2026, 6, 9, 0, 0, 0, 0, loc).UTC(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := tc.now.UTC()

			todayStarts := demoTodayCheckinStart(now)
			futureStarts := demoNextCheckinStart(todayStarts)

			assert.Equal(t, tc.wantToday, todayStarts)
			assert.Equal(t, tc.wantFuture, futureStarts)
			assert.False(t, todayStarts.After(now), "today check-in window must already be open")
			assert.True(t, now.Before(futureStarts), "future check-in window must start after now")
		})
	}
}
