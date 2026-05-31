package main

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/objectstore"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/ticketing"
)

func worker(cfg config.Config, logger *slog.Logger, args []string) error {
	var err error
	cfg, err = cfg.WithWorkerArgs(args)
	if err != nil {
		return err
	}
	if err := cfg.ValidateWorker(); err != nil {
		return err
	}
	runtimeObs, err := startRuntimeObservability(cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := runtimeObs.Shutdown(shutdownCtx); err != nil {
			logger.Error("runtime observability shutdown failed", "error", err)
		}
	}()

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

	service := newTicketingService(pool, cfg, logger)
	sender := ticketing.SMTPNotificationSender{Host: cfg.MailerHost, Port: cfg.MailerPort, From: cfg.MailerFrom}
	sender.RedirectTo = cfg.MailerRedirectTo
	reportStore := objectstore.S3CompatibleStore{
		Endpoint:  cfg.ObjectEndpoint,
		Bucket:    cfg.ObjectBucket,
		Region:    cfg.ObjectRegion,
		AccessKey: cfg.ObjectAccessKey,
		SecretKey: cfg.ObjectSecretKey,
	}
	loopCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	logger.Info("worker started",
		"poll_interval_ms", cfg.WorkerPollInterval.Milliseconds(),
		"shutdown_grace_seconds", int(cfg.WorkerShutdownGrace.Seconds()),
		"outbox_retry_max", cfg.OutboxRetryMax,
		"outbox_backoff_base_ms", cfg.OutboxBackoffBase.Milliseconds(),
		"outbox_backoff_max_ms", cfg.OutboxBackoffMax.Milliseconds(),
		"outbox_lease_ttl_seconds", int(cfg.OutboxLeaseTTL.Seconds()),
		"batch_size", cfg.WorkerBatchSize,
		"worker_kinds", cfg.WorkerKinds,
		"worker_concurrency", cfg.WorkerConcurrency,
	)

	return runWorkerKindLoops(loopCtx, workerKindLoopOptions{
		Logger:            logger,
		PollInterval:      cfg.WorkerPollInterval,
		RequestTimeout:    cfg.RequestTimeout,
		ShutdownGrace:     cfg.WorkerShutdownGrace,
		WorkerKinds:       cfg.WorkerKinds,
		WorkerConcurrency: cfg.WorkerConcurrency,
		Process: func(workCtx context.Context, spec workerKindLoopSpec) (int, error) {
			return service.ProcessOutboxOnceWithOptions(workCtx, ticketing.OutboxProcessorOptions{
				Sender:      sender,
				ReportStore: reportStore,
				BatchSize:   cfg.WorkerBatchSize,
				WorkerKinds: []string{spec.Kind},
				LeaseTTL:    cfg.OutboxLeaseTTL,
				RetryPolicy: &ticketing.OutboxRetryPolicy{
					MaxAttempts: cfg.OutboxRetryMax,
					BackoffBase: cfg.OutboxBackoffBase,
					BackoffMax:  cfg.OutboxBackoffMax,
				},
			})
		},
	})
}
