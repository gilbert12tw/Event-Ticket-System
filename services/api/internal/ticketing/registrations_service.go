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
	totalStarted := time.Now()
	totalOutcome := "error"
	defer func() {
		s.observeBookingStage("total", totalOutcome, time.Since(totalStarted))
	}()
	if err := requireRole(actor, RoleEmployee); err != nil {
		return BookingResponse{}, err
	}
	identity, err := normalizeBookingRequest(actor, req)
	if err != nil {
		return BookingResponse{}, err
	}

	hold, idempotencyHash, err := s.preadmitBooking(ctx, eventID, identity.employeeID, identity.idempotencyKey, identity.familyCount)
	if err != nil {
		return BookingResponse{}, err
	}
	gateConfirmed := false
	defer func() {
		s.finalizeReservation(ctx, eventID, idempotencyHash, gateConfirmed)
	}()

	result, err := s.bookInTransaction(ctx, actor, eventID, identity, hold, idempotencyHash)
	if err != nil {
		return BookingResponse{}, err
	}
	gateConfirmed = result.confirmedNewRegistration
	totalOutcome = result.status

	if result.action != "" {
		s.logger.Info("booking completed", "trace_id", traceid.FromContext(ctx), "action", result.action, "status", result.status, "event_id", eventID, "actor_role", actor.Role)
	}
	return result.response, nil
}

type bookingIdentity struct {
	idempotencyKey string
	employeeID     string
	familyCount    int
}

type bookingTxResult struct {
	response                 BookingResponse
	action                   string
	status                   string
	confirmedNewRegistration bool
}

type bookingCreation struct {
	actor          Actor
	event          Event
	employee       Employee
	identity       bookingIdentity
	status         string
	capacity       int
	confirmedCount int
}

func (s *Service) bookInTransaction(ctx context.Context, actor Actor, eventID string, identity bookingIdentity, hold reservation.Hold, idempotencyHash string) (bookingTxResult, error) {
	txStarted := time.Now()
	txOutcome := "error"
	defer func() {
		s.observeBookingStage("tx", txOutcome, time.Since(txStarted))
	}()
	beginStarted := time.Now()
	tx, err := s.db.Begin(ctx)
	s.observeBookingStage("begin_tx", outcomeForError(err), time.Since(beginStarted))
	if err != nil {
		return bookingTxResult{}, err
	}
	defer rollback(ctx, tx)

	if result, found, err := s.replayLockedBookingStage(ctx, tx, eventID, identity, idempotencyHash); err != nil || found {
		txOutcome = bookingOutcomeIfFound(txOutcome, result, found, err)
		return result, err
	}
	stageStarted := time.Now()
	event, rule, err := s.lockBookableEventTx(ctx, tx, eventID, hold)
	s.observeBookingStage("event_lock", outcomeForError(err), time.Since(stageStarted))
	if err != nil {
		return bookingTxResult{}, err
	}
	stageStarted = time.Now()
	employee, err := s.validateBookingTx(ctx, tx, actor, event, rule, identity)
	s.observeBookingStage("validate", outcomeForError(err), time.Since(stageStarted))
	if err != nil {
		return bookingTxResult{}, err
	}
	if result, found, err := s.duplicateBookingStage(ctx, tx, event, eventID, identity); err != nil || found {
		txOutcome = bookingOutcomeIfFound(txOutcome, result, found, err)
		return result, err
	}
	stageStarted = time.Now()
	status, capacity, confirmedCount, err := s.resolveBookingStatusTx(ctx, tx, event, eventID, hold)
	s.observeBookingStage("capacity", outcomeForError(err), time.Since(stageStarted))
	if err != nil {
		return bookingTxResult{}, err
	}
	creation := bookingCreation{actor: actor, event: event, employee: employee, identity: identity, status: status, capacity: capacity, confirmedCount: confirmedCount}
	stageStarted = time.Now()
	response, action, confirmedNewRegistration, err := s.createBookingResponseTx(ctx, tx, creation)
	s.observeBookingStage("create_response", outcomeForError(err), time.Since(stageStarted))
	if err != nil {
		return bookingTxResult{}, err
	}
	stageStarted = time.Now()
	if err := tx.Commit(ctx); err != nil {
		s.observeBookingStage("commit", "error", time.Since(stageStarted))
		return bookingTxResult{}, err
	}
	s.observeBookingStage("commit", "success", time.Since(stageStarted))
	txOutcome = status
	return bookingTxResult{response: response, action: action, status: status, confirmedNewRegistration: confirmedNewRegistration}, nil
}

