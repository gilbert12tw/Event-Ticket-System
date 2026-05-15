package ticketing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type NoShowProcessingResult struct {
	Processed        int `json:"processed"`
	CooldownsApplied int `json:"cooldowns_applied"`
}

func (s *Service) ProcessNoShows(ctx context.Context, actor Actor) (NoShowProcessingResult, error) {
	if err := requireRole(actor, RoleSystemAdmin); err != nil {
		return NoShowProcessingResult{}, err
	}
	policy := s.noShowPolicy.Normalize()
	cutoff := s.now().Add(-policy.GracePeriod)
	rows, err := s.db.Query(ctx, `SELECT r.registration_id, r.event_id, r.employee_id
		FROM registrations r
		JOIN events e ON e.event_id = r.event_id
		LEFT JOIN tickets t ON t.registration_id = r.registration_id
		LEFT JOIN checkin_records c ON c.ticket_id = t.ticket_id AND c.status = 'accepted'
		WHERE r.status = 'confirmed'
			AND e.capacity_type = 'limited'
			AND e.starts_at <= $1
			AND c.checkin_id IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM no_show_records n WHERE n.registration_id = r.registration_id
			)
		ORDER BY e.starts_at ASC, r.created_at ASC`, cutoff)
	if err != nil {
		return NoShowProcessingResult{}, err
	}
	defer rows.Close()

	type candidate struct {
		registrationID string
		eventID        string
		employeeID     string
	}
	var candidates []candidate
	for rows.Next() {
		var next candidate
		if err := rows.Scan(&next.registrationID, &next.eventID, &next.employeeID); err != nil {
			return NoShowProcessingResult{}, err
		}
		candidates = append(candidates, next)
	}
	if err := rows.Err(); err != nil {
		return NoShowProcessingResult{}, err
	}

	var result NoShowProcessingResult
	for _, next := range candidates {
		recorded, applied, err := s.recordNoShow(ctx, actor, next.registrationID, next.eventID, next.employeeID, policy)
		if err != nil {
			return NoShowProcessingResult{}, err
		}
		if !recorded {
			continue
		}
		result.Processed++
		if applied {
			result.CooldownsApplied++
		}
	}
	return result, nil
}

func (s *Service) activeNoShowCooldownTx(ctx context.Context, tx pgx.Tx, employeeID string, now time.Time) (time.Time, bool, error) {
	var cooldownUntil time.Time
	err := tx.QueryRow(ctx, `SELECT cooldown_until
		FROM no_show_records
		WHERE employee_id = $1
			AND status = 'cooldown_active'
			AND cooldown_until IS NOT NULL
			AND cooldown_until > $2
		ORDER BY cooldown_until DESC
		LIMIT 1`, employeeID, now).Scan(&cooldownUntil)
	if err == pgx.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return cooldownUntil, true, nil
}

func (s *Service) recordNoShow(ctx context.Context, actor Actor, registrationID string, eventID string, employeeID string, policy NoShowPolicy) (bool, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, false, err
	}
	defer rollback(ctx, tx)

	var previousCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM no_show_records WHERE employee_id = $1`, employeeID).Scan(&previousCount); err != nil {
		return false, false, err
	}
	appliesCooldown := previousCount+1 >= policy.Threshold
	status := "recorded"
	var cooldownUntil *time.Time
	if appliesCooldown {
		status = "cooldown_active"
		until := s.now().Add(policy.CooldownDuration).UTC()
		cooldownUntil = &until
	}
	noShowID, err := newID("nsh")
	if err != nil {
		return false, false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO no_show_records
		(no_show_id, registration_id, event_id, employee_id, status, recorded_at, cooldown_until)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (registration_id) DO NOTHING`,
		noShowID, registrationID, eventID, employeeID, status, s.now(), cooldownUntil)
	if err != nil {
		return false, false, err
	}
	if tag.RowsAffected() == 0 {
		return false, false, nil
	}
	auditID, err := newID("aud")
	if err != nil {
		return false, false, err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "registration.no_show_recorded", "registration", registrationID, map[string]interface{}{
		"event_id":        eventID,
		"cooldown_until":  cooldownUntil,
		"cooldown_status": status,
	}); err != nil {
		return false, false, err
	}
	if err := insertOutbox(ctx, tx, "registration.no_show_recorded", registrationID, map[string]interface{}{
		"registration_id": registrationID,
		"event_id":        eventID,
		"employee_id":     employeeID,
		"cooldown_until":  cooldownUntil,
	}); err != nil {
		return false, false, err
	}
	return true, appliesCooldown, tx.Commit(ctx)
}
