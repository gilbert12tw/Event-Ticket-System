package ticketing

import (
	"context"
	"database/sql"
)

func (s *Service) OutboxQueueStatus(ctx context.Context, actor Actor) (OutboxQueueStatus, error) {
	if err := requireRole(actor, RoleHRAdmin); err != nil {
		return OutboxQueueStatus{}, err
	}
	status := OutboxQueueStatus{Queues: defaultOutboxQueueStatusRows()}
	index := map[string]int{}
	for i, row := range status.Queues {
		index[row.Name] = i
	}
	rows, err := s.db.Query(ctx, `WITH classified AS (
			SELECT CASE
					WHEN event_type = ANY($1::text[]) THEN 'notification'
					WHEN event_type = ANY($2::text[]) THEN 'projection'
					WHEN event_type = ANY($3::text[]) THEN 'compensation'
					WHEN event_type = ANY($4::text[]) THEN 'export'
					ELSE 'unknown'
				END AS worker_kind,
				publish_status,
				created_at,
				published_at
			FROM outbox_events
		)
		SELECT
			CASE
				WHEN worker_kind IN ('notification', 'projection', 'compensation', 'export') THEN worker_kind
				ELSE 'unknown'
			END,
			count(*) FILTER (WHERE publish_status = 'pending')::int,
			count(*) FILTER (WHERE publish_status = 'processing')::int,
			count(*) FILTER (WHERE publish_status = 'dead_letter')::int,
			COALESCE(ceil(percentile_cont(0.95) WITHIN GROUP (
				ORDER BY EXTRACT(EPOCH FROM (now() - created_at))::double precision
			) FILTER (WHERE publish_status IN ('pending', 'processing', 'dead_letter'))), 0)::int,
			max(published_at) FILTER (WHERE publish_status = 'published')
		FROM classified
		GROUP BY worker_kind`,
		replayEventTypesForKind(outboxWorkerKindNotification),
		replayEventTypesForKind(outboxWorkerKindProjection),
		replayEventTypesForKind(outboxWorkerKindCompensation),
		replayEventTypesForKind(outboxWorkerKindExport))
	if err != nil {
		return OutboxQueueStatus{}, err
	}
	defer rows.Close()
	for rows.Next() {
		row, err := scanOutboxQueueStatusRow(rows)
		if err != nil {
			return OutboxQueueStatus{}, err
		}
		if position, ok := index[row.Name]; ok {
			status.Queues[position] = row
		}
	}
	if err := rows.Err(); err != nil {
		return OutboxQueueStatus{}, err
	}
	return status, nil
}

type outboxQueueStatusScanner interface {
	Scan(dest ...interface{}) error
}

func scanOutboxQueueStatusRow(row outboxQueueStatusScanner) (OutboxQueueStatusRow, error) {
	var got OutboxQueueStatusRow
	var lastProcessedAt sql.NullTime
	err := row.Scan(&got.Name, &got.Pending, &got.InFlight, &got.DeadLetter, &got.P95AgeSeconds, &lastProcessedAt)
	if err != nil {
		return OutboxQueueStatusRow{}, err
	}
	if lastProcessedAt.Valid {
		at := lastProcessedAt.Time
		got.LastProcessedAt = &at
	}
	return got, nil
}

func defaultOutboxQueueStatusRows() []OutboxQueueStatusRow {
	return []OutboxQueueStatusRow{
		{Name: outboxWorkerKindNotification},
		{Name: outboxWorkerKindProjection},
		{Name: outboxWorkerKindCompensation},
		{Name: outboxWorkerKindExport},
		{Name: outboxWorkerKindUnknown},
	}
}
