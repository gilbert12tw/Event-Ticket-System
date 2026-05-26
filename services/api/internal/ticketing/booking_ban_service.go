package ticketing

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// checkBookingBanTx queries whether there is an active ban for this employee on this event.
// It is intended to be called within the booking transaction, before the UNIQUE constraint
// check, to provide a clear business error.
func (s *Service) checkBookingBanTx(ctx context.Context, tx pgx.Tx, eventID, employeeID string) error {
	var banID string
	err := tx.QueryRow(ctx, `SELECT ban_id FROM booking_bans WHERE event_id = $1 AND employee_id = $2 AND lifted_at IS NULL`, eventID, employeeID).Scan(&banID)
	if err == nil {
		return bookingBanned("you are banned from booking this event")
	}
	if err == pgx.ErrNoRows {
		return nil
	}
	return err
}

// createBookingBanTx inserts a per-event ban for an employee. It is invoked when a confirmed
// booking is cancelled. With the partial unique index on (event_id, employee_id) WHERE lifted_at
// IS NULL, a lifted row is not in the index — so a new INSERT always succeeds and creates a
// fresh active ban row without conflicting with historical lifted rows.
//
// ErrNoRows means RETURNING found no row because an active ban already exists (ON CONFLICT DO
// NOTHING). That is an idempotent repeat of an already-banned cancel — skip the audit entry.
// Any other error is a genuine DB failure and is returned directly.
func (s *Service) createBookingBanTx(ctx context.Context, tx pgx.Tx, actor Actor, eventID, employeeID, registrationID, reason string) error {
	banID, err := newID("ban")
	if err != nil {
		return err
	}
	var activeBanID string
	err = tx.QueryRow(ctx, `
		INSERT INTO booking_bans (ban_id, event_id, employee_id, registration_id, reason, banned_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id, employee_id) WHERE lifted_at IS NULL DO NOTHING
		RETURNING ban_id`,
		banID, eventID, employeeID, registrationID, reason, s.now()).Scan(&activeBanID)

	if err == pgx.ErrNoRows {
		// Active ban already exists for this pair — idempotent cancel, skip audit.
		return nil
	}
	if err != nil {
		return err
	}

	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	return insertAudit(ctx, tx, auditID, actor, "booking.ban_created", "booking_ban", activeBanID, map[string]interface{}{
		"event_id":        eventID,
		"employee_id":     employeeID,
		"registration_id": registrationID,
		"reason":          reason,
	})
}

// LiftBookingBan resolves an active per-event ban, allowing the employee to book the event again.
func (s *Service) LiftBookingBan(ctx context.Context, actor Actor, eventID, employeeID string) error {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleSystemAdmin); err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)

	var banID string
	err = tx.QueryRow(ctx, `
		UPDATE booking_bans
		SET lifted_at = $1, lifted_by = $2
		WHERE event_id = $3 AND employee_id = $4 AND lifted_at IS NULL
		RETURNING ban_id`, s.now(), actor.ID, eventID, employeeID).Scan(&banID)

	if err == pgx.ErrNoRows {
		return notFound("no active ban found for this employee and event")
	}
	if err != nil {
		return err
	}

	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "booking.ban_lifted", "booking_ban", banID, map[string]interface{}{
		"event_id":    eventID,
		"employee_id": employeeID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
