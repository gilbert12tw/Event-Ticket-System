package ticketing

import (
	"context"
	"errors"
	"strings"
	"time"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

func (s *Service) CreateEvent(ctx context.Context, actor Actor, req CreateEventRequest) (EventSummary, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EventSummary{}, err
	}
	if strings.TrimSpace(req.Title) == "" {
		return EventSummary{}, badRequest("title is required")
	}
	if req.Capacity <= 0 {
		return EventSummary{}, badRequest("capacity must be positive")
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
		(event_id, title, description, location, starts_at, registration_start, registration_close, capacity, status, allocation_mode, category, tags, entry_method, visibility, version, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'fcfs',$10,$11,$12,$13,1,$14)`,
		eventID, strings.TrimSpace(req.Title), req.Description, req.Location, req.StartsAt, req.RegistrationStart, req.RegistrationClose, req.Capacity, req.Status,
		strings.TrimSpace(req.Category), joinTags(req.Tags), strings.TrimSpace(req.EntryMethod), strings.TrimSpace(req.Visibility), actor.ID)
	if err != nil {
		return EventSummary{}, err
	}
	event := Event{
		EventID:           eventID,
		Title:             strings.TrimSpace(req.Title),
		Description:       req.Description,
		Location:          req.Location,
		StartsAt:          req.StartsAt,
		RegistrationStart: req.RegistrationStart,
		RegistrationClose: req.RegistrationClose,
		Capacity:          req.Capacity,
		Status:            req.Status,
		AllocationMode:    "fcfs",
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
	if err := insertAudit(ctx, tx, auditID, actor, "event.created", "event", eventID, map[string]interface{}{"capacity": req.Capacity, "status": req.Status}); err != nil {
		return EventSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventSummary{}, err
	}

	s.logger.Info("event created", "trace_id", traceid.FromContext(ctx), "action", "event.created", "status", "success", "event_id", eventID, "actor_role", actor.Role)
	return s.GetEventSummary(ctx, eventID, "")
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
		summary, err := s.GetEventSummary(ctx, eventID, employeeID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func (s *Service) GetEventSummary(ctx context.Context, eventID string, employeeID string) (EventSummary, error) {
	var summary EventSummary
	var tags string
	err := s.db.QueryRow(ctx, `SELECT
			e.event_id, e.title, e.description, e.location, e.starts_at, e.registration_start, e.registration_close,
			e.capacity, e.status, e.allocation_mode, e.category, e.tags, e.entry_method, e.visibility, e.version,
			COALESCE(e.archived_at, '0001-01-01 00:00:00+00'::timestamptz), e.created_by, e.created_at, e.updated_at,
			r.rule_id, r.event_id, r.department, r.site, r.min_grade, r.employment_status, r.version,
			(SELECT count(*) FROM registrations rg WHERE rg.event_id = e.event_id AND rg.status = 'confirmed') AS confirmed_count,
			(SELECT count(*) FROM registrations rg WHERE rg.event_id = e.event_id AND rg.status = 'waitlisted') AS waitlist_count
		FROM events e
		JOIN eligibility_rules r ON r.event_id = e.event_id
		WHERE e.event_id = $1`, eventID).
		Scan(
			&summary.EventID, &summary.Title, &summary.Description, &summary.Location, &summary.StartsAt, &summary.RegistrationStart, &summary.RegistrationClose,
			&summary.Capacity, &summary.Status, &summary.AllocationMode, &summary.Category, &tags, &summary.EntryMethod, &summary.Visibility, &summary.Version,
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
	summary.Tags = splitTags(tags)
	summary.RemainingCapacity = max(summary.Capacity-summary.ConfirmedCount, 0)
	summary.Eligible = false
	summary.EligibilityReason = "employee_id query parameter is required"
	if employeeID != "" {
		employee, err := s.getEmployee(ctx, employeeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				summary.EligibilityReason = "employee not found"
			} else {
				return EventSummary{}, err
			}
		} else {
			summary.Eligible, summary.EligibilityReason = EvaluateEligibility(employee, summary.Rule)
		}
		reg, ticket, found, err := s.findRegistrationByEmployee(ctx, eventID, employeeID)
		if err != nil {
			return EventSummary{}, err
		}
		if found {
			summary.CurrentUserStatus = reg.Status
			summary.CurrentUserTicket = sanitizeTicket(ticket)
		}
	}
	return summary, nil
}

func (s *Service) CheckEligibility(ctx context.Context, actor Actor, eventID string, employeeID string) (map[string]interface{}, error) {
	if strings.TrimSpace(employeeID) == "" {
		return nil, badRequest("employee_id is required")
	}
	employeeID, err := authorizeEmployeeRead(actor, employeeID)
	if err != nil {
		return nil, err
	}
	summary, err := s.GetEventSummary(ctx, eventID, employeeID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"event_id":    eventID,
		"employee_id": employeeID,
		"eligible":    summary.Eligible,
		"reason":      summary.EligibilityReason,
	}, nil
}
