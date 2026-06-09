package ticketing

import (
	"strings"
	"testing"
	"time"
)

// TestProjectionOffsetKey_LexicalOrderMatchesTime is the regression guard for the
// RFC3339Nano monotonicity bug: with that format "…56.1Z" sorted AFTER "…56.12Z",
// so a 100ms event looked newer than a 120ms event and could overwrite it. The
// zero-padded UnixNano composite must keep lexical order == chronological order,
// including the exact truncation-prefix case (100ms vs 120ms).
func TestProjectionOffsetKey_LexicalOrderMatchesTime(t *testing.T) {
	base := time.Date(2026, 6, 9, 12, 0, 56, 0, time.UTC)
	cases := []struct {
		name    string
		earlier time.Time
		later   time.Time
	}{
		{"100ms before 120ms", base.Add(100 * time.Millisecond), base.Add(120 * time.Millisecond)},
		{"200ms before 250ms", base.Add(200 * time.Millisecond), base.Add(250 * time.Millisecond)},
		{"1ms before 1us-more", base.Add(1 * time.Millisecond), base.Add(1*time.Millisecond + time.Microsecond)},
		{"whole second", base, base.Add(time.Second)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			older := projectionOffsetKey(tc.earlier, "out_a")
			newer := projectionOffsetKey(tc.later, "out_a")
			if !(older < newer) {
				t.Fatalf("lexical order broken: older=%q must sort before newer=%q", older, newer)
			}
		})
	}
}

// TestProjectionOffsetKey_FixedWidthAndTiebreak verifies the timestamp field is a
// fixed 20-digit width (so comparisons never short-circuit on length) and that
// outboxID breaks ties for events sharing an instant.
func TestProjectionOffsetKey_FixedWidthAndTiebreak(t *testing.T) {
	instant := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	a := projectionOffsetKey(instant, "out_a")
	b := projectionOffsetKey(instant, "out_b")
	if tsField := strings.SplitN(a, "|", 2)[0]; len(tsField) != 20 {
		t.Fatalf("timestamp field not 20 digits wide: %q (field=%q)", a, tsField)
	}
	if !(a < b) {
		t.Fatalf("tiebreak by outboxID broken: %q must sort before %q", a, b)
	}
}

// TestProjectionOffsetKey_UTCNormalization proves the same instant expressed in
// different zones yields an identical key — claim.createdAt comes from Postgres
// and may arrive in a non-UTC location depending on pgx configuration.
func TestProjectionOffsetKey_UTCNormalization(t *testing.T) {
	taipei, err := time.LoadLocation("Asia/Taipei") // UTC+8
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	local := time.Date(2026, 6, 9, 20, 0, 0, 0, taipei)
	utc := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) // same instant
	if got, want := projectionOffsetKey(local, "out_x"), projectionOffsetKey(utc, "out_x"); got != want {
		t.Fatalf("UTC normalization broken: %q != %q", got, want)
	}
}
