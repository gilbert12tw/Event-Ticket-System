package ticketing

import (
	"context"
	"strings"
)

func (s *Service) RunHRSync(ctx context.Context, actor Actor, req HRSyncRequest) (HRSyncBatch, error) {
	if err := requireRole(actor, RoleHRAdmin); err != nil {
		return HRSyncBatch{}, err
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "manual"
	}

	batchID, err := newID("hrs")
	if err != nil {
		return HRSyncBatch{}, err
	}
	now := s.now()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return HRSyncBatch{}, err
	}
	defer rollback(ctx, tx)

	if _, err := tx.Exec(ctx, `INSERT INTO hr_sync_batches (batch_id, source, status, employee_count, started_at, completed_at)
		VALUES ($1, $2, 'pending', 0, $3, NULL)`, batchID, source, now); err != nil {
		return HRSyncBatch{}, err
	}

	rows, err := tx.Query(ctx, `SELECT e.event_id, r.department, r.site, r.min_grade, r.employment_status
		FROM events e
		JOIN eligibility_rules r ON r.event_id = e.event_id
		WHERE e.archived_at IS NULL
		ORDER BY e.created_at DESC`)
	if err != nil {
		return HRSyncBatch{}, err
	}

	eventRules := []struct {
		eventID string
		rule    RuleInput
	}{}
	defer rows.Close()
	for rows.Next() {
		var eventRule struct {
			eventID string
			rule    RuleInput
		}
		if err := rows.Scan(
			&eventRule.eventID,
			&eventRule.rule.Department,
			&eventRule.rule.Site,
			&eventRule.rule.MinGrade,
			&eventRule.rule.EmploymentStatus,
		); err != nil {
			return HRSyncBatch{}, err
		}
		eventRules = append(eventRules, eventRule)
	}
	if err := rows.Err(); err != nil {
		return HRSyncBatch{}, err
	}
	rows.Close()

	totalImpacted := 0
	for _, eventRule := range eventRules {
		normalizedRule := normalizeRuleInput(eventRule.rule)
		impacted, err := s.createEligibilityImpactReviewsTx(ctx, tx, eventRule.eventID, normalizedRule)
		if err != nil {
			return HRSyncBatch{}, err
		}
		if impacted == 0 {
			continue
		}
		totalImpacted += impacted
		eventAuditID, err := newID("aud")
		if err != nil {
			return HRSyncBatch{}, err
		}
		if err := insertAudit(ctx, tx, eventAuditID, actor, "eligibility_impact.created", "event", eventRule.eventID, map[string]interface{}{
			"batch_id":     batchID,
			"source":       source,
			"review_count": impacted,
		}); err != nil {
			return HRSyncBatch{}, err
		}
		if err := insertOutbox(ctx, tx, "eligibility.impact_review.created", eventRule.eventID, map[string]interface{}{
			"batch_id":     batchID,
			"event_id":     eventRule.eventID,
			"review_count": impacted,
			"source":       source,
		}); err != nil {
			return HRSyncBatch{}, err
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE hr_sync_batches SET status = 'applied', employee_count = $2, completed_at = $3 WHERE batch_id = $1`, batchID, totalImpacted, s.now()); err != nil {
		return HRSyncBatch{}, err
	}

	batchAuditID, err := newID("aud")
	if err != nil {
		return HRSyncBatch{}, err
	}
	if err := insertAudit(ctx, tx, batchAuditID, actor, "hr_sync.completed", "hr_sync_batch", batchID, map[string]interface{}{
		"batch_id":       batchID,
		"source":         source,
		"employee_count": totalImpacted,
	}); err != nil {
		return HRSyncBatch{}, err
	}
	if err := insertOutbox(ctx, tx, "hr_sync.completed", batchID, map[string]interface{}{
		"source":         source,
		"employee_count": totalImpacted,
		"review_count":   totalImpacted,
	}); err != nil {
		return HRSyncBatch{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return HRSyncBatch{}, err
	}

	return HRSyncBatch{
		BatchID:       batchID,
		Source:        source,
		Status:        "applied",
		EmployeeCount: totalImpacted,
		StartedAt:     now,
		CompletedAt:   s.now(),
	}, nil
}