func bookingOutcomeIfFound(current string, result bookingTxResult, found bool, err error) string {
	if err == nil && found {
		return result.status
	}
	return current
}

func (s *Service) replayLockedBookingStage(ctx context.Context, tx pgx.Tx, eventID string, identity bookingIdentity, idempotencyHash string) (bookingTxResult, bool, error) {
	stageStarted := time.Now()
	result, found, err := s.replayLockedBookingTx(ctx, tx, eventID, identity, idempotencyHash)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.observeBookingStage("idempotency_lock", outcome, time.Since(stageStarted))
	return result, found, err
}

func (s *Service) replayLockedBookingTx(ctx context.Context, tx pgx.Tx, eventID string, identity bookingIdentity, idempotencyHash string) (bookingTxResult, bool, error) {
	snapshot, found, err := s.lockBookingIdempotencyResultTx(ctx, tx, identity.idempotencyKey, eventID, identity.employeeID, identity.familyCount, idempotencyHash)
	if err != nil || !found {
		return bookingTxResult{}, found, err
	}
	response, err := s.bookingResponseFromIdempotencyResultTx(ctx, tx, snapshot)
	if err != nil {
		return bookingTxResult{}, false, err
	}
	return bookingTxResult{response: response, status: response.Registration.Status}, true, tx.Commit(ctx)
}

func (s *Service) duplicateBookingStage(ctx context.Context, tx pgx.Tx, event Event, eventID string, identity bookingIdentity) (bookingTxResult, bool, error) {
	stageStarted := time.Now()
	response, found, err := s.duplicateBookingResponseTx(ctx, tx, event, eventID, identity)
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	s.observeBookingStage("duplicate_lookup", outcome, time.Since(stageStarted))
	if err != nil || !found {
		return bookingTxResult{}, found, err
	}
	return bookingTxResult{response: response, status: response.Registration.Status}, true, nil
}

func normalizeBookingRequest(actor Actor, req BookingRequest) (bookingIdentity, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return bookingIdentity{}, badRequest("idempotency_key is required")
	}
	if req.FamilyCount < 0 || req.FamilyCount > maxFamilyCount {
		return bookingIdentity{}, badRequest("family_count must be between 0 and 10")
	}
	employeeID := strings.TrimSpace(req.EmployeeID)
	if employeeID == "" {
		employeeID = actor.ID
	}
	if employeeID == "" {
		return bookingIdentity{}, badRequest("employee_id is required")
	}
	if actor.ID != "" && actor.ID != employeeID {
		return bookingIdentity{}, forbidden("employees may only book for themselves")
	}
	return bookingIdentity{idempotencyKey: strings.TrimSpace(req.IdempotencyKey), employeeID: employeeID, familyCount: req.FamilyCount}, nil
}

func (s *Service) lockBookableEventTx(ctx context.Context, tx pgx.Tx, eventID string, hold reservation.Hold) (Event, EligibilityRule, error) {
	if hold.Outcome == reservation.OutcomeExhausted {
		// PH2-22 fast path: Redis already determined this event is full. The
		// booking commits a waitlist row, which has no capacity constraint.
		// A shared lock still protects the event-state recheck from racing
		// with close, cancel, or archive updates.
		return s.readEventWithRuleShareLockTx(ctx, tx, eventID)
	}
	return s.lockEventWithRule(ctx, tx, eventID)
}

func (s *Service) validateBookingTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, rule EligibilityRule, identity bookingIdentity) (Employee, error) {
	if err := s.validateBookableEventTx(ctx, tx, event, identity); err != nil {
		return Employee{}, err
	}
	employee, err := s.getEmployeeTx(ctx, tx, identity.employeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, notFound("employee not found")
	}
	if err != nil {
		return Employee{}, err
	}
	return employee, validateBookingEligibility(actor, employee, rule)
}

