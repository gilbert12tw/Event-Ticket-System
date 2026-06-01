package ticketing

import (
	"context"
	"errors"
	"time"

	"event-ticket-system/internal/reservation"

	"github.com/jackc/pgx/v5"
)

// BookingByHash implements reservation.BookingLookup. The PH2-23 compensation
// worker calls this to map a Redis pending hold (event_id, idempotency_hash)
// back to PostgreSQL truth: completed_at IS NULL means still in flight (skip
// — the application owns reconciliation); completed + confirmed means the
// slot is owned (drop the Redis hold without returning the slot); completed
// + non-confirmed means waitlist/cancel (release and return the slot).
//
// The query uses the partial unique index added by the column migration
// (idx_booking_idempotency_results_event_hash), so it is O(1) even on a
// large idempotency table.
func (s *Service) BookingByHash(ctx context.Context, eventID, idempotencyHash string) (reservation.BookingStatus, error) {
	if idempotencyHash == "" {
		// Defensive: the gate never emits an empty hash; if it ever did the
		// partial index would return nothing anyway.
		return reservation.BookingStatus{}, nil
	}
	var registrationStatus string
	var completedAt *time.Time
	err := s.db.QueryRow(ctx,
		`SELECT registration_status, completed_at
		   FROM booking_idempotency_results
		  WHERE event_id = $1 AND idempotency_hash = $2`,
		eventID, idempotencyHash,
	).Scan(&registrationStatus, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return reservation.BookingStatus{}, nil
	}
	if err != nil {
		return reservation.BookingStatus{}, err
	}
	return reservation.BookingStatus{
		Found:     true,
		Completed: completedAt != nil,
		Confirmed: registrationStatus == RegistrationConfirmed,
	}, nil
}

// RemainingCapacity is the PostgreSQL-derived remaining-seat count for an
// event. It is the same answer the gate's CapacityProbe gives, but reused
// here so the compensator's drift-cap step can clamp the advisory counter
// without re-implementing the truth-side math. Unlimited events return 0
// (they never produce reservation holds anyway).
func (s *Service) RemainingCapacity(ctx context.Context, eventID string) (int, error) {
	var capacityType string
	var capacity *int
	err := s.db.QueryRow(ctx,
		`SELECT capacity_type, capacity FROM events WHERE event_id = $1`,
		eventID,
	).Scan(&capacityType, &capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if capacityType != CapacityTypeLimited || capacity == nil {
		return 0, nil
	}
	var confirmed int
	if err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`,
		eventID,
	).Scan(&confirmed); err != nil {
		return 0, err
	}
	remaining := *capacity - confirmed
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}
