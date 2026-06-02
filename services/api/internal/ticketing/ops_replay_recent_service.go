package ticketing

import (
	"context"
	"database/sql"
	"strings"
)

const defaultRecentReplayResultLimit = 5

func (s *Service) recentOutboxReplays(ctx context.Context) ([]OutboxReplayRecentRow, error) {
	rows, err := s.db.Query(ctx, `SELECT audit_id,
			role,
			COALESCE(NULLIF(metadata->>'kind', ''), $3) AS kind,
			CASE
				WHEN lower(metadata->>'dry_run') IN ('true', 'false') THEN (metadata->>'dry_run')::boolean
				ELSE false
			END AS dry_run,
			CASE
				WHEN metadata->>'affected_count' ~ '^[0-9]+$' THEN (metadata->>'affected_count')::int
				ELSE 0
			END AS affected_count,
			CASE
				WHEN metadata->>'enqueued_count' ~ '^[0-9]+$' THEN (metadata->>'enqueued_count')::int
				ELSE NULL
			END AS enqueued_count,
			created_at
		FROM audit_logs
		WHERE action = $1
		ORDER BY created_at DESC, audit_id DESC
		LIMIT $2`, outboxReplayAction, defaultRecentReplayResultLimit, outboxWorkerKindUnknown)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	replays := []OutboxReplayRecentRow{}
	for rows.Next() {
		row, err := scanOutboxReplayRecentRow(rows)
		if err != nil {
			return nil, err
		}
		replays = append(replays, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return replays, nil
}

type outboxReplayRecentScanner interface {
	Scan(dest ...interface{}) error
}

func scanOutboxReplayRecentRow(scanner outboxReplayRecentScanner) (OutboxReplayRecentRow, error) {
	var row OutboxReplayRecentRow
	var enqueuedCount sql.NullInt64
	if err := scanner.Scan(
		&row.AuditID,
		&row.ActorRole,
		&row.Kind,
		&row.DryRun,
		&row.AffectedCount,
		&enqueuedCount,
		&row.CreatedAt,
	); err != nil {
		return OutboxReplayRecentRow{}, err
	}
	row.Kind = safeReplayWorkerKind(row.Kind)
	if enqueuedCount.Valid {
		value := int(enqueuedCount.Int64)
		row.EnqueuedCount = &value
	}
	return row, nil
}

func safeReplayWorkerKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == outboxWorkerKindReservationCompensation {
		return outboxWorkerKindCompensation
	}
	switch kind {
	case outboxWorkerKindNotification,
		outboxWorkerKindProjection,
		outboxWorkerKindCompensation,
		outboxWorkerKindExport:
		return kind
	default:
		return outboxWorkerKindUnknown
	}
}
