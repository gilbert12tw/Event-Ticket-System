package ticketing

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) CreateEvent(ctx context.Context, actor Actor, req CreateEventRequest) (EventSummary, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EventSummary{}, err
	}
	input, err := s.prepareCreateEventInput(req)
	if err != nil {
		return EventSummary{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EventSummary{}, err
	}
	defer rollback(ctx, tx)
	if err := s.insertCreatedEventTx(ctx, tx, actor, input); err != nil {
		return EventSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventSummary{}, err
	}

	s.logger.Info("event created", "trace_id", traceid.FromContext(ctx), "action", "event.created", "status", "success", "event_id", input.event.EventID, "actor_role", actor.Role)
	return s.GetEventSummary(ctx, actor, input.event.EventID, "")
}

type createEventInput struct {
	event   Event
	ruleID  string
	rule    RuleInput
	auditID string
}

func (s *Service) prepareCreateEventInput(req CreateEventRequest) (createEventInput, error) {
	if strings.TrimSpace(req.Title) == "" {
		return createEventInput{}, badRequest("title is required")
	}
	capacityType, capacity, allowsFamily, err := normalizeEventCapacity(req.CapacityType, req.Capacity, req.AllowsFamily)
	if err != nil {
		return createEventInput{}, err
	}
	allocationMode, err := normalizeAllocationMode(req.AllocationMode)
	if err != nil {
		return createEventInput{}, err
	}
	now := s.now()
	req = defaultCreateEventRequest(req, now)
	if req.Status != EventStatusDraft && req.Status != EventStatusPublished {
		return createEventInput{}, badRequest("status must be draft or published")
	}
	if !req.RegistrationStart.Before(req.RegistrationClose) {
		return createEventInput{}, badRequest("registration_start must be before registration_close")
	}
	if err := validateAllocationModeForCapacity(allocationMode, capacityType); err != nil {
		return createEventInput{}, err
	}
	eventID, ruleID, auditID, err := newCreateEventIDs()
	if err != nil {
		return createEventInput{}, err
	}
	return createEventInput{
		event: Event{
			EventID:           eventID,
			Title:             strings.TrimSpace(req.Title),
			Description:       req.Description,
			Location:          req.Location,
			EventCity:         eventCityOrFallback(req.EventCity, req.Location),
			EventSite:         eventSiteOrFallback(req.EventSite, req.Location),
			StartsAt:          req.StartsAt,
			RegistrationStart: req.RegistrationStart,
			RegistrationClose: req.RegistrationClose,
			CapacityType:      capacityType,
			Capacity:          capacity,
			AllowsFamily:      allowsFamily,
			Status:            req.Status,
			AllocationMode:    allocationMode,
			Category:          strings.TrimSpace(req.Category),
			Tags:              normalizeTags(req.Tags),
			EntryMethod:       strings.TrimSpace(req.EntryMethod),
			Visibility:        strings.TrimSpace(req.Visibility),
			Version:           1,
		},
		ruleID:  ruleID,
		rule:    normalizeRuleInput(req.Rule),
		auditID: auditID,
	}, nil
}

func defaultCreateEventRequest(req CreateEventRequest, now time.Time) CreateEventRequest {
	if req.Status == "" {
		req.Status = EventStatusPublished
	}
	if strings.TrimSpace(req.EntryMethod) == "" {
		req.EntryMethod = "qr"
	}
	if strings.TrimSpace(req.Visibility) == "" {
		req.Visibility = "eligible"
	}
	if req.StartsAt.IsZero() {
		req.StartsAt = now.Add(7 * 24 * time.Hour)
	}
	if req.RegistrationStart.IsZero() {
		req.RegistrationStart = now.Add(-time.Hour)
	}
	if req.RegistrationClose.IsZero() {
		req.RegistrationClose = req.StartsAt.Add(-time.Hour)
	}
	return req
}

func newCreateEventIDs() (string, string, string, error) {
	eventID, err := newID("evt")
	if err != nil {
		return "", "", "", err
	}
	ruleID, err := newID("rule")
	if err != nil {
		return "", "", "", err
	}
	auditID, err := newID("aud")
	if err != nil {
		return "", "", "", err
	}
	return eventID, ruleID, auditID, nil
}

