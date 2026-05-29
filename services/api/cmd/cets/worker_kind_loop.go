package main

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type workerKindProcessFunc func(context.Context, workerKindLoopSpec) (int, error)

type workerKindLoopOptions struct {
	Logger            *slog.Logger
	PollInterval      time.Duration
	RequestTimeout    time.Duration
	ShutdownGrace     time.Duration
	WorkerKinds       []string
	WorkerConcurrency map[string]int
	Process           workerKindProcessFunc
}

type workerKindLoopSpec struct {
	Kind string
	Slot int
}

func workerKindLoopSpecs(kinds []string, concurrency map[string]int) []workerKindLoopSpec {
	specs := []workerKindLoopSpec{}
	for _, kind := range kinds {
		count := concurrency[kind]
		if count <= 0 {
			count = 1
		}
		for slot := 1; slot <= count; slot++ {
			specs = append(specs, workerKindLoopSpec{Kind: kind, Slot: slot})
		}
	}
	return specs
}

func runWorkerKindLoops(ctx context.Context, options workerKindLoopOptions) error {
	specs := workerKindLoopSpecs(options.WorkerKinds, options.WorkerConcurrency)
	if len(specs) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(specs))
	for _, spec := range specs {
		spec := spec
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- runWorkerLoop(ctx, workerLoopOptions{
				Logger:         workerKindLogger(options.Logger, spec),
				PollInterval:   options.PollInterval,
				RequestTimeout: options.RequestTimeout,
				ShutdownGrace:  options.ShutdownGrace,
				Process: func(workCtx context.Context) (int, error) {
					return options.Process(workCtx, spec)
				},
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func workerKindLogger(logger *slog.Logger, spec workerKindLoopSpec) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With("worker_kind", spec.Kind, "worker_slot", spec.Slot)
}
