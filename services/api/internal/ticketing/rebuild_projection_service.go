package ticketing

import (
	"context"
	"fmt"
	"time"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rebuildValidateSampleSize bounds the spot-check: up to this many events are
// re-verified against OLTP truth before the rebuild commits.
const rebuildValidateSampleSize = 5

// RebuildOptions configures a read-model rebuild run. Values originate from env
// config (REBUILD_DRY_RUN / REBUILD_SAMPLE_VALIDATE) — never hardcoded.
type RebuildOptions struct {
	DryRun         bool
	SampleValidate bool
}

// RebuildResult is the summary returned to the CLI for logging/printing.
type RebuildResult struct {
	RowsInserted  int
	OffsetResetTo string
	DryRun        bool
	Validated     bool
}

// RebuildProjection rebuilds reporting_event_summary from PostgreSQL OLTP truth
// (registrations + employees) inside a single transaction. It never replays
// outbox history and never writes durable local files. Only system_admin may
// run it (AC-7).
//
// Steps (all in one tx; any failure rolls back to the pre-rebuild state):
//  1. aggregate counts + confirmed-only department breakdown from OLTP
//  2. read the rebuild watermark (composite projectionOffsetKey of the newest
//     outbox event) in the same snapshot
//  3. (dry-run stops here, writing nothing)
//  4. clear (DELETE) + reinsert reporting_event_summary, stamping the watermark
//     into each row's last_event_offset
//  5. reset reporting_projection_offsets watermark to the same composite offset
//  6. optional spot-check validation against OLTP (mismatch → rollback)
//
// The transaction runs at REPEATABLE READ so the OLTP aggregate and the
// watermark read observe one consistent snapshot. Without it a booking that
// commits between the two reads would be aggregated by neither the rebuild
// (snapshot too early) nor the worker (its outbox event sits at/below the
// watermark and is suppressed), permanently dropping the registration.
//
// The rebuild also holds the exclusive projection-rebuild advisory lock for
// the whole transaction. Projection workers hold it shared per upsert, so the
// DELETE + plain re-INSERT below can never race a concurrent worker row and
// die on a duplicate key. The lock is a session-level lock taken BEFORE
// BeginTx on a dedicated connection: a REPEATABLE READ snapshot is created by
// the first statement in the transaction, so an in-transaction lock would be
// acquired only after the snapshot — too late to see worker rows committed
// while waiting for the lock.
func (s *Service) RebuildProjection(ctx context.Context, actor Actor, opts RebuildOptions) (RebuildResult, error) {
	if err := requireRole(actor, RoleSystemAdmin); err != nil {
		return RebuildResult{}, err
	}

	conn, tx, err := s.beginRebuildTx(ctx)
	if err != nil {
		return RebuildResult{}, err
	}
	defer conn.Release()
	defer releaseProjectionRebuildLock(conn)
	defer rollback(ctx, tx)

	aggregated, err := aggregateFromOLTP(ctx, tx)
	if err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: aggregate from OLTP: %w", err)
	}

	offset, err := rebuildWatermark(ctx, tx)
	if err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: read watermark: %w", err)
	}

	if opts.DryRun {
		s.logRebuild(ctx, "rebuild dry-run (no write)", len(aggregated), "", false)
		return RebuildResult{RowsInserted: len(aggregated), DryRun: true}, nil
	}

	if err := clearEventSummary(ctx, tx); err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: clear summary: %w", err)
	}
	for _, row := range aggregated {
		if err := insertRebuiltEventSummary(ctx, tx, row, offset); err != nil {
			return RebuildResult{}, fmt.Errorf("rebuild: insert %s: %w", row.EventID, err)
		}
	}

	if err := resetProjectionOffset(ctx, tx, projectionProjectionName, offset); err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: reset offset: %w", err)
	}

	if err := recordRebuildAudit(ctx, tx, actor, len(aggregated), offset, opts.SampleValidate); err != nil {
		return RebuildResult{}, err
	}

	if opts.SampleValidate {
		if err := s.validateRebuild(ctx, tx, aggregated); err != nil {
			return RebuildResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: commit: %w", err)
	}

	s.logRebuild(ctx, "rebuild complete", len(aggregated), offset, opts.SampleValidate)
	return RebuildResult{
		RowsInserted:  len(aggregated),
		OffsetResetTo: offset,
		Validated:     opts.SampleValidate,
	}, nil
}

