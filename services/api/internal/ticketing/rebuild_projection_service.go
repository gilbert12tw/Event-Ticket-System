package ticketing

import (
	"context"
	"fmt"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
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
//  2. (dry-run stops here, writing nothing)
//  3. TRUNCATE + reinsert reporting_event_summary
//  4. reset reporting_projection_offsets watermark to MAX(outbox_id)
//  5. optional spot-check validation against OLTP (mismatch → rollback)
func (s *Service) RebuildProjection(ctx context.Context, actor Actor, opts RebuildOptions) (RebuildResult, error) {
	if err := requireRole(actor, RoleSystemAdmin); err != nil {
		return RebuildResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RebuildResult{}, err
	}
	defer rollback(ctx, tx)

	aggregated, err := aggregateFromOLTP(ctx, tx)
	if err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: aggregate from OLTP: %w", err)
	}

	if opts.DryRun {
		s.logRebuild(ctx, "rebuild dry-run (no write)", len(aggregated), "", false)
		return RebuildResult{RowsInserted: len(aggregated), DryRun: true}, nil
	}

	if err := truncateEventSummary(ctx, tx); err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: truncate: %w", err)
	}
	for _, row := range aggregated {
		if err := insertRebuiltEventSummary(ctx, tx, row); err != nil {
			return RebuildResult{}, fmt.Errorf("rebuild: insert %s: %w", row.EventID, err)
		}
	}

	offset, err := maxOutboxID(ctx, tx)
	if err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: read max outbox id: %w", err)
	}
	if err := resetProjectionOffset(ctx, tx, projectionProjectionName, offset); err != nil {
		return RebuildResult{}, fmt.Errorf("rebuild: reset offset: %w", err)
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
