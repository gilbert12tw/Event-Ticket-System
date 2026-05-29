package main

import (
	"context"
	"log/slog"
	"time"
)

type workerProcessFunc func(context.Context) (int, error)

type workerLoopOptions struct {
	Logger         *slog.Logger
	PollInterval   time.Duration
	RequestTimeout time.Duration
	ShutdownGrace  time.Duration
	Process        workerProcessFunc
}

type workerBatchResult struct {
	processed int
	err       error
}

const maxPostCancelShutdownWait = 2 * time.Second

func runWorkerLoop(ctx context.Context, options workerLoopOptions) error {
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	ticker := time.NewTicker(options.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			options.Logger.Info("worker shutdown requested", "phase", "idle")
			return nil
		case <-ticker.C:
			if ctx.Err() != nil {
				options.Logger.Info("worker shutdown requested", "phase", "idle")
				return nil
			}
			shutdown, err := runWorkerBatch(ctx, options)
			if shutdown {
				return nil
			}
			if err != nil {
				continue
			}
		}
	}
}

func runWorkerBatch(loopCtx context.Context, options workerLoopOptions) (bool, error) {
	if loopCtx.Err() != nil {
		options.Logger.Info("worker shutdown requested", "phase", "idle")
		return true, nil
	}
	workCtx, workCancel := context.WithTimeout(context.Background(), options.RequestTimeout)
	done := make(chan workerBatchResult, 1)
	go func() {
		processed, err := options.Process(workCtx)
		done <- workerBatchResult{processed: processed, err: err}
	}()

	select {
	case result := <-done:
		workCancel()
		logWorkerBatchResult(options.Logger, result)
		return false, result.err
	case <-loopCtx.Done():
		options.Logger.Info("worker shutdown requested", "phase", "draining", "grace_seconds", int(options.ShutdownGrace.Seconds()))
		return true, drainWorkerBatch(options, workCancel, done)
	}
}

func drainWorkerBatch(options workerLoopOptions, workCancel context.CancelFunc, done <-chan workerBatchResult) error {
	if options.ShutdownGrace <= 0 {
		workCancel()
		options.Logger.Warn("worker shutdown grace exceeded", "grace_seconds", 0)
		return nil
	}
	timer := time.NewTimer(options.ShutdownGrace)
	defer timer.Stop()
	select {
	case result := <-done:
		workCancel()
		logWorkerBatchResult(options.Logger, result)
		options.Logger.Info("worker shutdown drained in-flight batch")
		return nil
	case <-timer.C:
		workCancel()
		options.Logger.Warn("worker shutdown grace exceeded", "grace_seconds", int(options.ShutdownGrace.Seconds()))
		waitForCancelledWorkerBatch(options, done)
		return nil
	}
}

func waitForCancelledWorkerBatch(options workerLoopOptions, done <-chan workerBatchResult) {
	wait := postCancelShutdownWait(options.ShutdownGrace)
	if wait <= 0 {
		return
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case result := <-done:
		logWorkerBatchResult(options.Logger, result)
		options.Logger.Info("worker shutdown cancelled batch exited")
	case <-timer.C:
		options.Logger.Warn("worker shutdown cancellation wait exceeded", "wait_millis", wait.Milliseconds())
	}
}

func postCancelShutdownWait(grace time.Duration) time.Duration {
	if grace <= 0 {
		return 0
	}
	wait := grace / 10
	if wait < 100*time.Millisecond {
		wait = 100 * time.Millisecond
	}
	if wait > maxPostCancelShutdownWait {
		return maxPostCancelShutdownWait
	}
	return wait
}

func logWorkerBatchResult(logger *slog.Logger, result workerBatchResult) {
	if result.err != nil {
		logger.Error("outbox processing failed", "error", safeWorkerLogError(result.err))
		return
	}
	if result.processed > 0 {
		logger.Info("outbox processed", "count", result.processed)
	}
}
