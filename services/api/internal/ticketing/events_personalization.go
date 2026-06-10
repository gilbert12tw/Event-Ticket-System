package ticketing

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) loadEventListPersonalization(ctx context.Context, actor Actor, employeeID string) (eventListPersonalization, error) {
	personalization := eventListPersonalization{
		RegistrationsByEvent:  map[string]Registration{},
		TicketsByRegistration: map[string]*Ticket{},
		BannedEvents:          map[string]bool{},
		HiddenEvents:          map[string]bool{},
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

	banned, err := s.loadEmployeeBookingBans(ctx, employeeID)
	if err != nil {
		return personalization, err
	}
	personalization.BannedEvents = banned

	hidden, err := s.loadHiddenEvents(ctx, employeeID)
	if err != nil {
		return personalization, err
	}
	personalization.HiddenEvents = hidden
	return personalization, nil
}

func applyEventListPersonalization(summary *EventSummary, personalization eventListPersonalization) {
	initializeEventSummaryEligibility(summary, summary.EventID)
	// A hidden event is a personal view preference independent of eligibility or
	// claim state, so apply it before any early returns below.
	summary.Hidden = personalization.HiddenEvents[summary.EventID]
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
		// Mirror the detail page: an active booking ban (from cancelling a
		// confirmed booking) blocks re-booking/waitlisting, so surface it as a
		// cancelled status instead of a misleading enabled "加入候補" action.
		if personalization.BannedEvents[summary.EventID] {
			summary.CurrentUserStatus = RegistrationCancelled
		}
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

func (s *Service) loadEmployeeBookingBans(ctx context.Context, employeeID string) (map[string]bool, error) {
	banned := map[string]bool{}
	rows, err := s.db.Query(ctx, `SELECT event_id FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		banned[eventID] = true
	}
	return banned, rows.Err()
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
