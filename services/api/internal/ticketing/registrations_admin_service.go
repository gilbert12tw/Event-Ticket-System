package ticketing

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ListRegistrations(ctx context.Context, actor Actor, eventID string) ([]RegistrationDetail, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT
			r.registration_id, r.event_id, r.employee_id, r.status, r.idempotency_key, COALESCE(r.cancel_idempotency_key, ''),
			COALESCE(r.cancelled_at, '0001-01-01 00:00:00+00'::timestamptz), r.cancel_reason, r.family_count, r.created_at,
			e.full_name,
			COALESCE(t.ticket_id, ''), COALESCE(t.registration_id, ''), COALESCE(t.event_id, ''), COALESCE(t.employee_id, ''),
			COALESCE(t.status, ''), COALESCE(t.sequence_number, 0),
			COALESCE(t.expires_at, t.issued_at + interval '24 hours', r.created_at), COALESCE(t.revoked_reason, ''), COALESCE(t.issued_at, r.created_at)
		FROM registrations r
		JOIN employees e ON e.employee_id = r.employee_id
		LEFT JOIN tickets t ON t.registration_id = r.registration_id
		WHERE r.event_id = $1
		ORDER BY r.created_at ASC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var details []RegistrationDetail
	for rows.Next() {
		var detail RegistrationDetail
		var ticket Ticket
		var ticketID string
		err := rows.Scan(
			&detail.RegistrationID, &detail.EventID, &detail.EmployeeID, &detail.Status, &detail.IdempotencyKey, &detail.CancelKey, &detail.CancelledAt, &detail.CancelReason, &detail.FamilyCount, &detail.CreatedAt,
			&detail.EmployeeName,
			&ticketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber, &ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt,
		)
		if err != nil {
			return nil, err
		}
		if ticketID != "" {
			ticket.TicketID = ticketID
			ticket.FamilyCount = detail.FamilyCount
			ticket.NonTransferable = true
			detail.Ticket = sanitizeTicket(&ticket)
		}
		details = append(details, detail)
	}
	return details, rows.Err()
}

func (s *Service) CancelRegistration(ctx context.Context, actor Actor, eventID string, registrationID string, req CancelRegistrationRequest) (BookingResponse, error) {
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.IdempotencyKey == "" {
		return BookingResponse{}, badRequest("idempotency_key is required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BookingResponse{}, err
	}
	defer rollback(ctx, tx)

	event, rule, err := s.lockEventWithRule(ctx, tx, eventID)
	if err != nil {
		return BookingResponse{}, err
	}
	reg, err := s.lockRegistrationTx(ctx, tx, registrationID)
	if err != nil {
		return BookingResponse{}, err
	}
	if reg.EventID != eventID {
		return BookingResponse{}, notFound("registration not found")
	}
	if err := s.validateCancellationActorAccess(actor, event, reg); err != nil {
		return BookingResponse{}, err
	}
	if response, found, err := s.cancelledRegistrationResponseTx(ctx, tx, event, reg, req.IdempotencyKey); err != nil || found {
		return response, err
	}
	if err := s.validateNewCancellationRequest(actor, req); err != nil {
		return BookingResponse{}, err
	}
	wasConfirmed := reg.Status == RegistrationConfirmed
	if err := s.applyRegistrationCancellationTx(ctx, tx, actor, event, &reg, registrationID, req); err != nil {
		return BookingResponse{}, err
	}
	ticket, err := s.findTicketByRegistrationTx(ctx, tx, reg.RegistrationID)
	if err != nil {
		return BookingResponse{}, err
	}
	remaining, err := s.remainingAfterCancellationTx(ctx, tx, actor, event, rule, wasConfirmed)
	if err != nil {
		return BookingResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookingResponse{}, err
	}
	return BookingResponse{Registration: reg, Ticket: sanitizeTicket(ticket), RemainingCapacity: remaining, Message: "registration cancelled"}, nil
}

func (s *Service) validateCancellationActorAccess(actor Actor, event Event, reg Registration) error {
	if actor.Role == RoleEmployee {
		if actor.ID != reg.EmployeeID {
			return forbidden("employees may only cancel their own registrations")
		}
		if s.now().After(event.RegistrationClose) {
			return conflict("registration window is closed; contact an activity admin")
		}
		return nil
	}
	return requireRole(actor, RoleActivityAdmin)
}

func (s *Service) validateNewCancellationRequest(actor Actor, req CancelRegistrationRequest) error {
	if actor.Role == RoleEmployee {
		return nil
	}
	if strings.TrimSpace(req.Reason) == "" {
		return badRequest("reason is required for admin exception cancellation")
	}
	return nil
}

func (s *Service) cancelledRegistrationResponseTx(ctx context.Context, tx pgx.Tx, event Event, reg Registration, cancelID string) (BookingResponse, bool, error) {
	if reg.Status != RegistrationCancelled {
		return BookingResponse{}, false, nil
	}
	if reg.CancelKey != cancelID {
		return BookingResponse{}, true, conflict("registration cannot be cancelled")
	}
	ticket, err := s.findTicketByRegistrationTx(ctx, tx, reg.RegistrationID)
	if err != nil {
		return BookingResponse{}, true, err
	}
	remaining, err := s.remainingForResponseTx(ctx, tx, event)
	if err != nil {
		return BookingResponse{}, true, err
	}
	return BookingResponse{Registration: reg, Ticket: sanitizeTicket(ticket), RemainingCapacity: remaining, Message: "registration already cancelled"}, true, tx.Commit(ctx)
}

func (s *Service) applyRegistrationCancellationTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, reg *Registration, registrationID string, req CancelRegistrationRequest) error {
	if reg.Status != RegistrationConfirmed && reg.Status != RegistrationWaitlisted {
		return conflict("registration cannot be cancelled")
	}
	wasConfirmed := reg.Status == RegistrationConfirmed
	cancelledAt := s.now()
	_, err := tx.Exec(ctx, `UPDATE registrations SET status = 'cancelled', cancel_idempotency_key = $1, cancel_reason = $2, cancelled_at = $3
		WHERE registration_id = $4`, req.IdempotencyKey, req.Reason, cancelledAt, registrationID)
	if err != nil {
		return err
	}
	reg.Status = RegistrationCancelled
	reg.CancelKey = req.IdempotencyKey
	reg.CancelReason = req.Reason
	reg.CancelledAt = cancelledAt
	if wasConfirmed {
		if err := s.createBookingBanTx(ctx, tx, actor, event.EventID, reg.EmployeeID, registrationID, req.Reason); err != nil {
			return err
		}
	}
	if err := revokeRegistrationTicketTx(ctx, tx, registrationID); err != nil {
		return err
	}
	return insertRegistrationCancellationEffectsTx(ctx, tx, actor, event, *reg, registrationID, req.Reason)
}

