// Package reservation implements the Phase 2 PH2-22 Redis pre-admission
// gate. PostgreSQL remains the only source of truth for committed bookings;
// the gate only decides whether a request may enter the expensive DB path
// during pressure on a limited event.
//
// Spec: docs/specs/phase2-redis-reservation-gate.md.
package reservation

import (
	"context"
	"errors"
	"time"
)

// Outcome is the one-of result of a reservation attempt.
type Outcome string

const (
	OutcomeGranted       Outcome = "granted"
	OutcomeDuplicate     Outcome = "duplicate"
	OutcomeExhausted     Outcome = "exhausted"
	OutcomeMisconfigured Outcome = "misconfigured"
)

// Hold is the redacted snapshot the application service uses after a reserve.
// It deliberately omits any field that could correlate to an employee identity.
type Hold struct {
	Outcome         Outcome
	ReservationID   string
	IdempotencyHash string
	CapacityVersion int64
	ExpiresAt       time.Time
}

// CapacityProbe rebuilds the advisory remaining counter from PostgreSQL truth
// the first time the gate sees an event. The probe must return
// `capacity - confirmedCount` (i.e. remaining seats), not the raw capacity.
// capacityVersion lets the gate tag the hold for compensation/drift checks.
type CapacityProbe func(ctx context.Context) (remaining int, capacityVersion int64, err error)

// Gate is the contract the booking application service depends on. The
// implementation is either RedisGate (production / dev with Redis) or NoopGate
// (gate disabled — preserves the Phase 1 DB-only path).
type Gate interface {
	Enabled() bool
	Reserve(ctx context.Context, eventID, idempotencyHash, actorHash string, probe CapacityProbe) (Hold, error)
	Confirm(ctx context.Context, eventID, idempotencyHash string) error
	Release(ctx context.Context, eventID, idempotencyHash string) error
}

// NoopGate is the disabled gate. Reserve always returns OutcomeGranted so the
// caller falls through to the Phase 1 DB-only path.
type NoopGate struct{}

func (NoopGate) Enabled() bool { return false }

func (NoopGate) Reserve(_ context.Context, _, _, _ string, _ CapacityProbe) (Hold, error) {
	return Hold{Outcome: OutcomeGranted}, nil
}

func (NoopGate) Confirm(_ context.Context, _, _ string) error { return nil }
func (NoopGate) Release(_ context.Context, _, _ string) error { return nil }

// ErrUnavailable is returned when the gate is configured to fail closed and
// Redis is unreachable, times out, or returns misconfigured.
var ErrUnavailable = errors.New("reservation gate unavailable")
