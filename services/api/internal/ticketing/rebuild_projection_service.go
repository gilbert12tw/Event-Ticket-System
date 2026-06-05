package ticketing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rebuildActorID = "admin-rebuild-projection"

// RebuildOptions configures a projection rebuild run. Values come from env
// config (REBUILD_DRY_RUN / REBUILD_SAMPLE_VALIDATE); never hardcode them.
type RebuildOptions struct {
	DryRun         bool
	SampleValidate bool
}

// RebuildResult summarizes a completed (or dry-run) rebuild.
type RebuildResult struct {
	RowsInserted  int
	OffsetResetTo string
	Validated     bool
	DryRun        bool
}

// sampleValidator re-checks a sample of rebuilt rows against OLTP truth inside
// the rebuild transaction. It is injectable so the rollback-on-mismatch path
// can be exercised without a mock framework.
type sampleValidator func(ctx context.Context, tx pgx.Tx) error

// RebuildProjection rebuilds reporting_event_summary from OLTP source tables
// (registrations + employees) inside a single transaction, then resets the
// projection offset to the current outbox head. It reads OLTP as truth and
// writes only projection/audit tables. No durable local state is written.
func RebuildProjection(ctx context.Context, pool *pgxpool.Pool, opts RebuildOptions) (RebuildResult, error) {
	return rebuildProjection(ctx, pool, opts, sampleValidateConfirmed)
}

func rebuildProjection(ctx context.Context, pool *pgxpool.Pool, opts RebuildOptions, validate sampleValidator) (RebuildResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return RebuildResult{}, err
	}
	defer rollback(ctx, tx)

	result, err := runRebuild(ctx, tx, opts, validate)
	if err != nil {
		return RebuildResult{}, err
	}
	if opts.DryRun {
		// Dry run writes nothing; the deferred rollback discards the read.
		return result, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return RebuildResult{}, err
	}
	return result, nil
}

func runRebuild(ctx context.Context, tx pgx.Tx, opts RebuildOptions, validate sampleValidator) (RebuildResult, error) {
	if opts.DryRun {
		rows, err := countAggregateRows(ctx, tx)
		if err != nil {
			return RebuildResult{}, err
		}
		return RebuildResult{RowsInserted: rows, DryRun: true}, nil
	}

	if _, err := tx.Exec(ctx, truncateEventSummarySQL); err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: truncate: %w", err)
	}
	tag, err := tx.Exec(ctx, insertEventSummaryFromOLTPSQL)
	if err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: insert from oltp: %w", err)
	}
	var offset string
	if err := tx.QueryRow(ctx, resetProjectionOffsetSQL, projectionProjectionName).Scan(&offset); err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: reset offset: %w", err)
	}
	result := RebuildResult{RowsInserted: int(tag.RowsAffected()), OffsetResetTo: offset}

	if opts.SampleValidate {
		if err := validate(ctx, tx); err != nil {
			return RebuildResult{}, err
		}
		result.Validated = true
	}
	if err := recordRebuildAudit(ctx, tx, result); err != nil {
		return RebuildResult{}, err
	}
	return result, nil
}

// countAggregateRows reports how many summary rows the aggregation would
// produce, used by dry-run mode to preview impact without writing.
func countAggregateRows(ctx context.Context, tx pgx.Tx) (int, error) {
	rows, err := tx.Query(ctx, aggregateEventSummarySQL)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	return count, rows.Err()
}

// sampleValidateConfirmed samples up to 5 rebuilt events and verifies each
// stored confirmed_count matches a fresh COUNT from registrations. A mismatch
// returns an error so the caller rolls back before committing.
func sampleValidateConfirmed(ctx context.Context, tx pgx.Tx) error {
	eventIDs, err := sampleEventIDs(ctx, tx)
	if err != nil {
		return err
	}
	for _, id := range eventIDs {
		var stored, actual int
		if err := tx.QueryRow(ctx, summaryConfirmedCountSQL, id).Scan(&stored); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, oltpConfirmedCountSQL, id).Scan(&actual); err != nil {
			return err
		}
		if stored != actual {
			return fmt.Errorf("rebuild: validation mismatch for event %s: summary=%d oltp=%d", id, stored, actual)
		}
	}
	return nil
}

// sampleEventIDs reads the sample set fully and closes the cursor before any
// further queries run on the same transaction connection.
func sampleEventIDs(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, sampleEventIDsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// recordRebuildAudit writes a sensitive-action audit row inside the rebuild
// transaction so it commits atomically with the projection changes.
func recordRebuildAudit(ctx context.Context, tx pgx.Tx, result RebuildResult) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	audit := newAuditRecord(
		auditID,
		Actor{ID: rebuildActorID, Role: RoleSystemAdmin},
		"reporting.projection.rebuild",
		"reporting_projection",
		projectionProjectionName,
		map[string]interface{}{
			"rows_inserted":   result.RowsInserted,
			"offset_reset_to": result.OffsetResetTo,
			"validated":       result.Validated,
		},
	)
	return insertAudit(ctx, tx, audit)
}