func (s *Service) validateBookableEventTx(ctx context.Context, tx pgx.Tx, event Event, identity bookingIdentity) error {
	// checkBookingBanTx is called after lockEventWithRule so that the ban row
	// written by a concurrent cancel transaction is visible under the same lock;
	// a check before the lock would be a stale-read race.
	if err := s.checkBookingBanTx(ctx, tx, event.EventID, identity.employeeID); err != nil {
		return err
	}
	if event.Status != EventStatusPublished {
		return conflict("event is not open for booking")
	}
	now := s.now()
	if now.Before(event.RegistrationStart) || now.After(event.RegistrationClose) {
		return conflict("registration window is closed")
	}
	if event.CapacityType == CapacityTypeLimited && identity.familyCount > 0 {
		return badRequest("limited events cannot accept family attendees")
	}
	if event.CapacityType == CapacityTypeUnlimited && !event.AllowsFamily && identity.familyCount > 0 {
		return badRequest("event does not allow family attendees")
	}
	if event.CapacityType != CapacityTypeLimited {
		return nil
	}
	cooldown, found, err := s.activeNoShowCooldownTx(ctx, tx, identity.employeeID, now)
	if err != nil || !found {
		return err
	}
	return forbidden("limited event booking blocked by no-show cooldown until " + cooldown.Format("2006-01-02"))
}

func validateBookingEligibility(actor Actor, employee Employee, rule EligibilityRule) error {
	eligibilityEmployee := employee
	if actor.Claims != nil {
		fromClaims, err := employeeFromClaims(actor)
		if err != nil {
			return forbidden(ErrMissingClaims.Error())
		}
		eligibilityEmployee = fromClaims
	}
	eligible, reason := EvaluateEligibility(eligibilityEmployee, rule)
	if !eligible {
		return forbidden(reason)
	}
	return nil
}

func (s *Service) duplicateBookingResponseTx(ctx context.Context, tx pgx.Tx, event Event, eventID string, identity bookingIdentity) (BookingResponse, bool, error) {
	reg, ticket, found, err := s.findRegistrationByEmployeeTx(ctx, tx, eventID, identity.employeeID)
	if err != nil || !found {
		return BookingResponse{}, found, err
	}
	remaining, err := s.remainingForResponseTx(ctx, tx, event)
	if err != nil {
		return BookingResponse{}, false, err
	}
	response := BookingResponse{Registration: reg, Ticket: ticket, RemainingCapacity: remaining, Message: bookingMessage(reg.Status), Duplicate: true}
	if err := s.completeBookingIdempotencyResultTx(ctx, tx, identity.idempotencyKey, response); err != nil {
		return BookingResponse{}, false, err
	}
	return response, true, tx.Commit(ctx)
}

func (s *Service) resolveBookingStatusTx(ctx context.Context, tx pgx.Tx, event Event, eventID string, hold reservation.Hold) (string, int, int, error) {
	if event.AllocationMode == AllocationModeLottery {
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
		return RegistrationReceived, capacity, confirmedCount, nil
	}
	if event.CapacityType != CapacityTypeLimited {
		return RegistrationConfirmed, 0, 0, nil
	}
	capacity, err := limitedCapacity(event)
	if err != nil {
		return "", 0, 0, err
	}
	if hold.Outcome == reservation.OutcomeExhausted {
		return RegistrationWaitlisted, capacity, capacity, nil
	}
	confirmedCount, err := s.confirmedCountTx(ctx, tx, eventID)
	if err != nil {
		return "", 0, 0, err
	}
	if confirmedCount >= capacity {
		return RegistrationWaitlisted, capacity, confirmedCount, nil
	}
	return RegistrationConfirmed, capacity, confirmedCount, nil
}

