package ticketing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PH2-43 read-model rebuild SQL helpers.
//
// These queries rebuild reporting_event_summary from PostgreSQL OLTP truth
// (registrations + employees) rather than by replaying outbox history. They
// contain SQL only — all orchestration lives in rebuild_projection_service.go.
//
// Source of truth: registrations.status and employees.department. No OLTP
// table is ever modified here; the projection tables are the only output.

// rebuildSummaryRow is one aggregated reporting_event_summary row computed
// directly from OLTP. BreakdownJSON is the serialized confirmed-only
// department_breakdown (jsonb) — never decoded here, just passed through.
type rebuildSummaryRow struct {
	EventID        string
	ConfirmedCount int
	CancelledCount int
	WaitlistCount  int
	BreakdownJSON  []byte
}

// clearEventSummary clears reporting_event_summary inside the rebuild
// transaction so a rollback restores the pre-rebuild state. DELETE (not
// TRUNCATE) is used deliberately: the table holds one tiny row per event, and
// DELETE takes only a ROW EXCLUSIVE lock, so concurrent reporting reads and the
// projection worker are not blocked by an ACCESS EXCLUSIVE lock.
func clearEventSummary(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `DELETE FROM reporting_event_summary`)
	return err
}

// aggregateFromOLTP computes per-event counts and a confirmed-only department
// breakdown from registrations + employees. department_breakdown excludes empty
// department labels, matching the incremental worker's semantics.
func aggregateFromOLTP(ctx context.Context, tx pgx.Tx) ([]rebuildSummaryRow, error) {
	rows, err := tx.Query(ctx, `
		WITH counts AS (
			SELECT event_id,
			       COUNT(*) FILTER (WHERE status = 'confirmed')  AS confirmed_count,
			       COUNT(*) FILTER (WHERE status = 'cancelled')  AS cancelled_count,
			       COUNT(*) FILTER (WHERE status = 'waitlisted') AS waitlist_count
			FROM registrations
			GROUP BY event_id
		),
		dept AS (
			SELECT event_id,
			       COALESCE(jsonb_object_agg(department, cnt), '{}'::jsonb) AS department_breakdown
			FROM (
				SELECT r.event_id, emp.department, COUNT(*) AS cnt
				FROM registrations r
				JOIN employees emp ON emp.employee_id = r.employee_id
				WHERE r.status = 'confirmed' AND emp.department <> ''
				GROUP BY r.event_id, emp.department
			) d
			GROUP BY event_id
		)
		SELECT c.event_id, c.confirmed_count, c.cancelled_count, c.waitlist_count,
		       COALESCE(dept.department_breakdown, '{}'::jsonb)
		FROM counts c
		LEFT JOIN dept ON dept.event_id = c.event_id
		ORDER BY c.event_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []rebuildSummaryRow
	for rows.Next() {
		var row rebuildSummaryRow
		if err := rows.Scan(&row.EventID, &row.ConfirmedCount, &row.CancelledCount,
			&row.WaitlistCount, &row.BreakdownJSON); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// insertRebuiltEventSummary writes one aggregated row. last_event_offset is set
// to the rebuild watermark (the composite projectionOffsetKey of the newest
// outbox event at snapshot time), NOT the empty sentinel: the projection worker
// claims outbox rows by publish_status, not by the offset, so any still-pending
// event would otherwise re-apply on top of the already-correct OLTP counts.
// Stamping the watermark makes the worker's guard
// (excluded.last_event_offset > row.last_event_offset) suppress every event at
// or below the watermark, and apply only genuinely newer events. The watermark
// MUST use the same composite format the worker writes (see projectionOffsetKey)
// or the two become lexicographically incomparable. total_capacity is left at
// its column default.
func insertRebuiltEventSummary(ctx context.Context, tx pgx.Tx, row rebuildSummaryRow, offset string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO reporting_event_summary
			(event_id, confirmed_count, cancelled_count, waitlist_count,
			 department_breakdown, last_event_offset, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, now())`,
		row.EventID, row.ConfirmedCount, row.CancelledCount, row.WaitlistCount, row.BreakdownJSON, offset)
	return err
}

// rebuildWatermark returns the composite projectionOffsetKey of the newest
// outbox event at snapshot time, or `""` when the outbox is empty. It is the
// authoritative offset stamped into both reporting_event_summary.last_event_offset
// and the reporting_projection_offsets watermark after a full rebuild.
//
// outbox_id is a random out_<hex> value and is NOT time-sortable, so the newest
// event is selected by (created_at, outbox_id) — matching the worker's composite
// key tiebreak — and formatted with the SAME helper the worker uses, keeping the
// two offset representations lexicographically comparable under the upsert guard.
func rebuildWatermark(ctx context.Context, tx pgx.Tx) (string, error) {
	var createdAt time.Time
	var outboxID string
	err := tx.QueryRow(ctx, `
		SELECT created_at, outbox_id
		FROM outbox_events
		ORDER BY created_at DESC, outbox_id DESC
		LIMIT 1`).Scan(&createdAt, &outboxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return projectionOffsetKey(createdAt, outboxID), nil
}

// resetProjectionOffset force-sets last_processed_outbox_id for a projection to
// the rebuild watermark (not GREATEST — rebuild is an authoritative reset).
func resetProjectionOffset(ctx context.Context, tx pgx.Tx, projectionName, outboxID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO reporting_projection_offsets
			(projection_name, last_processed_outbox_id, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_outbox_id = excluded.last_processed_outbox_id,
			updated_at = now()`,
		projectionName, outboxID)
	return err
}

// confirmedCountForEvent reads the OLTP confirmed registration count for one
// event — the source-of-truth side of spot-check validation.
func confirmedCountForEvent(ctx context.Context, tx pgx.Tx, eventID string) (int, error) {
	var count int
	err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`,
		eventID).Scan(&count)
	return count, err
}
