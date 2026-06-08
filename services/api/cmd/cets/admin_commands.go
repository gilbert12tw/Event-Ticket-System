package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func migrate(cfg config.Config, logger *slog.Logger) error {
	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := postgres.Migrate(ctx, pool); err != nil {
			return err
		}
		logger.Info("migration complete")
		return nil
	})
}

func ready(cfg config.Config) error {
	return withDatabase(cfg, cfg.ValidateDatabase, func(context.Context, *pgxpool.Pool) error { return nil })
}

func seed(cfg config.Config, logger *slog.Logger) error {
	return withDatabase(cfg, cfg.ValidateForServe, func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := postgres.Migrate(ctx, pool); err != nil {
			return err
		}
		service := newTicketingService(pool, cfg, logger)
		if err := service.SeedDemoData(ctx); err != nil {
			return err
		}
		logger.Info("seed complete")
		return nil
	})
}

func hrSync(cfg config.Config, logger *slog.Logger, args []string) error {
	source := strings.TrimSpace(strings.Join(args, " "))
	if source == "" {
		source = "manual"
	}

	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		service := newTicketingService(pool, cfg, logger)
		batch, err := service.RunHRSync(ctx, ticketing.Actor{ID: "hr-sync", Role: ticketing.RoleHRAdmin}, ticketing.HRSyncRequest{Source: source})
		if err != nil {
			return err
		}
		logger.Info("hr sync completed", "batch_id", batch.BatchID, "source", batch.Source, "employee_count", batch.EmployeeCount, "status", batch.Status)
		return nil
	})
}

func newTicketingService(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) *ticketing.Service {
	return ticketing.NewServiceWithPolicy(pool, ticketing.NewSigner(cfg.TokenSigningSecret), logger, ticketing.NoShowPolicy{
		Threshold:        cfg.NoShowThreshold,
		CooldownDuration: time.Duration(cfg.NoShowCooldownDays) * 24 * time.Hour,
		GracePeriod:      time.Duration(cfg.NoShowGraceHours) * time.Hour,
	}).WithBookingContentionStrategy(cfg.BookingContentionStrategy)
}

func processNoShows(cfg config.Config, logger *slog.Logger) error {
	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		service := newTicketingService(pool, cfg, logger)
		result, err := service.ProcessNoShows(ctx, ticketing.Actor{ID: "no-show-processor", Role: ticketing.RoleSystemAdmin})
		if err != nil {
			return err
		}
		logger.Info("no-show processing complete", "processed", result.Processed, "cooldowns_applied", result.CooldownsApplied)
		return nil
	})
}

// adminCmd dispatches one-off `cets admin <subcommand>` operations. These run
// as same-binary admin processes with env config and write no durable local
// files.
func adminCmd(cfg config.Config, logger *slog.Logger, args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "rebuild-projection":
		return rebuildProjection(cfg, logger)
	default:
		return fmt.Errorf("unknown admin subcommand %q (supported: rebuild-projection)", sub)
	}
}

// rebuildProjection rebuilds reporting_event_summary from PostgreSQL OLTP truth.
// Behaviour is controlled by REBUILD_DRY_RUN and REBUILD_SAMPLE_VALIDATE env config.
func rebuildProjection(cfg config.Config, logger *slog.Logger) error {
	settings, err := config.LoadRebuildSettings()
	if err != nil {
		return err
	}
	return withDatabase(cfg, cfg.ValidateDatabase, func(ctx context.Context, pool *pgxpool.Pool) error {
		service := newTicketingService(pool, cfg, logger)
		actor := ticketing.Actor{ID: "rebuild-projection", Role: ticketing.RoleSystemAdmin}
		result, err := service.RebuildProjection(ctx, actor, ticketing.RebuildOptions{
			DryRun:         settings.DryRun,
			SampleValidate: settings.SampleValidate,
		})
		if err != nil {
			return err
		}
		logger.Info("read-model rebuild finished",
			"rows_inserted", result.RowsInserted,
			"offset_reset_to", result.OffsetResetTo,
			"dry_run", result.DryRun,
			"validated", result.Validated,
		)
		return nil
	})
}

func withDatabase(cfg config.Config, validate func() error, run func(context.Context, *pgxpool.Pool) error) error {
	if err := validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()
	databaseURL := strings.TrimSpace(cfg.DatabaseWriteURL)
	if databaseURL == "" {
		databaseURL = cfg.DatabaseURL
	}
	pool, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return run(ctx, pool)
}
