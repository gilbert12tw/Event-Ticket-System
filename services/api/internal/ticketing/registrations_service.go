package ticketing

import (
	"context"
	"errors"
	"strings"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Book(ctx context.Context, actor Actor, eventID string, req BookingRequest) (BookingResponse, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return BookingResponse{}, err
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return BookingResponse{}, badRequest("idempotency_key is required")
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
		return existing, tx.Commit(ctx)
	}

	event, rule, err := s.lockEventWithRule(ctx, tx, eventID)
	if err != nil {
		return BookingResponse{}, err
	}
	if event.Status != EventStatusPublished {
		return BookingResponse{}, conflict("event is not open for booking")
	}
	now := s.now()
	if now.Before(event.RegistrationStart) || now.After(event.RegistrationClose) {
		return BookingResponse{}, conflict("registration window is closed")
	}

	employee, err := s.getEmployeeTx(ctx, tx, employeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingResponse{}, notFound("employee not found")
	}
	if err != nil {
		return BookingResponse{}, err
	}
	eligible, reason := EvaluateEligibility(employee, rule)
	if !eligible {
		return BookingResponse{}, forbidden(reason)
	}

	if reg, ticket, found, err := s.findRegistrationByEmployeeTx(ctx, tx, eventID, employeeID); err != nil {
		return BookingResponse{}, err
	} else if found {
		remaining, err := s.remainingCapacityTx(ctx, tx, eventID, event.Capacity)
		if err != nil {
			return BookingResponse{}, err
		}
		return BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remaining, Message: bookingMessage(reg.Status)}, tx.Commit(ctx)
	}

	confirmedCount, err := s.confirmedCountTx(ctx, tx, eventID)
	if err != nil {
		return BookingResponse{}, err
	}
	status := RegistrationWaitlisted
	if confirmedCount < event.Capacity {
		status = RegistrationConfirmed
	}

	regID, err := newID("reg")
	if err != nil {
		return BookingResponse{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ($1,$2,$3,$4,$5)`, regID, eventID, employeeID, status, req.IdempotencyKey)
	if err != nil {
		if isUniqueViolation(err) {
			existing, found, findErr := s.findRegistrationByIdempotencyKey(ctx, tx, req.IdempotencyKey, eventID, employeeID)
			if findErr != nil {
				return BookingResponse{}, findErr
			}
			if found {
				return existing, tx.Commit(ctx)
			}
		}
		return BookingResponse{}, err
	}

	reg := Registration{RegistrationID: regID, EventID: eventID, EmployeeID: employeeID, Status: status, IdempotencyKey: req.IdempotencyKey, CreatedAt: now}
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
	if err := insertAudit(ctx, tx, auditID, actor, action, "registration", regID, map[string]interface{}{"event_id": eventID, "status": status}); err != nil {
		return BookingResponse{}, err
	}
	if err := insertOutbox(ctx, tx, action, regID, map[string]interface{}{"registration_id": regID, "event_id": eventID, "employee_id": employeeID}); err != nil {
		return BookingResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookingResponse{}, err
	}

	remaining := max(event.Capacity-confirmedCount-1, 0)
	if status == RegistrationWaitlisted {
		remaining = 0
	}
	s.logger.Info("booking completed", "trace_id", traceid.FromContext(ctx), "action", action, "status", status, "event_id", eventID, "actor_role", actor.Role)
	return BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remaining, Message: bookingMessage(status)}, nil
}
