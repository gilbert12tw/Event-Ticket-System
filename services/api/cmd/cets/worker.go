package main

import (
	"context"
	"log/slog"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/notification"
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
	databaseURL := strings.TrimSpace(cfg.DatabaseWriteURL)
	if databaseURL == "" {
		databaseURL = cfg.DatabaseURL
	}
	pool, err := postgres.Connect(ctx, databaseURL)
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
	sender := notification.SMTPNotificationSender{Host: cfg.MailerHost, Port: cfg.MailerPort, From: cfg.MailerFrom}
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
	compensator, redisClient, err := buildWorkerCompensator(cfg, service, service, logger, compensationEnabled)
	if err != nil {
		return err
	}
	if redisClient != nil {
		defer func() {
			if err := redisClient.Close(); err != nil {
				logger.Warn("compensation redis client close failed", "error_class", "redis_error")
			}
		}()
	}

	return runConfiguredWorkerLoops(loopCtx, configuredWorkerLoops{
		Config:      cfg,
		Logger:      logger,
		Service:     service,
		Sender:      sender,
		ReportStore: reportStore,
		OutboxKinds: outboxKinds,
		Compensator: compensator,
	})
}

type configuredWorkerLoops struct {
	Config      config.Config
	Logger      *slog.Logger
	Service     outboxProcessor
	Sender      notification.SMTPNotificationSender
	ReportStore objectstore.S3CompatibleStore
	OutboxKinds []string
	Compensator *reservation.Compensator
}

type outboxProcessor interface {
	ProcessOutboxOnceWithOptions(context.Context, ticketing.OutboxProcessorOptions) (int, error)
}

func runConfiguredWorkerLoops(loopCtx context.Context, loops configuredWorkerLoops) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	if len(loops.OutboxKinds) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runWorkerKindLoops(loopCtx, workerKindLoopOptions{
				Logger:            loops.Logger,
				PollInterval:      loops.Config.WorkerPollInterval,
				RequestTimeout:    loops.Config.RequestTimeout,
				ShutdownGrace:     loops.Config.WorkerShutdownGrace,
				WorkerKinds:       loops.OutboxKinds,
				WorkerConcurrency: loops.Config.WorkerConcurrency,
				Process:           loops.processOutboxKind,
			})
		}()
	}

	if loops.Compensator != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runCompensationLoop(loopCtx, compensationLoopOptions{
				Logger:        loops.Logger.With("loop", "compensation"),
				Compensator:   loops.Compensator,
				Interval:      loops.Config.ReservationCompensationInterval,
				ShutdownGrace: loops.Config.WorkerShutdownGrace,
			})
		}()
	}

	if len(loops.OutboxKinds) == 0 && loops.Compensator == nil {
		loops.Logger.Warn("worker has no active loops; compensation requested but BOOKING_PREADMISSION=off and no outbox kinds configured")
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

func (loops configuredWorkerLoops) processOutboxKind(workCtx context.Context, spec workerKindLoopSpec) (int, error) {
	return loops.Service.ProcessOutboxOnceWithOptions(workCtx, ticketing.OutboxProcessorOptions{
		Sender:      loops.Sender,
		ReportStore: loops.ReportStore,
		BatchSize:   loops.Config.WorkerBatchSize,
		WorkerKinds: []string{spec.Kind},
		LeaseTTL:    loops.Config.OutboxLeaseTTL,
		RetryPolicy: &ticketing.OutboxRetryPolicy{
			MaxAttempts: loops.Config.OutboxRetryMax,
			BackoffBase: loops.Config.OutboxBackoffBase,
			BackoffMax:  loops.Config.OutboxBackoffMax,
		},
	})
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
func buildWorkerCompensator(cfg config.Config, lookup reservation.BookingLookup, metrics reservation.CompensationMetrics, logger *slog.Logger, enabled bool) (*reservation.Compensator, *redisClientCloser, error) {
	if !enabled {
		return nil, nil, nil
	}
	if !cfg.BookingPreadmission {
		// Preadmission off means no Redis reservation holds are ever created,
		// so there is nothing to reconcile. Skip the Redis client and the
		// compensation loop entirely instead of hard-failing on an unset
		// REDIS_URL — a DB-only worker must start cleanly in this mode.
		logger.Info("compensation kind requested but BOOKING_PREADMISSION=off; skipping compensation loop (no reservation holds to reconcile)")
		return nil, nil, nil
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
	compensator := reservation.NewCompensator(client.client, cfgCompensation, lookup, logger, metrics)
	return compensator, client, nil
}
