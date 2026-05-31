package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrMissingClaims is returned when provider/HR claims are absent or
// incomplete. Callers must treat this as a safe degraded rejection (do not
// mutate state) and must not log the raw claims.
var ErrMissingClaims = errors.New("provider claims are missing or incomplete")

// CheckEligibilityFromClaims evaluates employee eligibility using only the
// provider/HR claims already present on actor.Claims. It does not query the
// employees table for the employee's attributes, which means it is safe to call
// on the hot path without a DB round-trip for attribute lookup.
//
// Cross-city mismatch produces a non-blocking EligibilityWarning; it never sets
// Eligible or CanBook to false on its own.
//
// Returns ErrMissingClaims (a safe degraded rejection) when actor.Claims is nil
// or any required field is empty, so callers never panic on missing claims.
func (s *Service) CheckEligibilityFromClaims(
	ctx context.Context,
	actor Actor,
	eventID string,
) (EligibilityDecision, error) {
	employee, err := employeeFromClaims(actor)
	if err != nil {
		return EligibilityDecision{}, err
	}

	// --- load event and its eligibility rule ---
	event, err := s.loadEventWithRule(ctx, eventID)
	if err != nil {
		return EligibilityDecision{}, err
	}

	// --- evaluate rule against claims ---
	// Re-use existing EvaluateEligibility but construct a synthetic Employee
	// from claims so we avoid a DB round-trip for attribute lookup.
	eligible, reason := EvaluateEligibility(employee, event.Rule)

	var reasons []string
	if !eligible {
		reasons = append(reasons, reason)
	}

	// --- cross-city warning (non-blocking) ---
	var warnings []EligibilityWarning
	employeeCity := normalizeLocation(actor.Claims.City)
	eventCity := normalizeLocation(event.EventCity)
	if eventCity != "" && employeeCity != eventCity {
		warnings = append(warnings, EligibilityWarning{
			Code:         WarningCrossCity,
			Message:      "This event is in " + eventCity + "; your registered city is " + employeeCity + ".",
			EmployeeCity: employeeCity,
			EventCity:    eventCity,
		})
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EligibilityDecision{}, err
	}
	defer rollback(ctx, tx)
	until, active, err := s.activeNoShowCooldownTx(ctx, tx, actor.ID, s.now())
	if err != nil {
		return EligibilityDecision{}, err
	}
	_ = tx.Commit(ctx)

	var cooldown NoShowCooldown
	if active && event.CapacityType == CapacityTypeLimited {
		cooldown = NoShowCooldown{
			Active:    true,
			AppliesTo: CapacityTypeLimited,
			Until:     &until,
			Reason:    "no_show_cooldown",
		}
	}

	canBook := eligible && !cooldown.Active

	return EligibilityDecision{
		EventID:        eventID,
		Eligible:       eligible,
		CanBook:        canBook,
		Reasons:        reasons,
		Warnings:       warnings,
		NoShowCooldown: cooldown,
	}, nil
}

func employeeFromClaims(actor Actor) (Employee, error) {
	if actor.ID == "" ||
		actor.Claims == nil ||
		strings.TrimSpace(actor.Claims.Department) == "" ||
		strings.TrimSpace(actor.Claims.Site) == "" ||
		strings.TrimSpace(actor.Claims.City) == "" ||
		actor.Claims.Grade <= 0 ||
		strings.TrimSpace(actor.Claims.EmploymentStatus) == "" {
		return Employee{}, ErrMissingClaims
	}
	return Employee{
		EmployeeID:       actor.ID,
		Department:       strings.TrimSpace(actor.Claims.Department),
		Site:             strings.TrimSpace(actor.Claims.Site),
		JobGrade:         actor.Claims.Grade,
		EmploymentStatus: strings.TrimSpace(actor.Claims.EmploymentStatus),
	}, nil
}

type eventWithRule struct {
	EventID      string
	EventCity    string
	Location     string
	CapacityType string
	Rule         EligibilityRule
}

const errEventNotFoundMessage = "event not found"

func (s *Service) loadEventWithRule(ctx context.Context, eventID string) (eventWithRule, error) {
	var ev eventWithRule
	err := s.db.QueryRow(ctx, `
		SELECT e.event_id, COALESCE(e.event_city, ''), e.location, e.capacity_type,
			   COALESCE(r.department, ''), COALESCE(r.site, ''),
			   COALESCE(r.min_grade, 0), COALESCE(r.employment_status, '')
		FROM events e
		LEFT JOIN eligibility_rules r ON r.event_id = e.event_id
		WHERE e.event_id = $1 AND e.archived_at IS NULL
	`, eventID).Scan(
		&ev.EventID, &ev.EventCity, &ev.Location, &ev.CapacityType,
		&ev.Rule.Department, &ev.Rule.Site,
		&ev.Rule.MinGrade, &ev.Rule.EmploymentStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ev, notFound(errEventNotFoundMessage)
	}
	if err != nil {
		return ev, err
	}
	ev.EventCity = eventCityOrFallback(ev.EventCity, ev.Location)
	return ev, nil
}

