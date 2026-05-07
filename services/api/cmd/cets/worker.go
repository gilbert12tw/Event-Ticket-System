package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/objectstore"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/ticketing"
)

func worker(cfg config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateWorker(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		migrateCtx, migrateCancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
		if err := postgres.Migrate(migrateCtx, pool); err != nil {
			migrateCancel()
			return err
		}
		migrateCancel()
		logger.Info("migration complete", "mode", "auto")
	}

	service := ticketing.NewService(pool, ticketing.NewSigner(cfg.TokenSigningSecret), logger)
	sender := ticketing.SMTPNotificationSender{Host: cfg.MailerHost, Port: cfg.MailerPort, From: cfg.MailerFrom}
	sender.RedirectTo = cfg.MailerRedirectTo
	reportStore := objectstore.S3CompatibleStore{
		Endpoint:  cfg.ObjectEndpoint,
		Bucket:    cfg.ObjectBucket,
		Region:    cfg.ObjectRegion,
		AccessKey: cfg.ObjectAccessKey,
		SecretKey: cfg.ObjectSecretKey,
	}
	ticker := time.NewTicker(cfg.WorkerPollInterval)
	defer ticker.Stop()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	logger.Info("worker started", "poll_interval_ms", cfg.WorkerPollInterval.Milliseconds(), "max_attempts", cfg.WorkerMaxAttempts, "batch_size", cfg.WorkerBatchSize)

	for {
		select {
		case <-ticker.C:
			workCtx, workCancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
			processed, err := service.ProcessOutboxOnceWithOptions(workCtx, ticketing.OutboxProcessorOptions{
				Sender:      sender,
				ReportStore: reportStore,
				MaxAttempts: cfg.WorkerMaxAttempts,
				BatchSize:   cfg.WorkerBatchSize,
			})
			workCancel()
			if err != nil {
				logger.Error("outbox processing failed", "error", err)
				continue
			}
			if processed > 0 {
				logger.Info("outbox processed", "count", processed)
			}
		case sig := <-stopCh:
			logger.Info("worker shutdown signal received", "signal", sig.String())
			return nil
		}
	}
}
