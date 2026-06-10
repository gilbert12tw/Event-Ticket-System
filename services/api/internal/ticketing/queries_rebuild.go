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

// projectionRebuildLockName keys the advisory lock that serializes a full
// rebuild against in-flight projection worker upserts. Workers take the lock
// shared (pg_advisory_xact_lock_shared) so they never block each other; the
// rebuild takes it exclusive before establishing its snapshot. Without this,
// a worker committing a fresh summary row between the rebuild's DELETE and
// plain re-INSERT fails the whole rebuild on a duplicate key.
const projectionRebuildLockName = "reporting_projection_rebuild"

// acquireProjectionRebuildSharedLockTx takes the rebuild lock in shared mode
// for the duration of a projection worker transaction.
func acquireProjectionRebuildSharedLockTx(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock_shared(hashtext($1)::bigint)`, projectionRebuildLockName)
	return err
}

// rebuildSummaryRow is one aggregated reporting_event_summary row computed
// directly from OLTP. BreakdownJSON is the serialized confirmed-only
// department_breakdown (jsonb) — never decoded here, just passed through.
type rebuildSummaryRow struct {
	EventID        string
	ConfirmedCount int
	CancelledCount int
	WaitlistCount  int
	EmployeeCount  int
	FamilyCount    int
	TicketCount    int
	CheckinCount   int
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
// breakdown from OLTP truth. department_breakdown excludes empty department
// labels, matching the incremental worker's semantics. The query drives from
// events (not registrations) so zero-activity events get genuine zero rows —
// a missing projection row then means "never projected", which the export
// path reports as pending_projection (PH2-45 / WS5-AC-4). Count semantics
// mirror the Phase 1 Reports() query: DISTINCT employees and tickets,
// confirmed-only family sums, check-ins joined through tickets.
func aggregateFromOLTP(ctx context.Context, tx pgx.Tx) ([]rebuildSummaryRow, error) {
	rows, err := tx.Query(ctx, `
		WITH counts AS (
			SELECT event_id,
			       COUNT(*) FILTER (WHERE status = 'confirmed')  AS confirmed_count,
			       COUNT(*) FILTER (WHERE status = 'cancelled')  AS cancelled_count,
			       COUNT(*) FILTER (WHERE status = 'waitlisted') AS waitlist_count,
			       COUNT(DISTINCT employee_id) FILTER (WHERE status = 'confirmed') AS employee_count,
			       COALESCE(SUM(family_count) FILTER (WHERE status = 'confirmed'), 0) AS family_count
			FROM registrations
			GROUP BY event_id
		),
		tix AS (
			SELECT event_id, COUNT(DISTINCT ticket_id) AS ticket_count
			FROM tickets
			GROUP BY event_id
		),
		chk AS (
			SELECT t.event_id, COUNT(DISTINCT c.checkin_id) AS checkin_count
			FROM tickets t
			JOIN checkin_records c ON c.ticket_id = t.ticket_id
			GROUP BY t.event_id
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
		SELECT e.event_id,
		       COALESCE(c.confirmed_count, 0), COALESCE(c.cancelled_count, 0),
		       COALESCE(c.waitlist_count, 0), COALESCE(c.employee_count, 0),
		       COALESCE(c.family_count, 0),
		       COALESCE(tix.ticket_count, 0), COALESCE(chk.checkin_count, 0),
		       COALESCE(dept.department_breakdown, '{}'::jsonb)
		FROM events e
		LEFT JOIN counts c ON c.event_id = e.event_id
		LEFT JOIN tix ON tix.event_id = e.event_id
		LEFT JOIN chk ON chk.event_id = e.event_id
		LEFT JOIN dept ON dept.event_id = e.event_id
		ORDER BY e.event_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []rebuildSummaryRow
	for rows.Next() {
		var row rebuildSummaryRow
		if err := rows.Scan(&row.EventID, &row.ConfirmedCount, &row.CancelledCount,
			&row.WaitlistCount, &row.EmployeeCount, &row.FamilyCount,
			&row.TicketCount, &row.CheckinCount, &row.BreakdownJSON); err != nil {
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
			 employee_count, family_count, ticket_count, checkin_count,
			 department_breakdown, last_event_offset, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, now())`,
		row.EventID, row.ConfirmedCount, row.CancelledCount, row.WaitlistCount,
		row.EmployeeCount, row.FamilyCount, row.TicketCount, row.CheckinCount,
		row.BreakdownJSON, offset)
	return err
}

// rebuildWatermark returns the composite projectionOffsetKey of the newest
// projection envelope event at snapshot time, or `""` when none exists. It is
// the authoritative offset stamped into both
// reporting_event_summary.last_event_offset and the
// reporting_projection_offsets watermark after a full rebuild.
//
// Only reporting.projection.update_required.v2 rows participate: the watermark
// exists solely to suppress projection events already reflected in the OLTP
// aggregate. A newer outbox row of any other type (notification, export)
// would extend the watermark past projection events that are still pending,
// and the worker's upsert guard would then drop their updates permanently.
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
		WHERE event_type = $1
		ORDER BY created_at DESC, outbox_id DESC
		LIMIT 1`, outboxEventReportingProjectionUpdateRequiredV2).Scan(&createdAt, &outboxID)
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
func resetProjectionOffset(ctx context.Context, tx pgx.Tx, projectionName, offset string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO reporting_projection_offsets
			(projection_name, last_processed_outbox_id, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_outbox_id = excluded.last_processed_outbox_id,
			updated_at = now()`,
		projectionName, offset)
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
