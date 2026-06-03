package ticketing

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// upsertEventSummary performs an idempotent upsert of aggregate counts for a
// single event.  The ON CONFLICT guard ensures that replaying an older outbox
// event never overwrites a newer projection state.
//
// Counts are supplied as absolute values derived from computeNewCounts; the
// SQL GREATEST guard prevents them from going below 0.
func upsertEventSummary(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
	confirmed, cancelled, waitlist int,
	breakdown map[string]int,
	outboxID string,
) error {
	breakdownJSON, err := json.Marshal(breakdown)
	if err != nil {
		return fmt.Errorf("projection: marshal department_breakdown: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO reporting_event_summary
			(event_id, confirmed_count, cancelled_count, waitlist_count,
			 department_breakdown, last_event_offset, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, now())
		ON CONFLICT (event_id) DO UPDATE SET
			confirmed_count = CASE
				WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
				THEN excluded.confirmed_count
				ELSE reporting_event_summary.confirmed_count END,
			cancelled_count = CASE
				WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
				THEN excluded.cancelled_count
				ELSE reporting_event_summary.cancelled_count END,
			waitlist_count = CASE
				WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
				THEN excluded.waitlist_count
				ELSE reporting_event_summary.waitlist_count END,
			department_breakdown = CASE
				WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
				THEN excluded.department_breakdown
				ELSE reporting_event_summary.department_breakdown END,
			last_event_offset = GREATEST(
				excluded.last_event_offset,
				reporting_event_summary.last_event_offset),
			updated_at = CASE
				WHEN excluded.last_event_offset > reporting_event_summary.last_event_offset
				THEN now()
				ELSE reporting_event_summary.updated_at END`,
		eventID,
		max(confirmed, 0),
		max(cancelled, 0),
		max(waitlist, 0),
		string(breakdownJSON),
		outboxID,
	)
	return err
}

// advanceProjectionOffset advances the last_processed_outbox_id watermark for
// a named projection. Called after a successful upsert so crash recovery
// resumes from the right position.
func advanceProjectionOffset(ctx context.Context, tx pgx.Tx, projectionName string, outboxID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO reporting_projection_offsets
			(projection_name, last_processed_outbox_id, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_outbox_id = GREATEST(
				excluded.last_processed_outbox_id,
				reporting_projection_offsets.last_processed_outbox_id),
			updated_at = now()`,
		projectionName, outboxID,
	)
	return err
}

// getProjectionOffset returns the last_processed_outbox_id for a named
// projection. Returns "" when no row exists yet (first run).
func getProjectionOffset(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}, projectionName string) (string, error) {
	var offset string
	err := db.QueryRow(ctx,
		`SELECT COALESCE(last_processed_outbox_id, '') FROM reporting_projection_offsets WHERE projection_name = $1`,
		projectionName,
	).Scan(&offset)
	if err != nil {
		return "", err
	}
	return offset, nil
}

// getEventSummaryRow reads the current aggregate state for an event.
// Returns a zero-value row when no row exists yet.
// Accepts any type that implements QueryRow (pgx.Tx or *pgxpool.Pool).
func getEventSummaryRow(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}, eventID string) (eventSummaryRow, error) {
	var row eventSummaryRow
	var breakdownJSON []byte
	err := db.QueryRow(ctx, `
		SELECT confirmed_count, cancelled_count, waitlist_count,
		       department_breakdown, COALESCE(last_event_offset, '')
		FROM reporting_event_summary
		WHERE event_id = $1`, eventID).
		Scan(
			&row.ConfirmedCount,
			&row.CancelledCount,
			&row.WaitlistCount,
			&breakdownJSON,
			&row.LastEventOffset,
		)
	if err == pgx.ErrNoRows {
		row.DepartmentBreakdown = map[string]int{}
		return row, nil
	}
	if err != nil {
		return eventSummaryRow{}, err
	}
	if err := json.Unmarshal(breakdownJSON, &row.DepartmentBreakdown); err != nil {
		row.DepartmentBreakdown = map[string]int{}
	}
	return row, nil
}