func (s *Service) PreviewEligibility(ctx context.Context, actor Actor, eventID string, req EligibilityPreviewRequest) (EligibilityPreviewResponse, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EligibilityPreviewResponse{}, err
	}
	if err := s.requireEventExists(ctx, eventID); err != nil {
		return EligibilityPreviewResponse{}, err
	}
	rule := normalizeRuleInput(req.Rule)
	count, err := s.countEligibleEmployees(ctx, rule)
	if err != nil {
		return EligibilityPreviewResponse{}, err
	}
	return EligibilityPreviewResponse{EventID: eventID, MatchCount: count, ZeroMatch: count == 0}, nil
}

func (s *Service) requireEventExists(ctx context.Context, eventID string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM events WHERE event_id = $1 AND archived_at IS NULL)`, eventID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return notFound(errEventNotFoundMessage)
	}
	return nil
}

func (s *Service) UpdateEligibility(ctx context.Context, actor Actor, eventID string, req UpdateEligibilityRequest) (EligibilityPreviewResponse, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EligibilityPreviewResponse{}, err
	}
	rule := normalizeRuleInput(req.Rule)
	count, err := s.countEligibleEmployees(ctx, rule)
	if err != nil {
		return EligibilityPreviewResponse{}, err
	}
	if count == 0 && !req.AllowZeroMatch {
		return EligibilityPreviewResponse{}, conflict("eligibility rule matches zero employees")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EligibilityPreviewResponse{}, err
	}
	defer rollback(ctx, tx)

	if _, _, err := s.updateEligibilityRuleTx(ctx, tx, actor, eventID, rule, count); err != nil {
		return EligibilityPreviewResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EligibilityPreviewResponse{}, err
	}
	return EligibilityPreviewResponse{EventID: eventID, MatchCount: count, ZeroMatch: count == 0}, nil
}

func (s *Service) updateEligibilityRuleTx(ctx context.Context, tx pgx.Tx, actor Actor, eventID string, rule RuleInput, matchCount int) (int, int, error) {
	nextVersion, err := updateEligibilityRuleVersionTx(ctx, tx, eventID, rule)
	if err != nil {
		return 0, 0, err
	}
	if err := insertEligibilityRuleVersionTx(ctx, tx, eventID, nextVersion, rule, matchCount, actor.ID); err != nil {
		return 0, 0, err
	}
	impactCount, err := s.createEligibilityImpactReviewsTx(ctx, tx, eventID, rule)
	if err != nil {
		return 0, 0, err
	}
	if err := insertEligibilityUpdateAuditTx(ctx, tx, actor, eventID, nextVersion, matchCount, impactCount); err != nil {
		return 0, 0, err
	}
	return nextVersion, impactCount, insertEligibilityImpactOutboxTx(ctx, tx, actor, eventID, impactCount)
}

func updateEligibilityRuleVersionTx(ctx context.Context, tx pgx.Tx, eventID string, rule RuleInput) (int, error) {
	var nextVersion int
	err := tx.QueryRow(ctx, `UPDATE eligibility_rules
		SET department = $1, site = $2, min_grade = $3, employment_status = $4, version = version + 1
		WHERE event_id = $5
		RETURNING version`, rule.Department, rule.Site, rule.MinGrade, rule.EmploymentStatus, eventID).Scan(&nextVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, notFound(errEventNotFoundMessage)
	}
	return nextVersion, err
}

func insertEligibilityUpdateAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, eventID string, nextVersion int, matchCount int, impactCount int) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	return insertAudit(ctx, tx, newAuditRecord(auditID, actor, "eligibility.updated", "event", eventID, map[string]interface{}{"version": nextVersion, "match_count": matchCount, "impact_reviews": impactCount}))
}

func insertEligibilityImpactOutboxTx(ctx context.Context, tx pgx.Tx, actor Actor, eventID string, impactCount int) error {
	if impactCount == 0 {
		return nil
	}
	if err := insertOutbox(ctx, tx, "eligibility.impact_review.created", eventID, map[string]interface{}{"event_id": eventID, "review_count": impactCount}); err != nil {
		return err
	}
	outboxAuditID, err := newID("aud")
	if err != nil {
		return err
	}
	return insertAudit(ctx, tx, newAuditRecord(outboxAuditID, actor, "eligibility_impact.created", "event", eventID, map[string]interface{}{"review_count": impactCount}))
}

func (s *Service) EligibilityImpactReviews(ctx context.Context, actor Actor) ([]EligibilityImpactReview, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT review_id, event_id, employee_id, COALESCE(ticket_id, ''), status, reason, created_at,
			COALESCE(resolved_at, '0001-01-01 00:00:00+00'::timestamptz)
		FROM eligibility_impact_reviews
		ORDER BY created_at DESC
		LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []EligibilityImpactReview
	for rows.Next() {
		var review EligibilityImpactReview
		var employeeID string
		if err := rows.Scan(&review.ReviewID, &review.EventID, &employeeID, &review.TicketID, &review.Status, &review.Reason, &review.CreatedAt, &review.ResolvedAt); err != nil {
			return nil, err
		}
		review.EmployeeRef = maskID(employeeID)
		reviews = append(reviews, review)
	}
	return reviews, rows.Err()
}

func (s *Service) ResolveEligibilityImpactReview(ctx context.Context, actor Actor, reviewID string, req ResolveImpactReviewRequest) (EligibilityImpactReview, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin); err != nil {
		return EligibilityImpactReview{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EligibilityImpactReview{}, err
	}
	defer rollback(ctx, tx)

	var review EligibilityImpactReview
	var employeeID string
	err = tx.QueryRow(ctx, `UPDATE eligibility_impact_reviews
		SET status = 'resolved', resolved_at = now(), reason = CASE WHEN $2 = '' THEN reason ELSE $2 END
		WHERE review_id = $1
		RETURNING review_id, event_id, employee_id, COALESCE(ticket_id, ''), status, reason, created_at, COALESCE(resolved_at, '0001-01-01 00:00:00+00'::timestamptz)`, reviewID, strings.TrimSpace(req.Reason)).
		Scan(&review.ReviewID, &review.EventID, &employeeID, &review.TicketID, &review.Status, &review.Reason, &review.CreatedAt, &review.ResolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EligibilityImpactReview{}, notFound("impact review not found")
	}
	if err != nil {
		return EligibilityImpactReview{}, err
	}
	review.EmployeeRef = maskID(employeeID)
	auditID, err := newID("aud")
	if err != nil {
		return EligibilityImpactReview{}, err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "eligibility_impact.resolved", "eligibility_impact_review", reviewID, map[string]interface{}{"event_id": review.EventID, "employee_ref": review.EmployeeRef})); err != nil {
		return EligibilityImpactReview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EligibilityImpactReview{}, err
	}
	return review, nil
}

func (s *Service) countEligibleEmployees(ctx context.Context, rule RuleInput) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT employee_id, full_name, department, site, job_grade, employment_status FROM employees`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int
	for rows.Next() {
		employee, err := scanEmployee(rows)
		if err != nil {
			return 0, err
		}
		if ok, _ := EvaluateEligibility(employee, EligibilityRule{Department: rule.Department, Site: rule.Site, MinGrade: rule.MinGrade, EmploymentStatus: rule.EmploymentStatus}); ok {
			count++
		}
	}
	return count, rows.Err()
}

