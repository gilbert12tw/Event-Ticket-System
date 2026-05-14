package ticketing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Service) findRegistrationByIdempotencyKey(ctx context.Context, tx pgx.Tx, key string, expectedEventID string, expectedEmployeeID string) (BookingResponse, bool, error) {
	var reg Registration
	err := tx.QueryRow(ctx, `SELECT registration_id, event_id, employee_id, status, idempotency_key, COALESCE(cancel_idempotency_key, ''),
			COALESCE(cancelled_at, '0001-01-01 00:00:00+00'::timestamptz), cancel_reason, created_at
		FROM registrations WHERE idempotency_key = $1`, key).
		Scan(&reg.RegistrationID, &reg.EventID, &reg.EmployeeID, &reg.Status, &reg.IdempotencyKey, &reg.CancelKey, &reg.CancelledAt, &reg.CancelReason, &reg.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingResponse{}, false, nil
	}
	if err != nil {
		return BookingResponse{}, false, err
	}
	if reg.EventID != expectedEventID || reg.EmployeeID != expectedEmployeeID {
		return BookingResponse{}, false, conflict("idempotency key belongs to a different booking request")
	}
	ticket, err := s.findTicketByRegistrationTx(ctx, tx, reg.RegistrationID)
	if err != nil {
		return BookingResponse{}, false, err
	}
	event, _, err := s.lockEventWithRule(ctx, tx, reg.EventID)
	if err != nil {
		return BookingResponse{}, false, err
	}
	capacity, err := limitedCapacity(event)
	if err != nil {
		return BookingResponse{}, false, err
	}
	remaining, err := s.remainingCapacityTx(ctx, tx, reg.EventID, capacity)
	if err != nil {
		return BookingResponse{}, false, err
	}
	return BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remaining, Message: bookingMessage(reg.Status)}, true, nil
}

func (s *Service) findRegistrationByEmployee(ctx context.Context, eventID string, employeeID string) (Registration, *Ticket, bool, error) {
	var reg Registration
	err := s.db.QueryRow(ctx, `SELECT registration_id, event_id, employee_id, status, idempotency_key, COALESCE(cancel_idempotency_key, ''),
			COALESCE(cancelled_at, '0001-01-01 00:00:00+00'::timestamptz), cancel_reason, created_at
		FROM registrations WHERE event_id = $1 AND employee_id = $2`, eventID, employeeID).
		Scan(&reg.RegistrationID, &reg.EventID, &reg.EmployeeID, &reg.Status, &reg.IdempotencyKey, &reg.CancelKey, &reg.CancelledAt, &reg.CancelReason, &reg.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Registration{}, nil, false, nil
	}
	if err != nil {
		return Registration{}, nil, false, err
	}
	ticket, err := s.findTicketByRegistration(ctx, reg.RegistrationID)
	if err != nil {
		return Registration{}, nil, false, err
	}
	return reg, ticket, true, nil
}

func (s *Service) findRegistrationByEmployeeTx(ctx context.Context, tx pgx.Tx, eventID string, employeeID string) (Registration, *Ticket, bool, error) {
	var reg Registration
	err := tx.QueryRow(ctx, `SELECT registration_id, event_id, employee_id, status, idempotency_key, COALESCE(cancel_idempotency_key, ''),
			COALESCE(cancelled_at, '0001-01-01 00:00:00+00'::timestamptz), cancel_reason, created_at
		FROM registrations WHERE event_id = $1 AND employee_id = $2`, eventID, employeeID).
		Scan(&reg.RegistrationID, &reg.EventID, &reg.EmployeeID, &reg.Status, &reg.IdempotencyKey, &reg.CancelKey, &reg.CancelledAt, &reg.CancelReason, &reg.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Registration{}, nil, false, nil
	}
	if err != nil {
		return Registration{}, nil, false, err
	}
	ticket, err := s.findTicketByRegistrationTx(ctx, tx, reg.RegistrationID)
	if err != nil {
		return Registration{}, nil, false, err
	}
	return reg, ticket, true, nil
}

func (s *Service) lockRegistrationTx(ctx context.Context, tx pgx.Tx, registrationID string) (Registration, error) {
	var reg Registration
	err := tx.QueryRow(ctx, `SELECT registration_id, event_id, employee_id, status, idempotency_key, COALESCE(cancel_idempotency_key, ''),
			COALESCE(cancelled_at, '0001-01-01 00:00:00+00'::timestamptz), cancel_reason, created_at
		FROM registrations WHERE registration_id = $1 FOR UPDATE`, registrationID).
		Scan(&reg.RegistrationID, &reg.EventID, &reg.EmployeeID, &reg.Status, &reg.IdempotencyKey, &reg.CancelKey, &reg.CancelledAt, &reg.CancelReason, &reg.CreatedAt)
	if err == pgx.ErrNoRows {
		return Registration{}, notFound("registration not found")
	}
	return reg, err
}