func (s *Service) createBookingResponseTx(ctx context.Context, tx pgx.Tx, creation bookingCreation) (BookingResponse, string, bool, error) {
	result, err := s.insertBookingRegistrationTx(ctx, tx, creation)
	if err != nil {
		return BookingResponse{}, "", false, err
	}
	if result.replayed != nil {
		return *result.replayed, "", false, nil
	}
	reg := result.registration
	action := bookingAction(creation.status)
	if err := s.recordBookingSideEffectsTx(ctx, tx, creation.actor, creation.event, reg, action); err != nil {
		return BookingResponse{}, "", false, err
	}
	response := BookingResponse{
		Registration:      reg,
		Ticket:            result.ticket,
		RemainingCapacity: remainingForNewBooking(creation.event, creation.status, creation.capacity, creation.confirmedCount),
		Message:           bookingMessage(creation.status),
	}
	if err := s.completeBookingIdempotencyResultTx(ctx, tx, creation.identity.idempotencyKey, response); err != nil {
		return BookingResponse{}, "", false, err
	}
	return response, action, creation.status == RegistrationConfirmed, nil
}

type bookingRegistrationResult struct {
	registration Registration
	ticket       *Ticket
	replayed     *BookingResponse
}

func (s *Service) insertBookingRegistrationTx(ctx context.Context, tx pgx.Tx, creation bookingCreation) (bookingRegistrationResult, error) {
	regID, err := newID("reg")
	if err != nil {
		return bookingRegistrationResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key, family_count)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		regID, creation.event.EventID, creation.identity.employeeID, creation.status, creation.identity.idempotencyKey, creation.identity.familyCount)
	if err != nil {
		return s.registrationInsertConflictResponseTx(ctx, tx, creation.event.EventID, creation.identity, err)
	}
	reg := Registration{
		RegistrationID: regID,
		EventID:        creation.event.EventID,
		EmployeeID:     creation.identity.employeeID,
		Status:         creation.status,
		IdempotencyKey: creation.identity.idempotencyKey,
		FamilyCount:    creation.identity.familyCount,
		CreatedAt:      s.now(),
	}
	ticket, err := s.createBookingTicketTx(ctx, tx, creation.actor, reg, creation.employee, creation.status)
	return bookingRegistrationResult{registration: reg, ticket: ticket}, err
}

func (s *Service) registrationInsertConflictResponseTx(ctx context.Context, tx pgx.Tx, eventID string, identity bookingIdentity, insertErr error) (bookingRegistrationResult, error) {
	if !isUniqueViolation(insertErr) {
		return bookingRegistrationResult{}, insertErr
	}
	existing, found, err := s.findRegistrationByIdempotencyKey(ctx, tx, identity.idempotencyKey, eventID, identity.employeeID)
	if err != nil || !found {
		return bookingRegistrationResult{}, err
	}
	if existing.Registration.FamilyCount != identity.familyCount {
		return bookingRegistrationResult{}, conflict("idempotency key belongs to a different booking request")
	}
	if err := s.completeBookingIdempotencyResultTx(ctx, tx, identity.idempotencyKey, existing); err != nil {
		return bookingRegistrationResult{}, err
	}
	return bookingRegistrationResult{replayed: &existing}, nil
}

func (s *Service) createBookingTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, reg Registration, employee Employee, status string) (*Ticket, error) {
	if status != RegistrationConfirmed {
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

func (s *Service) recordBookingSideEffectsTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, reg Registration, action string) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	details := map[string]interface{}{"event_id": event.EventID, "status": reg.Status, "capacity_type": event.CapacityType, "family_count": reg.FamilyCount}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, action, "registration", reg.RegistrationID, details)); err != nil {
		return err
	}
	return insertOutbox(ctx, tx, action, reg.RegistrationID, bookingNotificationPayload(reg, event, actor))
}

func bookingAction(status string) string {
	switch status {
	case RegistrationWaitlisted:
		return "booking.waitlisted"
	case RegistrationReceived:
		return "booking.received"
	default:
		return "booking.confirmed"
	}
}

func (s *Service) observeBookingStage(stage string, outcome string, duration time.Duration) {
	if s.metrics == nil {
		return
	}
	s.metrics.ObserveBookingStage(stage, outcome, duration)
}

func outcomeForError(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
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