func (s *Service) insertCreatedEventTx(ctx context.Context, tx pgx.Tx, actor Actor, input createEventInput) error {
	event := input.event
	_, err := tx.Exec(ctx, `INSERT INTO events
		(event_id, title, description, location, event_city, event_site, starts_at, registration_start, registration_close,
		 capacity_type, capacity, allows_family, status, allocation_mode, category, tags, entry_method, visibility, version, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,1,$19)`,
		event.EventID, event.Title, event.Description, event.Location, event.EventCity, event.EventSite, event.StartsAt, event.RegistrationStart, event.RegistrationClose,
		event.CapacityType, event.Capacity, event.AllowsFamily, event.Status, event.AllocationMode, event.Category, joinTags(event.Tags), event.EntryMethod, event.Visibility, actor.ID)
	if err != nil {
		return err
	}
	if err := s.insertEventVersionTx(ctx, tx, actor, event, "event created"); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO eligibility_rules
		(rule_id, event_id, department, site, min_grade, employment_status, version)
		VALUES ($1,$2,$3,$4,$5,$6,1)`,
		input.ruleID, event.EventID, input.rule.Department, input.rule.Site, input.rule.MinGrade, input.rule.EmploymentStatus)
	if err != nil {
		return err
	}
	matchCount, err := s.countEligibleEmployees(ctx, input.rule)
	if err != nil {
		return err
	}
	if err := insertEligibilityRuleVersionTx(ctx, tx, event.EventID, 1, input.rule, matchCount, actor.ID); err != nil {
		return err
	}
	return insertAudit(ctx, tx, newAuditRecord(input.auditID, actor, "event.created", "event", event.EventID, map[string]interface{}{"capacity_type": event.CapacityType, "capacity": event.Capacity, "allows_family": event.AllowsFamily, "allocation_mode": event.AllocationMode, "status": event.Status}))
}

func (s *Service) ListEvents(ctx context.Context, actor Actor, employeeID string) ([]EventSummary, error) {
	employeeID, err := authorizeEmployeeRead(actor, employeeID)
	if err != nil {
		return nil, err
	}
	summaries, err := s.loadPublishedEventSummaries(ctx)
	if err != nil {
		return nil, err
	}
	personalization, err := s.loadEventListPersonalization(ctx, actor, employeeID)
	if err != nil {
		return nil, err
	}
	for i := range summaries {
		applyEventListPersonalization(&summaries[i], personalization)
	}
	return summaries, nil
}

func (s *Service) loadPublishedEventSummaries(ctx context.Context) ([]EventSummary, error) {
	rows, err := s.db.Query(ctx, `SELECT
			e.event_id, e.title, e.description, e.location, e.event_city, e.event_site, e.starts_at, e.registration_start, e.registration_close,
			e.capacity_type, e.capacity, e.allows_family, e.status, e.allocation_mode, e.category, e.tags, e.entry_method, e.visibility, e.version,
			COALESCE(e.archived_at, '0001-01-01 00:00:00+00'::timestamptz), e.created_by, e.created_at, e.updated_at,
			r.rule_id, r.event_id, r.department, r.site, r.min_grade, r.employment_status, r.version,
			COALESCE(counts.confirmed_count, 0), COALESCE(counts.waitlist_count, 0)
		FROM events e
		JOIN eligibility_rules r ON r.event_id = e.event_id
		LEFT JOIN (
			SELECT event_id,
				count(*) FILTER (WHERE status = 'confirmed') AS confirmed_count,
				count(*) FILTER (WHERE status = 'waitlisted') AS waitlist_count
			FROM registrations
			GROUP BY event_id
		) counts ON counts.event_id = e.event_id
		WHERE e.status = 'published'
		ORDER BY e.starts_at ASC, e.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []EventSummary
	for rows.Next() {
		summary, err := scanEventSummaryRow(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func (s *Service) GetEventSummary(ctx context.Context, actor Actor, eventID string, employeeID string) (EventSummary, error) {
	summary, err := s.loadEventSummary(ctx, eventID)
	if err != nil {
		return EventSummary{}, err
	}
	initializeEventSummaryEligibility(&summary, eventID)
	if employeeID == "" {
		return summary, nil
	}
	if err := s.populateEmployeeEventSummary(ctx, actor, &summary, eventID, employeeID); err != nil {
		return EventSummary{}, err
	}
	return summary, nil
}

func (s *Service) loadEventSummary(ctx context.Context, eventID string) (EventSummary, error) {
	row := s.db.QueryRow(ctx, `SELECT
			e.event_id, e.title, e.description, e.location, e.event_city, e.event_site, e.starts_at, e.registration_start, e.registration_close,
			e.capacity_type, e.capacity, e.allows_family, e.status, e.allocation_mode, e.category, e.tags, e.entry_method, e.visibility, e.version,
			COALESCE(e.archived_at, '0001-01-01 00:00:00+00'::timestamptz), e.created_by, e.created_at, e.updated_at,
			r.rule_id, r.event_id, r.department, r.site, r.min_grade, r.employment_status, r.version,
			(SELECT count(*) FROM registrations rg WHERE rg.event_id = e.event_id AND rg.status = 'confirmed') AS confirmed_count,
			(SELECT count(*) FROM registrations rg WHERE rg.event_id = e.event_id AND rg.status = 'waitlisted') AS waitlist_count
		FROM events e
		JOIN eligibility_rules r ON r.event_id = e.event_id
		WHERE e.event_id = $1`, eventID)
	summary, err := scanEventSummaryRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return EventSummary{}, notFound(errEventNotFoundMessage)
	}
	if err != nil {
		return EventSummary{}, err
	}
	return summary, nil
}

type eventSummaryScanner interface {
	Scan(dest ...any) error
}

func scanEventSummaryRow(row eventSummaryScanner) (EventSummary, error) {
	var summary EventSummary
	var tags string
	var capacity pgtype.Int4
	if err := row.Scan(
		&summary.EventID, &summary.Title, &summary.Description, &summary.Location, &summary.EventCity, &summary.EventSite, &summary.StartsAt, &summary.RegistrationStart, &summary.RegistrationClose,
		&summary.CapacityType, &capacity, &summary.AllowsFamily, &summary.Status, &summary.AllocationMode, &summary.Category, &tags, &summary.EntryMethod, &summary.Visibility, &summary.Version,
		&summary.ArchivedAt, &summary.CreatedBy, &summary.CreatedAt, &summary.UpdatedAt,
		&summary.Rule.RuleID, &summary.Rule.EventID, &summary.Rule.Department, &summary.Rule.Site, &summary.Rule.MinGrade, &summary.Rule.EmploymentStatus, &summary.Rule.Version,
		&summary.ConfirmedCount, &summary.WaitlistCount,
	); err != nil {
		return EventSummary{}, err
	}
	if capacity.Valid {
		value := int(capacity.Int32)
		summary.Capacity = &value
	}
	summary.EventCity = eventCityOrFallback(summary.EventCity, summary.Location)
	summary.EventSite = eventSiteOrFallback(summary.EventSite, summary.Location)
	summary.Tags = splitTags(tags)
	if summary.CapacityType == CapacityTypeLimited && summary.Capacity != nil {
		summary.RemainingCapacity = intPtr(max(*summary.Capacity-summary.ConfirmedCount, 0))
	}
	return summary, nil
}

func initializeEventSummaryEligibility(summary *EventSummary, eventID string) {
	summary.Eligibility = EligibilityDecision{EventID: eventID, Eligible: false, Reasons: []string{"provider claims employee identity is required"}}
	summary.Eligible = false
	summary.EligibilityReason = summary.Eligibility.Reasons[0]
	summary.NoShowCooldown = NoShowCooldown{Active: false}
}

type eventListPersonalization struct {
	Employee              Employee
	EmployeeFound         bool
	MissingClaims         bool
	EmployeeCity          string
	Cooldown              NoShowCooldown
	RegistrationsByEvent  map[string]Registration
	TicketsByRegistration map[string]*Ticket
}

func (s *Service) loadEventListPersonalization(ctx context.Context, actor Actor, employeeID string) (eventListPersonalization, error) {
	personalization := eventListPersonalization{
		RegistrationsByEvent:  map[string]Registration{},
		TicketsByRegistration: map[string]*Ticket{},
	}
	if actor.ID == employeeID && actor.Claims != nil {
		employee, err := employeeFromClaims(actor)
		if err != nil {
			personalization.MissingClaims = true
		} else {
			personalization.Employee = employee
			personalization.EmployeeFound = true
			personalization.EmployeeCity = normalizeLocation(actor.Claims.City)
		}
	} else {
		employee, err := s.getEmployee(ctx, employeeID)
		if errors.Is(err, pgx.ErrNoRows) {
			personalization.EmployeeFound = false
		} else if err != nil {
			return personalization, err
		} else {
			personalization.Employee = employee
			personalization.EmployeeFound = true
		}
	}

	cooldown, err := s.loadActiveNoShowCooldown(ctx, employeeID)
	if err != nil {
		return personalization, err
	}
	personalization.Cooldown = cooldown

	registrations, tickets, err := s.loadEmployeeEventRegistrations(ctx, employeeID)
	if err != nil {
		return personalization, err
	}
	personalization.RegistrationsByEvent = registrations
	personalization.TicketsByRegistration = tickets
	return personalization, nil
}

func applyEventListPersonalization(summary *EventSummary, personalization eventListPersonalization) {
	initializeEventSummaryEligibility(summary, summary.EventID)
	if personalization.MissingClaims {
		summary.Eligibility.Reasons = []string{ErrMissingClaims.Error()}
		applyEventListCooldown(summary, personalization)
		applyEligibilityCompatibilityFields(summary)
		applyEventListRegistration(summary, personalization)
		return
	}
	if !personalization.EmployeeFound {
		summary.Eligibility.Reasons = []string{"employee not found"}
		applyEventListCooldown(summary, personalization)
		applyEligibilityCompatibilityFields(summary)
		applyEventListRegistration(summary, personalization)
		return
	}

	eligible, reason := EvaluateEligibility(personalization.Employee, summary.Rule)
	summary.Eligibility = EligibilityDecision{
		EventID:  summary.EventID,
		Eligible: eligible,
		CanBook:  eligible,
		Reasons:  eligibilityReasons(reason),
	}
	if personalization.EmployeeCity != "" {
		eventCity := normalizeLocation(summary.EventCity)
		if eventCity != "" && personalization.EmployeeCity != eventCity {
			summary.Eligibility.Warnings = append(summary.Eligibility.Warnings, EligibilityWarning{
				Code:         WarningCrossCity,
				Message:      "This event is in " + eventCity + "; your registered city is " + personalization.EmployeeCity + ".",
				EmployeeCity: personalization.EmployeeCity,
				EventCity:    eventCity,
			})
		}
	}
	applyEventListCooldown(summary, personalization)
	applyEligibilityCompatibilityFields(summary)
	applyEventListRegistration(summary, personalization)
}

func applyEventListCooldown(summary *EventSummary, personalization eventListPersonalization) {
	if summary.CapacityType != CapacityTypeLimited || !personalization.Cooldown.Active {
		return
	}
	summary.NoShowCooldown = personalization.Cooldown
	summary.Eligibility.NoShowCooldown = personalization.Cooldown
	summary.Eligibility.CanBook = false
}

func applyEventListRegistration(summary *EventSummary, personalization eventListPersonalization) {
	reg, found := personalization.RegistrationsByEvent[summary.EventID]
	if !found {
		return
	}
	summary.CurrentUserStatus = reg.Status
	summary.CurrentUserRegistrationID = reg.RegistrationID
	summary.CurrentUserTicket = sanitizeTicket(personalization.TicketsByRegistration[reg.RegistrationID])
}

func (s *Service) loadActiveNoShowCooldown(ctx context.Context, employeeID string) (NoShowCooldown, error) {
	var cooldownUntil time.Time
	err := s.db.QueryRow(ctx, `SELECT cooldown_until
		FROM no_show_records
		WHERE employee_id = $1
			AND status = 'cooldown_active'
			AND cooldown_until IS NOT NULL
			AND cooldown_until > $2
		ORDER BY cooldown_until DESC
		LIMIT 1`, employeeID, s.now()).Scan(&cooldownUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return NoShowCooldown{Active: false}, nil
	}
	if err != nil {
		return NoShowCooldown{}, err
	}
	return NoShowCooldown{
		Active:    true,
		AppliesTo: CapacityTypeLimited,
		Until:     &cooldownUntil,
		Reason:    "no_show_cooldown",
	}, nil
}

func (s *Service) loadEmployeeEventRegistrations(ctx context.Context, employeeID string) (map[string]Registration, map[string]*Ticket, error) {
	rows, err := s.db.Query(ctx, `SELECT
			r.registration_id, r.event_id, r.employee_id, r.status, r.idempotency_key, COALESCE(r.cancel_idempotency_key, ''),
			COALESCE(r.cancelled_at, '0001-01-01 00:00:00+00'::timestamptz), r.cancel_reason, r.family_count, r.created_at,
			t.ticket_id, t.status, t.sequence_number, COALESCE(t.expires_at, t.issued_at + interval '24 hours'), t.revoked_reason, t.issued_at
		FROM registrations r
		LEFT JOIN tickets t ON t.registration_id = r.registration_id
		WHERE r.employee_id = $1 AND r.status <> 'cancelled'`, employeeID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	registrations := map[string]Registration{}
	tickets := map[string]*Ticket{}
	for rows.Next() {
		var reg Registration
		var ticketID sql.NullString
		var ticketStatus sql.NullString
		var sequenceNumber sql.NullInt64
		var expiresAt sql.NullTime
		var revokedReason sql.NullString
		var issuedAt sql.NullTime
		if err := rows.Scan(
			&reg.RegistrationID, &reg.EventID, &reg.EmployeeID, &reg.Status, &reg.IdempotencyKey, &reg.CancelKey,
			&reg.CancelledAt, &reg.CancelReason, &reg.FamilyCount, &reg.CreatedAt,
			&ticketID, &ticketStatus, &sequenceNumber, &expiresAt, &revokedReason, &issuedAt,
		); err != nil {
			return nil, nil, err
		}
		registrations[reg.EventID] = reg
		if ticketID.Valid {
			tickets[reg.RegistrationID] = &Ticket{
				TicketID:       ticketID.String,
				RegistrationID: reg.RegistrationID,
				EventID:        reg.EventID,
				EmployeeID:     reg.EmployeeID,
				Status:         ticketStatus.String,
				SequenceNumber: int(sequenceNumber.Int64),
				ExpiresAt:      expiresAt.Time,
				RevokedReason:  revokedReason.String,
				IssuedAt:       issuedAt.Time,
			}
		}
	}
	return registrations, tickets, rows.Err()
}

func (s *Service) populateEmployeeEventSummary(ctx context.Context, actor Actor, summary *EventSummary, eventID string, employeeID string) error {
	if err := s.populateEventSummaryEligibility(ctx, actor, summary, eventID, employeeID); err != nil {
		return err
	}
	applyEligibilityCompatibilityFields(summary)
	if err := s.populateCurrentUserRegistration(ctx, summary, eventID, employeeID); err != nil {
		return err
	}
	return s.populateNoShowCooldown(ctx, summary, employeeID)
}

func (s *Service) populateEventSummaryEligibility(ctx context.Context, actor Actor, summary *EventSummary, eventID string, employeeID string) error {
	if actor.ID == employeeID && actor.Claims != nil {
		decision, err := s.CheckEligibilityFromClaims(ctx, actor, eventID)
		if err == nil {
			summary.Eligibility = decision
			return nil
		}
		if !errors.Is(err, ErrMissingClaims) {
			return err
		}
		summary.Eligibility.Reasons = []string{ErrMissingClaims.Error()}
		return nil
	}
	employee, err := s.getEmployee(ctx, employeeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			summary.Eligibility.Reasons = []string{"employee not found"}
			return nil
		}
		return err
	}
	eligible, reason := EvaluateEligibility(employee, summary.Rule)
	summary.Eligibility = EligibilityDecision{
		EventID:  eventID,
		Eligible: eligible,
		CanBook:  eligible, // Admin preview ignores cooldown dynamically
		Reasons:  eligibilityReasons(reason),
	}
	return nil
}

func eligibilityReasons(reason string) []string {
	if reason == "" {
		return nil
	}
	return []string{reason}
}

func applyEligibilityCompatibilityFields(summary *EventSummary) {
	summary.Eligible = summary.Eligibility.Eligible
	if len(summary.Eligibility.Reasons) > 0 {
		summary.EligibilityReason = summary.Eligibility.Reasons[0]
		return
	}
	summary.EligibilityReason = ""
}

func (s *Service) populateCurrentUserRegistration(ctx context.Context, summary *EventSummary, eventID string, employeeID string) error {
	reg, ticket, found, err := s.findRegistrationByEmployee(ctx, eventID, employeeID)
	if err != nil || !found {
		return err
	}
	summary.CurrentUserStatus = reg.Status
	summary.CurrentUserRegistrationID = reg.RegistrationID
	summary.CurrentUserTicket = sanitizeTicket(ticket)
	return nil
}

func (s *Service) populateNoShowCooldown(ctx context.Context, summary *EventSummary, employeeID string) error {
	if summary.CapacityType != CapacityTypeLimited {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	until, active, err := s.activeNoShowCooldownTx(ctx, tx, employeeID, s.now())
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if active {
		summary.NoShowCooldown = NoShowCooldown{
			Active:    true,
			AppliesTo: CapacityTypeLimited,
			Until:     &until,
			Reason:    "no_show_cooldown",
		}
	}
	return nil
}

// CheckEligibility is the employee-facing eligibility entry point. It
// delegates to CheckEligibilityFromClaims and returns EligibilityDecision.
// Callers that previously used map[string]interface{} must be updated to use
// EligibilityDecision fields directly.
func (s *Service) CheckEligibility(
	ctx context.Context,
	actor Actor,
	eventID string,
	_ string, // employeeID arg retained for interface compat; ignored — use actor.Claims
) (EligibilityDecision, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return EligibilityDecision{}, err
	}
	return s.CheckEligibilityFromClaims(ctx, actor, eventID)
}
