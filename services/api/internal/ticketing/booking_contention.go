package ticketing

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	BookingContentionStrategyPhase1   = "phase1"
	BookingContentionStrategyAdvisory = "advisory"
)

func ParseBookingContentionStrategy(strategy string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(strategy))
	if value == "" {
		return BookingContentionStrategyPhase1, nil
	}
	switch value {
	case BookingContentionStrategyPhase1, BookingContentionStrategyAdvisory:
		return value, nil
	default:
		return "", fmt.Errorf("BOOKING_CONTENTION_STRATEGY must be one of phase1|advisory, got %q", strategy)
	}
}

func (s *Service) advisoryBookingContentionEnabled() bool {
	return s.bookingContentionStrategy == BookingContentionStrategyAdvisory
}

func (s *Service) acquireBookingAdvisoryLockTx(ctx context.Context, tx pgx.Tx, eventID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, bookingAdvisoryLockKey(eventID))
	return err
}

func bookingAdvisoryLockKey(eventID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("registration.book:"))
	_, _ = h.Write([]byte(eventID))
	return int64(h.Sum64())
}
