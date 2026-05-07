package main

import (
	"context"
	"log/slog"
	"strings"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/ticketing"
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
	service := ticketing.NewService(pool, ticketing.NewSigner(cfg.TokenSigningSecret), logger)
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

	service := ticketing.NewService(pool, ticketing.NewSigner(cfg.TokenSigningSecret), logger)
	batch, err := service.RunHRSync(ctx, ticketing.Actor{ID: "hr-sync", Role: ticketing.RoleHRAdmin}, ticketing.HRSyncRequest{Source: source})
	if err != nil {
		return err
	}

	logger.Info("hr sync completed", "batch_id", batch.BatchID, "source", batch.Source, "employee_count", batch.EmployeeCount, "status", batch.Status)
	return nil
}
