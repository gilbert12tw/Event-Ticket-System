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
	counts eventSummaryRow,
	outboxID string,
) error {
	breakdownJSON, err := json.Marshal(counts.DepartmentBreakdown)
	if err != nil {
		return fmt.Errorf("projection: marshal department_breakdown: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO reporting_event_summary
			(event_id, confirmed_count, cancelled_count, waitlist_count,
			 employee_count, family_count, ticket_count, checkin_count,
			 department_breakdown, last_event_offset, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, now())
		ON CONFLICT (event_id) DO UPDATE SET
			confirmed_count = excluded.confirmed_count,
			cancelled_count = excluded.cancelled_count,
			waitlist_count = excluded.waitlist_count,
			employee_count = excluded.employee_count,
			family_count = excluded.family_count,
			ticket_count = excluded.ticket_count,
			checkin_count = excluded.checkin_count,
			department_breakdown = excluded.department_breakdown,
			last_event_offset = excluded.last_event_offset,
			updated_at = now()
		WHERE excluded.last_event_offset > reporting_event_summary.last_event_offset`,
		eventID,
		max(counts.ConfirmedCount, 0),
		max(counts.CancelledCount, 0),
		max(counts.WaitlistCount, 0),
		max(counts.EmployeeCount, 0),
		max(counts.FamilyCount, 0),
		max(counts.TicketCount, 0),
		max(counts.CheckinCount, 0),
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
		       employee_count, family_count, ticket_count, checkin_count,
		       department_breakdown, COALESCE(last_event_offset, '')
		FROM reporting_event_summary
		WHERE event_id = $1`, eventID).
		Scan(
			&row.ConfirmedCount,
			&row.CancelledCount,
			&row.WaitlistCount,
			&row.EmployeeCount,
			&row.FamilyCount,
			&row.TicketCount,
			&row.CheckinCount,
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
