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
	cancelID := strings.TrimSpace(req.IdempotencyKey)
	if cancelID == "" {
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
	isEmployeeCancel := actor.Role == RoleEmployee
	if isEmployeeCancel {
		if actor.ID != reg.EmployeeID {
			return BookingResponse{}, forbidden("employees may only cancel their own registrations")
		}
	} else if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return BookingResponse{}, err
	}
	if reg.Status == RegistrationCancelled {
		if reg.CancelKey == cancelID {
			ticket, err := s.findTicketByRegistrationTx(ctx, tx, reg.RegistrationID)
			if err != nil {
				return BookingResponse{}, err
			}
			remaining, err := s.remainingForResponseTx(ctx, tx, event)
			if err != nil {
				return BookingResponse{}, err
			}
			return BookingResponse{Registration: reg, Ticket: sanitizeTicket(ticket), RemainingCapacity: remaining, Message: "registration already cancelled"}, tx.Commit(ctx)
		}
	}
	if isEmployeeCancel {
		if s.now().After(event.RegistrationClose) {
			return BookingResponse{}, conflict("registration window is closed; contact an activity admin")
		}
	} else if strings.TrimSpace(req.Reason) == "" {
		return BookingResponse{}, badRequest("reason is required for admin exception cancellation")
	}
	if reg.Status != RegistrationConfirmed && reg.Status != RegistrationWaitlisted {
		return BookingResponse{}, conflict("registration cannot be cancelled")
	}
	wasConfirmed := reg.Status == RegistrationConfirmed
	cancelledAt := s.now()
	_, err = tx.Exec(ctx, `UPDATE registrations SET status = 'cancelled', cancel_idempotency_key = $1, cancel_reason = $2, cancelled_at = $3
		WHERE registration_id = $4`, cancelID, req.Reason, cancelledAt, registrationID)
	if err != nil {
		return BookingResponse{}, err
	}
	reg.Status = RegistrationCancelled
	reg.CancelKey = cancelID
	reg.CancelReason = req.Reason
	reg.CancelledAt = cancelledAt
	if _, err := tx.Exec(ctx, `UPDATE tickets SET status = 'revoked', revoked_reason = $1 WHERE registration_id = $2 AND status = 'active'`, "registration cancelled", registrationID); err != nil {
		return BookingResponse{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return BookingResponse{}, err
	}
	eventContext := eventContextMetadata(event)
	cancellationMetadata := mergeMetadata(eventContext, map[string]interface{}{
		"registration_id": registrationID,
		"reason":          req.Reason,
	})
	if err := insertAudit(ctx, tx, auditID, actor, "registration.cancelled", "registration", registrationID, cancellationMetadata); err != nil {
		return BookingResponse{}, err
	}
	cancellationPayload := mergeMetadata(eventContext, map[string]interface{}{
		"registration_id": registrationID,
		"employee_id":     reg.EmployeeID,
		"reason":          req.Reason,
	})
	if err := insertOutbox(ctx, tx, "registration.cancelled", registrationID, cancellationPayload); err != nil {
		return BookingResponse{}, err
	}
	ticket, err := s.findTicketByRegistrationTx(ctx, tx, reg.RegistrationID)
	if err != nil {
		return BookingResponse{}, err
	}

	remaining := 0
	if event.CapacityType == CapacityTypeLimited {
		capacity, err := limitedCapacity(event)
		if err != nil {
			return BookingResponse{}, err
		}
		if wasConfirmed {
			promotion, err := s.promoteWaitlistedRegistrationTx(ctx, tx, actor, eventID, capacity, rule)
			if err != nil {
				return BookingResponse{}, err
			}
			remaining = promotion.RemainingCapacity
		} else {
			remaining, err = s.remainingCapacityTx(ctx, tx, eventID, capacity)
			if err != nil {
				return BookingResponse{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return BookingResponse{}, err
	}
	return BookingResponse{Registration: reg, Ticket: sanitizeTicket(ticket), RemainingCapacity: remaining, Message: "registration cancelled"}, nil
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
