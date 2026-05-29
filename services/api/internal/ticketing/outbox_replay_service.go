package ticketing

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

const outboxReplayAction = "outbox.replay"
const maxReplayAuditRows = 100

func (s *Service) ReplayOutbox(ctx context.Context, actor Actor, req ReplayOutboxRequest) (ReplayOutboxResult, error) {
	if err := requireRole(actor, RoleHRAdmin); err != nil {
		return ReplayOutboxResult{}, err
	}
	req, err := ValidateReplayOutboxRequest(req)
	if err != nil {
		return ReplayOutboxResult{}, err
	}
	result := ReplayOutboxResult{
		Kind:   req.Kind,
		From:   req.From,
		To:     req.To,
		DryRun: req.DryRun,
	}
	where, args := replayOutboxWhere(req)
	countSQL := fmt.Sprintf(`SELECT count(*) FROM outbox_events WHERE %s`, where)
	if req.DryRun {
		if err := s.db.QueryRow(ctx, countSQL, args...).Scan(&result.AffectedCount); err != nil {
			return ReplayOutboxResult{}, err
		}
		s.logReplayOutboxResult(ctx, actor, result)
		return result, nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReplayOutboxResult{}, err
	}
	defer rollback(ctx, tx)
	replayedRows, replayedRowsTruncated, err := loadReplayAuditRows(ctx, tx, where, args)
	if err != nil {
		return ReplayOutboxResult{}, err
	}
	updateSQL := fmt.Sprintf(`UPDATE outbox_events
		SET publish_status = 'pending',
			attempts = 0,
			retry_count = 0,
			available_at = now(),
			last_error = '',
			lease_started_at = NULL,
			dead_letter_at = NULL
		WHERE %s`, where)
	tag, err := tx.Exec(ctx, updateSQL, args...)
	if err != nil {
		return ReplayOutboxResult{}, err
	}
	enqueued := int(tag.RowsAffected())
	result.AffectedCount = enqueued
	result.EnqueuedCount = &enqueued
	auditID, err := newID("aud")
	if err != nil {
		return ReplayOutboxResult{}, err
	}
	result.AuditID = auditID
	metadata := replayAuditMetadata(req, result, replayedRows, replayedRowsTruncated)
	if err := insertAudit(ctx, tx, auditID, actor, outboxReplayAction, "outbox_queue", req.Kind, metadata); err != nil {
		return ReplayOutboxResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReplayOutboxResult{}, err
	}
	s.logReplayOutboxResult(ctx, actor, result)
	return result, nil
}

func (s *Service) logReplayOutboxResult(ctx context.Context, actor Actor, result ReplayOutboxResult) {
	if s == nil || s.logger == nil {
		return
	}
	message := "outbox replay applied"
	if result.DryRun {
		message = "outbox replay dry-run"
	}
	attrs := []interface{}{
		"trace_id", traceid.FromContext(ctx),
		"action", outboxReplayAction,
		"actor_role", actor.Role,
		"kind", result.Kind,
		"dry_run", result.DryRun,
		"affected_count", result.AffectedCount,
	}
	if result.EnqueuedCount != nil {
		attrs = append(attrs, "enqueued_count", *result.EnqueuedCount)
	}
	if result.AuditID != "" {
		attrs = append(attrs, "audit_id", result.AuditID)
	}
	s.logger.Info(message, attrs...)
}

func replayOutboxWhere(req ReplayOutboxRequest) (string, []interface{}) {
	eventTypes := replayEventTypesForKind(req.Kind)
	if len(req.EventTypes) > 0 {
		eventTypes = req.EventTypes
	}
	args := []interface{}{req.From, req.To, replaySQLWorkerKind(req.Kind), eventTypes}
	parts := []string{
		"created_at >= $1",
		"created_at <= $2",
		"publish_status = 'dead_letter'",
		fmt.Sprintf("event_type = ANY($%d::text[])", len(args)),
		`CASE
			WHEN event_type IN ('report.export.requested', 'report.export.requested.v2') THEN 'export'
			WHEN event_type = 'reporting.projection.update_required.v2' THEN 'projection'
			WHEN event_type LIKE 'reservation.compensation.%' THEN 'compensation'
			ELSE 'notification'
		END = $3`,
	}
	return strings.Join(parts, " AND "), args
}

type replayAuditRow struct {
	OutboxID             string `json:"outbox_id"`
	EventType            string `json:"event_type"`
	WorkerKind           string `json:"worker_kind"`
	PreviousStatus       string `json:"previous_status"`
	PreviousAttempts     int    `json:"previous_attempts"`
	PreviousRetryCount   int    `json:"previous_retry_count"`
	PreviousLastError    string `json:"previous_last_error,omitempty"`
	PreviousDeadLetterAt string `json:"previous_dead_letter_at,omitempty"`
}

func loadReplayAuditRows(ctx context.Context, tx pgx.Tx, where string, args []interface{}) ([]replayAuditRow, bool, error) {
	query := fmt.Sprintf(`SELECT outbox_id, event_type, publish_status, attempts, retry_count, last_error, dead_letter_at
		FROM outbox_events
		WHERE %s
		ORDER BY created_at, outbox_id
		LIMIT %d`, where, maxReplayAuditRows+1)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	auditRows := []replayAuditRow{}
	truncated := false
	for rows.Next() {
		if len(auditRows) >= maxReplayAuditRows {
			truncated = true
			continue
		}
		var row replayAuditRow
		var eventType string
		var lastError string
		var deadLetterAt sql.NullTime
		if err := rows.Scan(
			&row.OutboxID,
			&eventType,
			&row.PreviousStatus,
			&row.PreviousAttempts,
			&row.PreviousRetryCount,
			&lastError,
			&deadLetterAt,
		); err != nil {
			return nil, false, err
		}
		row.EventType = safeOutboxTelemetryEventType(eventType)
		row.WorkerKind = safeOutboxTelemetryWorkerKind(eventType)
		row.PreviousLastError = redactReplayAuditLastError(lastError)
		if deadLetterAt.Valid {
			row.PreviousDeadLetterAt = deadLetterAt.Time.UTC().Format(pgTimestampFormat)
		}
		auditRows = append(auditRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return auditRows, truncated, nil
}

func redactReplayAuditLastError(raw string) string {
	message := redactNotificationDeliveryError(raw, "")
	return outboxTelemetryEmployeePattern.ReplaceAllStringFunc(message, maskID)
}

func replayAuditMetadata(
	req ReplayOutboxRequest,
	result ReplayOutboxResult,
	replayedRows []replayAuditRow,
	replayedRowsTruncated bool,
) map[string]interface{} {
	enqueuedCount := 0
	if result.EnqueuedCount != nil {
		enqueuedCount = *result.EnqueuedCount
	}
	return map[string]interface{}{
		"kind":                    req.Kind,
		"from":                    req.From.Format(pgTimestampFormat),
		"to":                      req.To.Format(pgTimestampFormat),
		"event_types":             req.EventTypes,
		"dry_run":                 req.DryRun,
		"affected_count":          result.AffectedCount,
		"enqueued_count":          enqueuedCount,
		"replayed_rows":           replayedRows,
		"replayed_row_count":      result.AffectedCount,
		"replayed_rows_truncated": replayedRowsTruncated || len(replayedRows) < result.AffectedCount,
	}
}

const pgTimestampFormat = "2006-01-02T15:04:05Z07:00"
