package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"event-ticket-system/internal/reservation"
)

type compensationLoopOptions struct {
	Logger        *slog.Logger
	Compensator   *reservation.Compensator
	Interval      time.Duration
	ShutdownGrace time.Duration
}

// runCompensationLoop drives the PH2-23 reservation gate compensator on its
// own cadence (RESERVATION_COMPENSATION_INTERVAL_SECONDS). It is intentionally
// not part of the outbox workerKindLoop pipeline because compensation is
// not outbox-claimed — it sweeps Redis state and reconciles against
// PostgreSQL on a fixed timer. ctx cancellation stops the loop within
// ShutdownGrace; an in-flight sweep is allowed to finish before exit.
func runCompensationLoop(ctx context.Context, opts compensationLoopOptions) error {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Compensator == nil {
		logger.Info("reservation compensation loop skipped (gate disabled or not in WORKER_KINDS)")
		<-ctx.Done()
		return nil
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	logger.Info("reservation compensation loop started",
		"interval_seconds", int(interval.Seconds()))

	sweep := func() {
		sweepCtx, cancel := context.WithTimeout(ctx, interval)
		defer cancel()
		if err := opts.Compensator.Sweep(sweepCtx); err != nil {
			logger.Warn("reservation compensation sweep failed",
				"error_class", classifyCompensationError(err))
		}
	}

	// One immediate sweep so we don't wait a full interval for the first
	// reconciliation after a worker restart.
	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("reservation compensation loop stopping")
			return nil
		case <-ticker.C:
			sweep()
		}
	}
}

func classifyCompensationError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	return "redis_error"
}
