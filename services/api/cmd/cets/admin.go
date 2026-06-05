package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// admin dispatches one-off administrative subcommands. These are same-binary
// maintenance processes (12-Factor admin processes), reachable only by
// operators with env + DB access, never via the HTTP API.
func admin(cfg config.Config, logger *slog.Logger, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("admin command is required")
	}
	switch strings.TrimSpace(args[0]) {
	case "rebuild-projection":
		return adminRebuildProjection(cfg, logger, args[1:])
	default:
		return fmt.Errorf("unknown admin command")
	}
}

// adminRebuildProjection rebuilds reporting_event_summary from OLTP truth and
// resets the projection offset to the current outbox head. Behavior is driven
// entirely by env config (REBUILD_DRY_RUN / REBUILD_SAMPLE_VALIDATE).
func adminRebuildProjection(cfg config.Config, logger *slog.Logger, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown admin rebuild-projection argument")
	}
	opts := ticketing.RebuildOptions{
		DryRun:         cfg.RebuildDryRun,
		SampleValidate: cfg.RebuildSampleValidate,
	}
	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		result, err := ticketing.RebuildProjection(ctx, pool, opts)
		if err != nil {
			return err
		}
		logger.Info("projection rebuild complete",
			"dry_run", result.DryRun,
			"rows_inserted", result.RowsInserted,
			"offset_reset_to", result.OffsetResetTo,
			"validated", result.Validated,
		)
		return nil
	})
}
