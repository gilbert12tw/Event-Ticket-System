package ticketing

import (
	"context"
	"errors"
	"strings"
	"time"

	"event-ticket-system/internal/reservation"
	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

const maxFamilyCount = 10

func (s *Service) Book(ctx context.Context, actor Actor, eventID string, req BookingRequest) (BookingResponse, error) {
	input, err := normalizeBookingInput(actor, eventID, req)
	if err != nil {
		return BookingResponse{}, err
	}

	if response, found, err := s.replayCompletedBooking(ctx, input.idempotencyKey, input.eventID, input.employeeID, input.familyCount); err != nil || found {
		return response, err
	}

	hold, idempotencyHash, err := s.preadmitBooking(ctx, input.eventID, input.employeeID, input.idempotencyKey, input.familyCount)
	if err != nil {
		return BookingResponse{}, err
	}
	gateConfirmed := false
	defer func() {
		s.finalizeReservation(ctx, input.eventID, idempotencyHash, gateConfirmed)
	}()

	response, action, confirmed, err := s.commitBookingTx(ctx, actor, input, hold, idempotencyHash)
	if err != nil {
		return BookingResponse{}, err
	}
	gateConfirmed = confirmed
	if action != "" {
		s.logger.Info("booking completed", "trace_id", traceid.FromContext(ctx), "action", action, "status", response.Registration.Status, "event_id", input.eventID, "actor_role", actor.Role)
	}
	return response, nil
}

func (s *Service) commitBookingTx(ctx context.Context, actor Actor, input bookingInput, hold reservation.Hold, idempotencyHash string) (BookingResponse, string, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BookingResponse{}, "", false, err
	}
	defer rollback(ctx, tx)

	if response, found, err := s.replayLockedBookingIdempotencyTx(ctx, tx, input, idempotencyHash); err != nil || found {
		return response, "", false, err
	}

	event, rule, err := s.loadBookingEventWithRuleTx(ctx, tx, input.eventID, hold)
	if err != nil {
		return BookingResponse{}, "", false, err
	}
	now := s.now()
	if err := s.validateBookingRequestTx(ctx, tx, event, input, now); err != nil {
		return BookingResponse{}, "", false, err
	}

	employee, eligibilityEmployee, err := s.bookingEmployeeForEligibilityTx(ctx, tx, actor, input.employeeID)
	if err != nil {
		return BookingResponse{}, "", false, err
	}
	if err := requireEligible(eligibilityEmployee, rule); err != nil {
		return BookingResponse{}, "", false, err
	}

	if response, found, err := s.replayExistingBookingTx(ctx, tx, event, input); err != nil || found {
		return response, "", false, err
	}

	return s.createAndCommitNewBookingTx(ctx, tx, newBookingTxInput{
		actor:    actor,
		input:    input,
		hold:     hold,
		event:    event,
		employee: employee,
		now:      now,
	})
}

func (s *Service) replayLockedBookingIdempotencyTx(ctx context.Context, tx pgx.Tx, input bookingInput, idempotencyHash string) (BookingResponse, bool, error) {
	snapshot, found, err := s.lockBookingIdempotencyResultTx(ctx, tx, input.idempotencyKey, input.eventID, input.employeeID, input.familyCount, idempotencyHash)
	if err != nil || !found {
		return BookingResponse{}, found, err
	}
	response, err := s.bookingResponseFromIdempotencyResultTx(ctx, tx, snapshot)
	if err != nil {
		return BookingResponse{}, false, err
	}
	return response, true, tx.Commit(ctx)
}

type newBookingTxInput struct {
	actor    Actor
	input    bookingInput
	hold     reservation.Hold
	event    Event
	employee Employee
	now      time.Time
}

func (s *Service) createAndCommitNewBookingTx(ctx context.Context, tx pgx.Tx, data newBookingTxInput) (BookingResponse, string, bool, error) {
	status, capacity, confirmedCount, err := s.statusForNewBookingTx(ctx, tx, data.event, data.input.eventID, data.hold)
	if err != nil {
		return BookingResponse{}, "", false, err
	}
	reg, existingResponse, err := s.insertNewBookingRegistrationTx(ctx, tx, data.input, status, data.now)
	if err != nil {
		return BookingResponse{}, "", false, err
	}
	if existingResponse != nil {
		return *existingResponse, "", false, tx.Commit(ctx)
	}

	ticket, err := s.issueTicketForConfirmedRegistrationTx(ctx, tx, data.actor, data.employee, reg)
	if err != nil {
		return BookingResponse{}, "", false, err
	}
	action := bookingActionForStatus(status)
	if err := s.recordBookingEffectsTx(ctx, tx, data.actor, data.event, reg, action); err != nil {
		return BookingResponse{}, "", false, err
	}
	response := BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remainingForNewBooking(data.event, status, capacity, confirmedCount), Message: bookingMessage(status)}
	if err := s.completeBookingIdempotencyResultTx(ctx, tx, data.input.idempotencyKey, response); err != nil {
		return BookingResponse{}, "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookingResponse{}, "", false, err
	}
	return response, action, status == RegistrationConfirmed, nil
}