// beginRebuildTx acquires a dedicated connection, takes the exclusive
// projection-rebuild advisory lock on it, and opens the REPEATABLE READ
// transaction. On success the caller owns all three teardown steps
// (rollback, releaseProjectionRebuildLock, conn.Release); on error every
// partially-acquired resource is already cleaned up here.
func (s *Service) beginRebuildTx(ctx context.Context) (*pgxpool.Conn, pgx.Tx, error) {
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("rebuild: acquire connection: %w", err)
	}
	if _, err := conn.Exec(ctx,
		`SELECT pg_advisory_lock(hashtext($1)::bigint)`, projectionRebuildLockName); err != nil {
		conn.Release()
		return nil, nil, fmt.Errorf("rebuild: acquire rebuild lock: %w", err)
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		releaseProjectionRebuildLock(conn)
		conn.Release()
		return nil, nil, err
	}
	return conn, tx, nil
}

// releaseProjectionRebuildLock releases the session-level rebuild lock before
// the connection returns to the pool. A fresh bounded context is used so the
// unlock still runs when the caller's context is already cancelled; if the
// unlock fails anyway, the underlying session is closed so the lock dies with
// it instead of leaking into the pool and blocking every future rebuild and
// projection worker.
func releaseProjectionRebuildLock(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := conn.Exec(ctx,
		`SELECT pg_advisory_unlock(hashtext($1)::bigint)`, projectionRebuildLockName); err != nil {
		_ = conn.Conn().Close(ctx)
	}
}

// recordRebuildAudit writes the rebuild as a sensitive admin action in the same
// tx so the audit entry commits atomically with the projection rewrite. Metadata
// holds only counts/offset/flags — never employee PII.
func recordRebuildAudit(ctx context.Context, tx pgx.Tx, actor Actor, rowsInserted int, offset string, sampleValidate bool) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor,
		"projection.rebuilt", "reporting_projection", projectionProjectionName,
		map[string]interface{}{
			"rows_inserted":   rowsInserted,
			"offset_reset_to": offset,
			"sample_validate": sampleValidate,
		})); err != nil {
		return fmt.Errorf("rebuild: audit: %w", err)
	}
	return nil
}

// validateRebuild spot-checks up to rebuildValidateSampleSize inserted rows by
// comparing the written projection confirmed_count against OLTP truth. Any
// mismatch returns an error, forcing the caller's deferred rollback.
func (s *Service) validateRebuild(ctx context.Context, tx pgx.Tx, rows []rebuildSummaryRow) error {
	reader := s.rebuildConfirmedCounter
	if reader == nil {
		reader = confirmedCountForEvent
	}
	for i, row := range rows {
		if i >= rebuildValidateSampleSize {
			break
		}
		projected, err := getEventSummaryRow(ctx, tx, row.EventID)
		if err != nil {
			return fmt.Errorf("rebuild: validate read %s: %w", row.EventID, err)
		}
		oltp, err := reader(ctx, tx, row.EventID)
		if err != nil {
			return fmt.Errorf("rebuild: validate oltp %s: %w", row.EventID, err)
		}
		if projected.ConfirmedCount != oltp {
			return fmt.Errorf("rebuild: validation mismatch for event %s: projection confirmed_count=%d oltp=%d",
				row.EventID, projected.ConfirmedCount, oltp)
		}
	}
	return nil
}

func (s *Service) logRebuild(ctx context.Context, msg string, rows int, offset string, validated bool) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info(msg,
		"trace_id", traceid.FromContext(ctx),
		"rows", rows,
		"offset_reset_to", offset,
		"validated", validated,
	)
}
