package main

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func migrate(cfg config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateDatabase(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	logger.Info("migration complete")
	return nil
}

func ready(cfg config.Config) error {
	if err := cfg.ValidateDatabase(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return nil
}

func seed(cfg config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateForServe(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	service := newTicketingService(pool, cfg, logger)
	if err := service.SeedDemoData(ctx); err != nil {
		return err
	}
	logger.Info("seed complete")
	return nil
}

func hrSync(cfg config.Config, logger *slog.Logger, args []string) error {
	if err := cfg.ValidateDatabase(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	source := strings.TrimSpace(strings.Join(args, " "))
	if source == "" {
		source = "manual"
	}

	service := newTicketingService(pool, cfg, logger)
	batch, err := service.RunHRSync(ctx, ticketing.Actor{ID: "hr-sync", Role: ticketing.RoleHRAdmin}, ticketing.HRSyncRequest{Source: source})
	if err != nil {
		return err
	}

	logger.Info("hr sync completed", "batch_id", batch.BatchID, "source", batch.Source, "employee_count", batch.EmployeeCount, "status", batch.Status)
	return nil
}

func newTicketingService(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) *ticketing.Service {
	return ticketing.NewServiceWithPolicy(pool, ticketing.NewSigner(cfg.TokenSigningSecret), logger, ticketing.NoShowPolicy{
		Threshold:        cfg.NoShowThreshold,
		CooldownDuration: time.Duration(cfg.NoShowCooldownDays) * 24 * time.Hour,
		GracePeriod:      time.Duration(cfg.NoShowGraceHours) * time.Hour,
	})
}

func processNoShows(cfg config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateDatabase(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	service := newTicketingService(pool, cfg, logger)
	result, err := service.ProcessNoShows(ctx, ticketing.Actor{ID: "no-show-processor", Role: ticketing.RoleSystemAdmin})
	if err != nil {
		return err
	}
	logger.Info("no-show processing complete", "processed", result.Processed, "cooldowns_applied", result.CooldownsApplied)
	return nil
}