type bookingInput struct {
	eventID        string
	employeeID     string
	idempotencyKey string
	familyCount    int
}

func normalizeBookingInput(actor Actor, eventID string, req BookingRequest) (bookingInput, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return bookingInput{}, err
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return bookingInput{}, badRequest("idempotency_key is required")
	}
	if req.FamilyCount < 0 || req.FamilyCount > maxFamilyCount {
		return bookingInput{}, badRequest("family_count must be between 0 and 10")
	}
	employeeID := strings.TrimSpace(req.EmployeeID)
	if employeeID == "" {
		employeeID = actor.ID
	}
	if employeeID == "" {
		return bookingInput{}, badRequest("employee_id is required")
	}
	if actor.ID != "" && actor.ID != employeeID {
		return bookingInput{}, forbidden("employees may only book for themselves")
	}
	return bookingInput{eventID: eventID, employeeID: employeeID, idempotencyKey: req.IdempotencyKey, familyCount: req.FamilyCount}, nil
}

func (s *Service) loadBookingEventWithRuleTx(ctx context.Context, tx pgx.Tx, eventID string, hold reservation.Hold) (Event, EligibilityRule, error) {
	if hold.Outcome == reservation.OutcomeExhausted {
		// PH2-22 fast path: Redis already determined this event is full. The
		// booking commits a waitlist row, which has no capacity constraint.
		// A shared lock still protects the event-state recheck from racing
		// with close, cancel, or archive updates.
		return s.readEventWithRuleShareLockTx(ctx, tx, eventID)
	}
	return s.lockEventWithRule(ctx, tx, eventID)
}

func (s *Service) validateBookingRequestTx(ctx context.Context, tx pgx.Tx, event Event, input bookingInput, now time.Time) error {
	// checkBookingBanTx is called after lockEventWithRule so that the ban row
	// written by a concurrent cancel transaction is visible under the same lock;
	// a check before the lock would be a stale-read race.
	if err := s.checkBookingBanTx(ctx, tx, input.eventID, input.employeeID); err != nil {
		return err
	}
	if event.Status != EventStatusPublished {
		return conflict("event is not open for booking")
	}
	if now.Before(event.RegistrationStart) || now.After(event.RegistrationClose) {
		return conflict("registration window is closed")
	}
	if event.CapacityType == CapacityTypeLimited && input.familyCount > 0 {
		return badRequest("limited events cannot accept family attendees")
	}
	if event.CapacityType == CapacityTypeUnlimited && !event.AllowsFamily && input.familyCount > 0 {
		return badRequest("event does not allow family attendees")
	}
	return s.requireNoActiveNoShowCooldownTx(ctx, tx, event, input.employeeID, now)
}

func (s *Service) requireNoActiveNoShowCooldownTx(ctx context.Context, tx pgx.Tx, event Event, employeeID string, now time.Time) error {
	if event.CapacityType != CapacityTypeLimited {
		return nil
	}
	cooldown, found, err := s.activeNoShowCooldownTx(ctx, tx, employeeID, now)
	if err != nil {
		return err
	}
	if found {
		return forbidden("limited event booking blocked by no-show cooldown until " + cooldown.Format("2006-01-02"))
	}
	return nil
}

func (s *Service) bookingEmployeeForEligibilityTx(ctx context.Context, tx pgx.Tx, actor Actor, employeeID string) (Employee, Employee, error) {
	employee, err := s.getEmployeeTx(ctx, tx, employeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, Employee{}, notFound("employee not found")
	}
	if err != nil {
		return Employee{}, Employee{}, err
	}
	if actor.Claims == nil {
		return employee, employee, nil
	}
	eligibilityEmployee, err := employeeFromClaims(actor)
	if err != nil {
		return Employee{}, Employee{}, forbidden(ErrMissingClaims.Error())
	}
	return employee, eligibilityEmployee, nil
}

func requireEligible(employee Employee, rule EligibilityRule) error {
	eligible, reason := EvaluateEligibility(employee, rule)
	if !eligible {
		return forbidden(reason)
	}
	return nil
}

func (s *Service) replayExistingBookingTx(ctx context.Context, tx pgx.Tx, event Event, input bookingInput) (BookingResponse, bool, error) {
	reg, ticket, found, err := s.findRegistrationByEmployeeTx(ctx, tx, input.eventID, input.employeeID)
	if err != nil || !found {
		return BookingResponse{}, false, err
	}
	remaining, err := s.remainingForResponseTx(ctx, tx, event)
	if err != nil {
		return BookingResponse{}, false, err
	}
	response := BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remaining, Message: bookingMessage(reg.Status), Duplicate: true}
	if err := s.completeBookingIdempotencyResultTx(ctx, tx, input.idempotencyKey, response); err != nil {
		return BookingResponse{}, false, err
	}
	return response, true, tx.Commit(ctx)
}

