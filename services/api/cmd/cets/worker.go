package main

import (
	"context"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/objectstore"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/reservation"
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

	outboxKinds, compensationEnabled := splitWorkerKinds(cfg.WorkerKinds)
	compensator, redisClient, err := buildWorkerCompensator(cfg, service, logger, compensationEnabled)
	if err != nil {
		return err
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	if len(outboxKinds) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runWorkerKindLoops(loopCtx, workerKindLoopOptions{
				Logger:            logger,
				PollInterval:      cfg.WorkerPollInterval,
				RequestTimeout:    cfg.RequestTimeout,
				ShutdownGrace:     cfg.WorkerShutdownGrace,
				WorkerKinds:       outboxKinds,
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
		}()
	}

	if compensationEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runCompensationLoop(loopCtx, compensationLoopOptions{
				Logger:        logger.With("loop", "compensation"),
				Compensator:   compensator,
				Interval:      cfg.ReservationCompensationInterval,
				ShutdownGrace: cfg.WorkerShutdownGrace,
			})
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// splitWorkerKinds separates compensation (Redis-driven periodic sweep) from
// the outbox-driven kinds (notification, projection, export) so each runs
// in its own loop with the right cadence.
func splitWorkerKinds(kinds []string) ([]string, bool) {
	var outbox []string
	compensation := false
	for _, kind := range kinds {
		if kind == config.WorkerKindCompensation {
			compensation = true
			continue
		}
		outbox = append(outbox, kind)
	}
	return outbox, compensation
}

// buildWorkerCompensator constructs the Redis client + Compensator for the
// PH2-23 reservation gate. Returns (nil, nil, nil) when compensation is not
// requested or when the gate is off (no advisory holds exist that need
// reconciliation). On nil client + nil compensator, the caller skips the
// dedicated compensation loop.
func buildWorkerCompensator(cfg config.Config, lookup reservation.BookingLookup, logger *slog.Logger, enabled bool) (*reservation.Compensator, *redisClientCloser, error) {
	if !enabled {
		return nil, nil, nil
	}
	if !cfg.BookingPreadmission {
		logger.Info("compensation kind enabled but BOOKING_PREADMISSION=off; sweep will run but no orphan holds exist")
	}
	client, err := newWorkerRedisClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	cfgCompensation := reservation.CompensationConfig{
		GraceTTL:         cfg.ReservationGraceTTL,
		BatchSize:        cfg.WorkerBatchSize,
		MaxEvents:        64,
		DriftMarkerTTL:   60 * time.Second,
		OperationTimeout: cfg.ReservationOperationTimeout,
	}
	compensator := reservation.NewCompensator(client.client, cfgCompensation, lookup, logger, reservation.LogCompensationMetrics{Logger: logger})
	return compensator, client, nil
}