func revokeRegistrationTicketTx(ctx context.Context, tx pgx.Tx, registrationID string) error {
	_, err := tx.Exec(ctx, `UPDATE tickets SET status = 'revoked', revoked_reason = $1 WHERE registration_id = $2 AND status = 'active'`, "registration cancelled", registrationID)
	return err
}

func insertRegistrationCancellationEffectsTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, reg Registration, registrationID string, reason string) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	eventContext := eventContextMetadata(event)
	cancellationMetadata := mergeMetadata(eventContext, map[string]interface{}{
		"registration_id": registrationID,
		"reason":          reason,
	})
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "registration.cancelled", "registration", registrationID, cancellationMetadata)); err != nil {
		return err
	}
	cancellationPayload := mergeMetadata(eventContext, map[string]interface{}{
		"registration_id": registrationID,
		"employee_id":     reg.EmployeeID,
		"reason":          reason,
	})
	return insertOutbox(ctx, tx, "registration.cancelled", registrationID, cancellationPayload)
}

func (s *Service) remainingAfterCancellationTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, rule EligibilityRule, wasConfirmed bool) (int, error) {
	if event.CapacityType != CapacityTypeLimited {
		return 0, nil
	}
	capacity, err := limitedCapacity(event)
	if err != nil {
		return 0, err
	}
	if wasConfirmed {
		promotion, err := s.promoteWaitlistedRegistrationTx(ctx, tx, actor, event.EventID, capacity, rule)
		return promotion.RemainingCapacity, err
	}
	return s.remainingCapacityTx(ctx, tx, event.EventID, capacity)
}

func (s *Service) CancelMyRegistration(ctx context.Context, actor Actor, registrationID string, req CancelRegistrationRequest) (BookingResponse, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return BookingResponse{}, err
	}
	var eventID string
	if err := s.db.QueryRow(ctx, `SELECT event_id FROM registrations WHERE registration_id = $1`, registrationID).Scan(&eventID); err != nil {
		if err == pgx.ErrNoRows {
			return BookingResponse{}, notFound("registration not found")
		}
		return BookingResponse{}, err
	}
	return s.CancelRegistration(ctx, actor, eventID, registrationID, req)
}

func (s *Service) PromoteWaitlist(ctx context.Context, actor Actor, eventID string) (PromoteWaitlistResponse, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return PromoteWaitlistResponse{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PromoteWaitlistResponse{}, err
	}
	defer rollback(ctx, tx)

	event, rule, err := s.lockEventWithRule(ctx, tx, eventID)
	if err != nil {
		return PromoteWaitlistResponse{}, err
	}
	capacity, err := limitedCapacity(event)
	if err != nil {
		return PromoteWaitlistResponse{}, err
	}
	promotion, err := s.promoteWaitlistedRegistrationTx(ctx, tx, actor, eventID, capacity, rule)
	if err != nil {
		return PromoteWaitlistResponse{}, err
	}
	if promotion.Message != "waitlist registration promoted" {
		if err := tx.Commit(ctx); err != nil {
			return PromoteWaitlistResponse{}, err
		}
		return PromoteWaitlistResponse{RemainingCapacity: promotion.RemainingCapacity, Message: promotion.Message}, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return PromoteWaitlistResponse{}, err
	}
	result := BookingResponse{Registration: promotion.Registration, Ticket: sanitizeTicket(promotion.Ticket), RemainingCapacity: promotion.RemainingCapacity, Message: promotion.Message}
	return PromoteWaitlistResponse{Promoted: &result, RemainingCapacity: promotion.RemainingCapacity, Message: "waitlist registration promoted"}, nil
}
