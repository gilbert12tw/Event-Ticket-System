package ticketing

import (
	"context"
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
	if strings.TrimSpace(req.Title) == "" {
		return EventSummary{}, badRequest("title is required")
	}
	capacityType, capacity, allowsFamily, err := normalizeEventCapacity(req.CapacityType, req.Capacity, req.AllowsFamily)
	if err != nil {
		return EventSummary{}, err
	}
	if req.Status == "" {
		req.Status = EventStatusPublished
	}
	if req.Status != EventStatusDraft && req.Status != EventStatusPublished {
		return EventSummary{}, badRequest("status must be draft or published")
	}
	if strings.TrimSpace(req.EntryMethod) == "" {
		req.EntryMethod = "qr"
	}
	if strings.TrimSpace(req.Visibility) == "" {
		req.Visibility = "eligible"
	}
	now := s.now()
	if req.StartsAt.IsZero() {
		req.StartsAt = now.Add(7 * 24 * time.Hour)
	}
	if req.RegistrationStart.IsZero() {
		req.RegistrationStart = now.Add(-time.Hour)
	}
	if req.RegistrationClose.IsZero() {
		req.RegistrationClose = req.StartsAt.Add(-time.Hour)
	}
	if !req.RegistrationStart.Before(req.RegistrationClose) {
		return EventSummary{}, badRequest("registration_start must be before registration_close")
	}
	ruleInput := normalizeRuleInput(req.Rule)
	eventCity := eventCityOrFallback(req.EventCity, req.Location)
	eventSite := eventSiteOrFallback(req.EventSite, req.Location)

	eventID, err := newID("evt")
	if err != nil {
		return EventSummary{}, err
	}
	ruleID, err := newID("rule")
	if err != nil {
		return EventSummary{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return EventSummary{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EventSummary{}, err
	}
	defer rollback(ctx, tx)

	_, err = tx.Exec(ctx, `INSERT INTO events
		(event_id, title, description, location, event_city, event_site, starts_at, registration_start, registration_close,
		 capacity_type, capacity, allows_family, status, allocation_mode, category, tags, entry_method, visibility, version, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'fcfs',$14,$15,$16,$17,1,$18)`,
		eventID, strings.TrimSpace(req.Title), req.Description, req.Location, eventCity, eventSite, req.StartsAt, req.RegistrationStart, req.RegistrationClose,
		capacityType, capacity, allowsFamily, req.Status, strings.TrimSpace(req.Category), joinTags(req.Tags), strings.TrimSpace(req.EntryMethod), strings.TrimSpace(req.Visibility), actor.ID)
	if err != nil {
		return EventSummary{}, err
	}
	event := Event{
		EventID:           eventID,
		Title:             strings.TrimSpace(req.Title),
		Description:       req.Description,
		Location:          req.Location,
		EventCity:         eventCity,
		EventSite:         eventSite,
		StartsAt:          req.StartsAt,
		RegistrationStart: req.RegistrationStart,
		RegistrationClose: req.RegistrationClose,
		CapacityType:      capacityType,
		Capacity:          capacity,
		AllowsFamily:      allowsFamily,
		Status:            req.Status,
		AllocationMode:    AllocationModeFCFS,
		Category:          strings.TrimSpace(req.Category),
		Tags:              normalizeTags(req.Tags),
		EntryMethod:       strings.TrimSpace(req.EntryMethod),
		Visibility:        strings.TrimSpace(req.Visibility),
		Version:           1,
	}
	if err := s.insertEventVersionTx(ctx, tx, actor, event, "event created"); err != nil {
		return EventSummary{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO eligibility_rules
		(rule_id, event_id, department, site, min_grade, employment_status, version)
		VALUES ($1,$2,$3,$4,$5,$6,1)`,
		ruleID, eventID, ruleInput.Department, ruleInput.Site, ruleInput.MinGrade, ruleInput.EmploymentStatus)
	if err != nil {
		return EventSummary{}, err
	}
	matchCount, err := s.countEligibleEmployees(ctx, ruleInput)
	if err != nil {
		return EventSummary{}, err
	}
	if err := insertEligibilityRuleVersionTx(ctx, tx, eventID, 1, ruleInput, matchCount, actor.ID); err != nil {
		return EventSummary{}, err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "event.created", "event", eventID, map[string]interface{}{"capacity_type": capacityType, "capacity": capacity, "allows_family": allowsFamily, "status": req.Status}); err != nil {
		return EventSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventSummary{}, err
	}

	s.logger.Info("event created", "trace_id", traceid.FromContext(ctx), "action", "event.created", "status", "success", "event_id", eventID, "actor_role", actor.Role)
	return s.GetEventSummary(ctx, actor, eventID, "")
}

func (s *Service) ListEvents(ctx context.Context, actor Actor, employeeID string) ([]EventSummary, error) {
	employeeID, err := authorizeEmployeeRead(actor, employeeID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT event_id FROM events WHERE status = 'published' ORDER BY starts_at ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []EventSummary
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		summary, err := s.GetEventSummary(ctx, actor, eventID, employeeID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func (s *Service) GetEventSummary(ctx context.Context, actor Actor, eventID string, employeeID string) (EventSummary, error) {
	var summary EventSummary
	var tags string
	var capacity pgtype.Int4
	err := s.db.QueryRow(ctx, `SELECT
			e.event_id, e.title, e.description, e.location, e.event_city, e.event_site, e.starts_at, e.registration_start, e.registration_close,
			e.capacity_type, e.capacity, e.allows_family, e.status, e.allocation_mode, e.category, e.tags, e.entry_method, e.visibility, e.version,
			COALESCE(e.archived_at, '0001-01-01 00:00:00+00'::timestamptz), e.created_by, e.created_at, e.updated_at,
			r.rule_id, r.event_id, r.department, r.site, r.min_grade, r.employment_status, r.version,
			(SELECT count(*) FROM registrations rg WHERE rg.event_id = e.event_id AND rg.status = 'confirmed') AS confirmed_count,
			(SELECT count(*) FROM registrations rg WHERE rg.event_id = e.event_id AND rg.status = 'waitlisted') AS waitlist_count
		FROM events e
		JOIN eligibility_rules r ON r.event_id = e.event_id
		WHERE e.event_id = $1`, eventID).
		Scan(
			&summary.EventID, &summary.Title, &summary.Description, &summary.Location, &summary.EventCity, &summary.EventSite, &summary.StartsAt, &summary.RegistrationStart, &summary.RegistrationClose,
			&summary.CapacityType, &capacity, &summary.AllowsFamily, &summary.Status, &summary.AllocationMode, &summary.Category, &tags, &summary.EntryMethod, &summary.Visibility, &summary.Version,
			&summary.ArchivedAt, &summary.CreatedBy, &summary.CreatedAt, &summary.UpdatedAt,
			&summary.Rule.RuleID, &summary.Rule.EventID, &summary.Rule.Department, &summary.Rule.Site, &summary.Rule.MinGrade, &summary.Rule.EmploymentStatus, &summary.Rule.Version,
			&summary.ConfirmedCount, &summary.WaitlistCount,
		)
	if errors.Is(err, pgx.ErrNoRows) {
		return EventSummary{}, notFound("event not found")
	}
	if err != nil {
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

	summary.Eligibility = EligibilityDecision{EventID: eventID, Eligible: false, Reasons: []string{"provider claims employee identity is required"}}
	summary.Eligible = false
	summary.EligibilityReason = summary.Eligibility.Reasons[0]
	summary.NoShowCooldown = NoShowCooldown{Active: false}

	if employeeID != "" {
		if actor.ID == employeeID && actor.Claims != nil {
			decision, err := s.CheckEligibilityFromClaims(ctx, actor, eventID)
			if err != nil {
				if !errors.Is(err, ErrMissingClaims) {
					return EventSummary{}, err
				}
				summary.Eligibility.Reasons = []string{ErrMissingClaims.Error()}
			} else {
				summary.Eligibility = decision
			}
		} else {
			employee, err := s.getEmployee(ctx, employeeID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					summary.Eligibility.Reasons = []string{"employee not found"}
				} else {
					return EventSummary{}, err
				}
			} else {
				eligible, reason := EvaluateEligibility(employee, summary.Rule)
				var reasons []string
				if reason != "" {
					reasons = append(reasons, reason)
				}
				summary.Eligibility = EligibilityDecision{
					EventID:  eventID,
					Eligible: eligible,
					CanBook:  eligible, // Admin preview ignores cooldown dynamically
					Reasons:  reasons,
				}
			}
		}

		// Shims for backward compatibility
		summary.Eligible = summary.Eligibility.Eligible
		if len(summary.Eligibility.Reasons) > 0 {
			summary.EligibilityReason = summary.Eligibility.Reasons[0]
		} else {
			summary.EligibilityReason = ""
		}

		reg, ticket, found, err := s.findRegistrationByEmployee(ctx, eventID, employeeID)
		if err != nil {
			return EventSummary{}, err
		}
		if found {
			summary.CurrentUserStatus = reg.Status
			summary.CurrentUserRegistrationID = reg.RegistrationID
			summary.CurrentUserTicket = sanitizeTicket(ticket)
		}
		if summary.CapacityType == CapacityTypeLimited {
			tx, err := s.db.Begin(ctx)
			if err != nil {
				return EventSummary{}, err
			}
			defer rollback(ctx, tx)
			until, active, err := s.activeNoShowCooldownTx(ctx, tx, employeeID, s.now())
			if err != nil {
				return EventSummary{}, err
			}
			if err := tx.Commit(ctx); err != nil {
				return EventSummary{}, err
			}
			if active {
				summary.NoShowCooldown = NoShowCooldown{
					Active:    true,
					AppliesTo: CapacityTypeLimited,
					Until:     &until,
					Reason:    "no_show_cooldown",
				}
			}
		}
	}
	return summary, nil
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