func insertEligibilityRuleVersionTx(ctx context.Context, tx pgx.Tx, eventID string, version int, rule RuleInput, matchCount int, actorID string) error {
	versionID, err := newID("elv")
	if err != nil {
		return err
	}
	expression, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO eligibility_rule_versions
		(version_id, event_id, version, expression_json, match_count, created_by)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6)`, versionID, eventID, version, string(expression), matchCount, actorID)
	return err
}

func (s *Service) createEligibilityImpactReviewsTx(ctx context.Context, tx pgx.Tx, eventID string, rule RuleInput) (int, error) {
	type pendingImpact struct {
		employeeID string
		ticketID   string
		reason     string
	}
	rows, err := tx.Query(ctx, `SELECT
			r.employee_id,
			e.full_name,
			e.department,
			e.site,
			e.job_grade,
			e.employment_status,
			COALESCE(t.ticket_id, '')
		FROM registrations r
		JOIN employees e ON e.employee_id = r.employee_id
		LEFT JOIN tickets t ON t.registration_id = r.registration_id AND t.status = 'active'
		WHERE r.event_id = $1
			AND r.status IN ('confirmed', 'waitlisted')
			AND NOT EXISTS (
				SELECT 1 FROM eligibility_impact_reviews pending
				WHERE pending.event_id = r.event_id
					AND pending.employee_id = r.employee_id
					AND pending.status = 'pending'
			)
		ORDER BY r.created_at ASC`, eventID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var pending []pendingImpact
	for rows.Next() {
		var employee Employee
		var ticketID string
		if err := rows.Scan(&employee.EmployeeID, &employee.FullName, &employee.Department, &employee.Site, &employee.JobGrade, &employee.EmploymentStatus, &ticketID); err != nil {
			return 0, err
		}
		eligible, reason := EvaluateEligibility(employee, EligibilityRule{
			Department:       rule.Department,
			Site:             rule.Site,
			MinGrade:         rule.MinGrade,
			EmploymentStatus: rule.EmploymentStatus,
		})
		if eligible {
			continue
		}
		pending = append(pending, pendingImpact{employeeID: employee.EmployeeID, ticketID: ticketID, reason: reason})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()

	var count int
	for _, impact := range pending {
		reviewID, err := newID("rev")
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO eligibility_impact_reviews
			(review_id, event_id, employee_id, ticket_id, status, reason)
			VALUES ($1,$2,$3,NULLIF($4, ''),'pending',$5)`, reviewID, eventID, impact.employeeID, impact.ticketID, impact.reason); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

// maskID returns the first 4 characters of id followed by "****" to
// prevent full employee IDs from appearing in audit metadata.
func maskID(id string) string {
	if len(id) <= 4 {
		return "****"
	}
	return id[:4] + "****"
}