func (s *Service) statusForNewBookingTx(ctx context.Context, tx pgx.Tx, event Event, eventID string, hold reservation.Hold) (string, int, int, error) {
	status := RegistrationConfirmed
	capacity := 0
	confirmedCount := 0
	if event.CapacityType == CapacityTypeLimited {
		var err error
		capacity, err = limitedCapacity(event)
		if err != nil {
			return "", 0, 0, err
		}
		confirmedCount, err = s.confirmedCountTx(ctx, tx, eventID)
		if err != nil {
			return "", 0, 0, err
		}
	}
	if event.AllocationMode == AllocationModeLottery {
		status = RegistrationReceived
	} else if event.CapacityType == CapacityTypeLimited && (hold.Outcome == reservation.OutcomeExhausted || confirmedCount >= capacity) {
		status = RegistrationWaitlisted
		confirmedCount = capacity
	}
	return status, capacity, confirmedCount, nil
}

func (s *Service) insertNewBookingRegistrationTx(ctx context.Context, tx pgx.Tx, input bookingInput, status string, now time.Time) (Registration, *BookingResponse, error) {
	regID, err := newID("reg")
	if err != nil {
		return Registration{}, nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key, family_count)
			VALUES ($1,$2,$3,$4,$5,$6)`, regID, input.eventID, input.employeeID, status, input.idempotencyKey, input.familyCount)
	if err != nil {
		response, replayErr := s.replayRegistrationInsertConflictTx(ctx, tx, input, err)
		return Registration{}, response, replayErr
	}
	reg := Registration{RegistrationID: regID, EventID: input.eventID, EmployeeID: input.employeeID, Status: status, IdempotencyKey: input.idempotencyKey, FamilyCount: input.familyCount, CreatedAt: now}
	return reg, nil, nil
}

func (s *Service) replayRegistrationInsertConflictTx(ctx context.Context, tx pgx.Tx, input bookingInput, insertErr error) (*BookingResponse, error) {
	if !isUniqueViolation(insertErr) {
		return nil, insertErr
	}
	existing, found, err := s.findRegistrationByIdempotencyKey(ctx, tx, input.idempotencyKey, input.eventID, input.employeeID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, insertErr
	}
	if existing.Registration.FamilyCount != input.familyCount {
		return nil, conflict("idempotency key belongs to a different booking request")
	}
	if err := s.completeBookingIdempotencyResultTx(ctx, tx, input.idempotencyKey, existing); err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *Service) issueTicketForConfirmedRegistrationTx(ctx context.Context, tx pgx.Tx, actor Actor, employee Employee, reg Registration) (*Ticket, error) {
	if reg.Status != RegistrationConfirmed {
		return nil, nil
	}
	created, err := s.createTicketTx(ctx, tx, reg, employee)
	if err != nil {
		return nil, err
	}
	if err := insertTicketIssuedAuditTx(ctx, tx, actor, created); err != nil {
		return nil, err
	}
	return &created, nil
}

func bookingActionForStatus(status string) string {
	switch status {
	case RegistrationWaitlisted:
		return "booking.waitlisted"
	case RegistrationReceived:
		return "booking.received"
	default:
		return "booking.confirmed"
	}
}

func (s *Service) recordBookingEffectsTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, reg Registration, action string) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{"event_id": reg.EventID, "status": reg.Status, "capacity_type": event.CapacityType, "family_count": reg.FamilyCount}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, action, "registration", reg.RegistrationID, metadata)); err != nil {
		return err
	}
	return insertOutbox(ctx, tx, action, reg.RegistrationID, bookingNotificationPayload(reg, event, actor))
}

func (s *Service) replayCompletedBooking(ctx context.Context, key, eventID, employeeID string, familyCount int) (BookingResponse, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BookingResponse{}, false, err
	}
	defer rollback(ctx, tx)

	snapshot, found, err := s.completedBookingIdempotencyResultTx(ctx, tx, key, eventID, employeeID, familyCount)
	if err != nil || !found {
		return BookingResponse{}, found, err
	}
	response, err := s.bookingResponseFromIdempotencyResultTx(ctx, tx, snapshot)
	if err != nil {
		return BookingResponse{}, false, err
	}
	return response, true, tx.Commit(ctx)
}

func remainingForNewBooking(event Event, status string, capacity int, confirmedCount int) int {
	if event.CapacityType == CapacityTypeLimited && status == RegistrationConfirmed {
		return max(capacity-confirmedCount-1, 0)
	}
	if event.CapacityType == CapacityTypeLimited && status == RegistrationReceived {
		return max(capacity-confirmedCount, 0)
	}
	return 0
}
