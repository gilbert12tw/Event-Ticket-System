package ticketing

import (
	"context"
	"errors"
	"strings"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

const maxFamilyCount = 10

func (s *Service) Book(ctx context.Context, actor Actor, eventID string, req BookingRequest) (BookingResponse, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return BookingResponse{}, err
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return BookingResponse{}, badRequest("idempotency_key is required")
	}
	if req.FamilyCount < 0 || req.FamilyCount > maxFamilyCount {
		return BookingResponse{}, badRequest("family_count must be between 0 and 10")
	}
	employeeID := strings.TrimSpace(req.EmployeeID)
	if employeeID == "" {
		employeeID = actor.ID
	}
	if employeeID == "" {
		return BookingResponse{}, badRequest("employee_id is required")
	}
	if actor.ID != "" && actor.ID != employeeID {
		return BookingResponse{}, forbidden("employees may only book for themselves")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BookingResponse{}, err
	}
	defer rollback(ctx, tx)

	if existing, found, err := s.findRegistrationByIdempotencyKey(ctx, tx, req.IdempotencyKey, eventID, employeeID); err != nil {
		return BookingResponse{}, err
	} else if found {
		if existing.Registration.FamilyCount != req.FamilyCount {
			return BookingResponse{}, conflict("idempotency key belongs to a different booking request")
		}
		return existing, tx.Commit(ctx)
	}

	event, rule, err := s.lockEventWithRule(ctx, tx, eventID)
	if err != nil {
		return BookingResponse{}, err
	}

	// checkBookingBanTx is called after lockEventWithRule so that the ban row
	// written by a concurrent cancel transaction is visible under the same lock;
	// a check before the lock would be a stale-read race.
	if err := s.checkBookingBanTx(ctx, tx, eventID, employeeID); err != nil {
		return BookingResponse{}, err
	}
	if event.Status != EventStatusPublished {
		return BookingResponse{}, conflict("event is not open for booking")
	}
	now := s.now()
	if now.Before(event.RegistrationStart) || now.After(event.RegistrationClose) {
		return BookingResponse{}, conflict("registration window is closed")
	}
	if event.CapacityType == CapacityTypeLimited && req.FamilyCount > 0 {
		return BookingResponse{}, badRequest("limited events cannot accept family attendees")
	}
	if event.CapacityType == CapacityTypeUnlimited && !event.AllowsFamily && req.FamilyCount > 0 {
		return BookingResponse{}, badRequest("event does not allow family attendees")
	}
	if event.CapacityType == CapacityTypeLimited {
		cooldown, found, err := s.activeNoShowCooldownTx(ctx, tx, employeeID, now)
		if err != nil {
			return BookingResponse{}, err
		}
		if found {
			return BookingResponse{}, forbidden("limited event booking blocked by no-show cooldown until " + cooldown.Format("2006-01-02"))
		}
	}

	employee, err := s.getEmployeeTx(ctx, tx, employeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingResponse{}, notFound("employee not found")
	}
	if err != nil {
		return BookingResponse{}, err
	}
	eligibilityEmployee := employee
	if actor.Claims != nil {
		eligibilityEmployee, err = employeeFromClaims(actor)
		if err != nil {
			return BookingResponse{}, forbidden(ErrMissingClaims.Error())
		}
	}
	eligible, reason := EvaluateEligibility(eligibilityEmployee, rule)
	if !eligible {
		return BookingResponse{}, forbidden(reason)
	}

	if reg, ticket, found, err := s.findRegistrationByEmployeeTx(ctx, tx, eventID, employeeID); err != nil {
		return BookingResponse{}, err
	} else if found && reg.Status != RegistrationCancelled {
		// Only return the existing row as a duplicate if it is still active
		// (confirmed or waitlisted). A cancelled registration means the ban was
		// lifted and the employee is re-booking: fall through to create a fresh
		// registration, ticket, outbox event, and booking audit.
		remaining, err := s.remainingForResponseTx(ctx, tx, event)
		if err != nil {
			return BookingResponse{}, err
		}
		return BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remaining, Message: bookingMessage(reg.Status), Duplicate: true}, tx.Commit(ctx)
	}

	status := RegistrationConfirmed
	capacity := 0
	confirmedCount := 0
	if event.CapacityType == CapacityTypeLimited {
		capacity, err = limitedCapacity(event)
		if err != nil {
			return BookingResponse{}, err
		}
		confirmedCount, err = s.confirmedCountTx(ctx, tx, eventID)
		if err != nil {
			return BookingResponse{}, err
		}
		if confirmedCount >= capacity {
			status = RegistrationWaitlisted
		}
	}

	// Use an upsert that handles two distinct cases:
	//  1. Fresh booking (no prior row)   → INSERT succeeds, RETURNING yields regID.
	//  2. Re-book after ban lift (a cancelled row exists for this event+employee) → ON
	//     CONFLICT updates the cancelled row in-place and RETURNING yields the old
	//     registration_id that was reused (we overwrite regID so downstream code is
	//     consistent).
	//  3. Duplicate active/waitlisted booking → the WHERE status='cancelled' predicate
	//     does not match, DO UPDATE fires but RETURNING yields no row (pgx.ErrNoRows),
	//     so we return a conflict error rather than silently returning the stale row.
	regID, err := newID("reg")
	if err != nil {
		return BookingResponse{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key, family_count, cancelled_at, cancel_idempotency_key, cancel_reason)
		VALUES ($1,$2,$3,$4,$5,$6, NULL, NULL, '')
		ON CONFLICT (event_id, employee_id) DO UPDATE
			SET registration_id       = EXCLUDED.registration_id,
			    status                = EXCLUDED.status,
			    idempotency_key       = EXCLUDED.idempotency_key,
			    family_count          = EXCLUDED.family_count,
			    cancelled_at          = NULL,
			    cancel_idempotency_key = NULL,
			    cancel_reason         = ''
			WHERE registrations.status = 'cancelled'
		RETURNING registration_id`,
		regID, eventID, employeeID, status, req.IdempotencyKey, req.FamilyCount).Scan(&regID)
	if err == pgx.ErrNoRows {
		// Conflict row exists but is not cancelled — duplicate active booking.
		return BookingResponse{}, conflict("employee already has an active or waitlisted registration for this event")
	}
	if err != nil {
		return BookingResponse{}, err
	}

	reg := Registration{RegistrationID: regID, EventID: eventID, EmployeeID: employeeID, Status: status, IdempotencyKey: req.IdempotencyKey, FamilyCount: req.FamilyCount, CreatedAt: now}

	var ticket *Ticket
	if status == RegistrationConfirmed {
		created, err := s.createTicketTx(ctx, tx, reg, employee)
		if err != nil {
			return BookingResponse{}, err
		}
		if err := insertTicketIssuedAuditTx(ctx, tx, actor, created); err != nil {
			return BookingResponse{}, err
		}
		ticket = &created
	}

	auditID, err := newID("aud")
	if err != nil {
		return BookingResponse{}, err
	}
	action := "booking.confirmed"
	if status == RegistrationWaitlisted {
		action = "booking.waitlisted"
	}
	if err := insertAudit(ctx, tx, auditID, actor, action, "registration", regID, map[string]interface{}{"event_id": eventID, "status": status, "capacity_type": event.CapacityType, "family_count": req.FamilyCount}); err != nil {
		return BookingResponse{}, err
	}
	if err := insertOutbox(ctx, tx, action, regID, bookingNotificationPayload(reg, event, actor)); err != nil {
		return BookingResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookingResponse{}, err
	}

	remaining := 0
	if event.CapacityType == CapacityTypeLimited && status == RegistrationConfirmed {
		remaining = max(capacity-confirmedCount-1, 0)
	}
	s.logger.Info("booking completed", "trace_id", traceid.FromContext(ctx), "action", action, "status", status, "event_id", eventID, "actor_role", actor.Role)
	return BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remaining, Message: bookingMessage(status)}, nil
}