func (s *Service) nextEligibleWaitlistedTx(ctx context.Context, tx pgx.Tx, eventID string, rule EligibilityRule) (Registration, Employee, bool, error) {
	rows, err := tx.Query(ctx, `SELECT
			r.registration_id, r.event_id, r.employee_id, r.status, r.idempotency_key, COALESCE(r.cancel_idempotency_key, ''),
			COALESCE(r.cancelled_at, '0001-01-01 00:00:00+00'::timestamptz), r.cancel_reason, r.created_at,
			e.full_name, e.department, e.site, e.job_grade, e.employment_status
		FROM registrations r
		JOIN employees e ON e.employee_id = r.employee_id
		WHERE r.event_id = $1 AND r.status = 'waitlisted'
		ORDER BY r.created_at ASC
		FOR UPDATE OF r`, eventID)
	if err != nil {
		return Registration{}, Employee{}, false, err
	}
	defer rows.Close()

	for rows.Next() {
		var reg Registration
		var employee Employee
		if err := rows.Scan(
			&reg.RegistrationID, &reg.EventID, &reg.EmployeeID, &reg.Status, &reg.IdempotencyKey, &reg.CancelKey, &reg.CancelledAt, &reg.CancelReason, &reg.CreatedAt,
			&employee.FullName, &employee.Department, &employee.Site, &employee.JobGrade, &employee.EmploymentStatus,
		); err != nil {
			return Registration{}, Employee{}, false, err
		}
		employee.EmployeeID = reg.EmployeeID
		if eligible, _ := EvaluateEligibility(employee, rule); eligible {
			return reg, employee, true, nil
		}
	}
	return Registration{}, Employee{}, false, rows.Err()
}

func (s *Service) promoteWaitlistedRegistrationTx(ctx context.Context, tx pgx.Tx, actor Actor, eventID string, eventCapacity int, rule EligibilityRule) (*BookingResponse, error) {
	remaining, err := s.remainingCapacityTx(ctx, tx, eventID, eventCapacity)
	if err != nil {
		return nil, err
	}
	if remaining <= 0 {
		if err := insertPromotionNoopAuditTx(ctx, tx, actor, eventID, "no_capacity_available", 0); err != nil {
			return nil, err
		}
		return &BookingResponse{RemainingCapacity: 0, Message: "no capacity available"}, nil
	}
	reg, employee, found, err := s.nextEligibleWaitlistedTx(ctx, tx, eventID, rule)
	if err != nil {
		return nil, err
	}
	if !found {
		if err := insertPromotionNoopAuditTx(ctx, tx, actor, eventID, "no_eligible_waitlist_registrations", remaining); err != nil {
			return nil, err
		}
		return &BookingResponse{RemainingCapacity: remaining, Message: "no eligible waitlist registrations"}, nil
	}

	_, err = tx.Exec(ctx, `UPDATE registrations SET status = 'confirmed' WHERE registration_id = $1`, reg.RegistrationID)
	if err != nil {
		return nil, err
	}
	reg.Status = RegistrationConfirmed

	ticket, err := s.createTicketTx(ctx, tx, reg, employee)
	if err != nil {
		return nil, err
	}
	if err := insertTicketIssuedAuditTx(ctx, tx, actor, ticket); err != nil {
		return nil, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return nil, err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "waitlist.promoted", "registration", reg.RegistrationID, map[string]interface{}{"event_id": eventID, "employee_id": reg.EmployeeID}); err != nil {
		return nil, err
	}
	if err := insertOutbox(ctx, tx, "waitlist.promoted", reg.RegistrationID, map[string]interface{}{"registration_id": reg.RegistrationID, "event_id": eventID, "employee_id": reg.EmployeeID}); err != nil {
		return nil, err
	}

	remaining, err = s.remainingCapacityTx(ctx, tx, eventID, eventCapacity)
	if err != nil {
		return nil, err
	}
	return &BookingResponse{Registration: reg, Ticket: &ticket, RemainingCapacity: remaining, Message: "waitlist registration promoted"}, nil
}

func insertPromotionNoopAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, eventID string, reason string, remaining int) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	return insertAudit(ctx, tx, auditID, actor, "waitlist.promotion_noop", "event", eventID, map[string]interface{}{
		"event_id":           eventID,
		"reason":             reason,
		"remaining_capacity": remaining,
	})
}

func (s *Service) confirmedCountTx(ctx context.Context, tx pgx.Tx, eventID string) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, eventID).Scan(&count)
	return count, err
}

func (s *Service) remainingCapacityTx(ctx context.Context, tx pgx.Tx, eventID string, capacity int) (int, error) {
	confirmed, err := s.confirmedCountTx(ctx, tx, eventID)
	if err != nil {
		return 0, err
	}
	return max(capacity-confirmed, 0), nil
}

func bookingMessage(status string) string {
	switch status {
	case RegistrationConfirmed:
		return "booking confirmed and ticket issued"
	case RegistrationWaitlisted:
		return "event is full; employee joined waitlist"
	default:
		return fmt.Sprintf("registration status: %s", status)
	}
}
