package ticketing

import (
	"context"
	"errors"

	"event-ticket-system/internal/reservation"

	"github.com/jackc/pgx/v5"
)

const reservationOperation = "registration.book"

type eventCapacityPeek struct {
	CapacityType string
	Capacity     int
	Version      int64
}

// peekEventCapacity reads the event capacity_type and capacity outside any
// transaction. It is only consulted to decide whether the Redis reservation
// gate applies; PostgreSQL still locks and rechecks the event row inside the
// booking transaction (PH2-20 §3, §7).
func (s *Service) peekEventCapacity(ctx context.Context, eventID string) (eventCapacityPeek, error) {
	var peek eventCapacityPeek
	var capacity *int
	var version int64
	err := s.db.QueryRow(ctx, `SELECT capacity_type, capacity, version FROM events WHERE event_id = $1`, eventID).
		Scan(&peek.CapacityType, &capacity, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return eventCapacityPeek{}, notFound("event not found")
	}
	if err != nil {
		return eventCapacityPeek{}, err
	}
	if capacity != nil {
		peek.Capacity = *capacity
	}
	peek.Version = version
	return peek, nil
}

// preadmitBooking runs the Redis reservation gate before the booking
// transaction. It returns the Hold and the HMAC-derived idempotency hash that
// the caller must pass to finalizeReservation after the DB outcome is known.
// When the gate is disabled, the event is unlimited, or peek fails to find
// the event, the returned Hold is granted and idempotencyHash is empty (the
// caller skips gate finalization).
func (s *Service) preadmitBooking(ctx context.Context, eventID, employeeID, idempotencyKey string) (reservation.Hold, string, error) {
	if !s.reservationGate.Enabled() {
		return reservation.Hold{Outcome: reservation.OutcomeGranted}, "", nil
	}
	peek, err := s.peekEventCapacity(ctx, eventID)
	if err != nil {
		// Event missing or DB error: let the booking tx surface the real error.
		return reservation.Hold{Outcome: reservation.OutcomeGranted}, "", nil
	}
	if peek.CapacityType != CapacityTypeLimited {
		return reservation.Hold{Outcome: reservation.OutcomeGranted}, "", nil
	}
	idempotencyHash := reservation.Hash(s.reservationSecret, reservationOperation, eventID, employeeID, idempotencyKey)
	actorHash := reservation.ActorHash(s.reservationSecret, employeeID)
	probe := s.remainingCapacityProbe(eventID, peek)
	hold, err := s.reservationGate.Reserve(ctx, eventID, idempotencyHash, actorHash, probe)
	if err != nil {
		if errors.Is(err, reservation.ErrUnavailable) {
			return reservation.Hold{}, "", conflict("reservation gate unavailable; retry shortly")
		}
		return reservation.Hold{}, "", err
	}
	return hold, idempotencyHash, nil
}

// remainingCapacityProbe builds a CapacityProbe closure that lazily counts
// confirmed registrations the first time Redis needs to seed its advisory
// counter. The probe is only invoked once per event-key lifetime; subsequent
// reservations hit the cached counter directly.
func (s *Service) remainingCapacityProbe(eventID string, peek eventCapacityPeek) reservation.CapacityProbe {
	return func(ctx context.Context) (int, int64, error) {
		var confirmed int
		err := s.db.QueryRow(ctx,
			`SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, eventID,
		).Scan(&confirmed)
		if err != nil {
			return 0, peek.Version, err
		}
		remaining := peek.Capacity - confirmed
		if remaining < 0 {
			remaining = 0
		}
		return remaining, peek.Version, nil
	}
}

// finalizeReservation releases or confirms the gate hold based on the DB
// outcome. Confirmed limited-event bookings call Confirm (counter NOT
// returned). Every other outcome — waitlist, duplicate, rollback, error —
// calls Release so the slot returns to the advisory pool. Errors are
// swallowed: TTL + the PH2-23 compensation worker are the safety net.
func (s *Service) finalizeReservation(ctx context.Context, eventID, idempotencyHash string, confirmed bool) {
	if idempotencyHash == "" {
		return
	}
	if confirmed {
		if err := s.reservationGate.Confirm(ctx, eventID, idempotencyHash); err != nil {
			s.logger.Warn("reservation confirm failed", "event_id", eventID, "error", err)
		}
		return
	}
	if err := s.reservationGate.Release(ctx, eventID, idempotencyHash); err != nil {
		s.logger.Warn("reservation release failed", "event_id", eventID, "error", err)
	}
}
