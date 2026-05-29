package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerKindLoopSpecsApplySelectedConcurrency(t *testing.T) {
	specs := workerKindLoopSpecs([]string{"projection", "notification"}, map[string]int{
		"notification": 2,
		"projection":   1,
		"export":       9,
	})

	assert.Equal(t, []workerKindLoopSpec{
		{Kind: "projection", Slot: 1},
		{Kind: "notification", Slot: 1},
		{Kind: "notification", Slot: 2},
	}, specs)
}

func TestRunWorkerKindLoopsStartsKindScopedWorkers(t *testing.T) {
	loopCtx, stop := context.WithCancel(context.Background())
	started := make(chan workerKindLoopSpec, 3)
	done := make(chan error, 1)

	go func() {
		done <- runWorkerKindLoops(loopCtx, workerKindLoopOptions{
			Logger:         testLogger(),
			PollInterval:   time.Millisecond,
			RequestTimeout: time.Second,
			ShutdownGrace:  time.Millisecond,
			WorkerKinds:    []string{"notification", "projection"},
			WorkerConcurrency: map[string]int{
				"notification": 2,
				"projection":   1,
			},
			Process: func(ctx context.Context, spec workerKindLoopSpec) (int, error) {
				started <- spec
				<-ctx.Done()
				return 0, ctx.Err()
			},
		})
	}()

	seen := map[workerKindLoopSpec]bool{}
	require.Eventually(t, func() bool {
		for {
			select {
			case spec := <-started:
				seen[spec] = true
			default:
				return len(seen) == 3
			}
		}
	}, time.Second, time.Millisecond)
	stop()

	require.NoError(t, <-done)
	assert.True(t, seen[workerKindLoopSpec{Kind: "notification", Slot: 1}])
	assert.True(t, seen[workerKindLoopSpec{Kind: "notification", Slot: 2}])
	assert.True(t, seen[workerKindLoopSpec{Kind: "projection", Slot: 1}])
}

func TestRunWorkerKindLoopsDoesNotBlockProjectionOnSlowNotification(t *testing.T) {
	loopCtx, stop := context.WithCancel(context.Background())
	notificationStarted := make(chan struct{})
	projectionProcessed := make(chan struct{})
	done := make(chan error, 1)
	var closeNotificationStarted sync.Once
	var closeProjectionProcessed sync.Once

	go func() {
		done <- runWorkerKindLoops(loopCtx, workerKindLoopOptions{
			Logger:         testLogger(),
			PollInterval:   time.Millisecond,
			RequestTimeout: time.Second,
			ShutdownGrace:  10 * time.Millisecond,
			WorkerKinds:    []string{"notification", "projection"},
			WorkerConcurrency: map[string]int{
				"notification": 1,
				"projection":   1,
			},
			Process: func(ctx context.Context, spec workerKindLoopSpec) (int, error) {
				if spec.Kind == "notification" {
					closeNotificationStarted.Do(func() { close(notificationStarted) })
					<-ctx.Done()
					return 0, ctx.Err()
				}
				if spec.Kind == "projection" {
					closeProjectionProcessed.Do(func() { close(projectionProcessed) })
					return 1, nil
				}
				return 0, nil
			},
		})
	}()

	require.Eventually(t, func() bool {
		select {
		case <-notificationStarted:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case <-projectionProcessed:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	stop()

	require.NoError(t, <-done)
}

func TestRunWorkerKindLoopsDrainsAllKindsOnShutdown(t *testing.T) {
	loopCtx, stop := context.WithCancel(context.Background())
	notificationStarted := make(chan struct{})
	projectionStarted := make(chan struct{})
	releaseNotification := make(chan struct{})
	done := make(chan error, 1)
	var closeNotificationStarted sync.Once
	var closeProjectionStarted sync.Once
	var notificationCalls int32
	var projectionCalls int32

	go func() {
		done <- runWorkerKindLoops(loopCtx, workerKindLoopOptions{
			Logger:         testLogger(),
			PollInterval:   time.Millisecond,
			RequestTimeout: time.Second,
			ShutdownGrace:  100 * time.Millisecond,
			WorkerKinds:    []string{"notification", "projection"},
			WorkerConcurrency: map[string]int{
				"notification": 1,
				"projection":   1,
			},
			Process: func(ctx context.Context, spec workerKindLoopSpec) (int, error) {
				switch spec.Kind {
				case "notification":
					atomic.AddInt32(&notificationCalls, 1)
					closeNotificationStarted.Do(func() { close(notificationStarted) })
					<-releaseNotification
					return 1, nil
				case "projection":
					atomic.AddInt32(&projectionCalls, 1)
					closeProjectionStarted.Do(func() { close(projectionStarted) })
					<-ctx.Done()
					return 0, ctx.Err()
				default:
					return 0, nil
				}
			},
		})
	}()

	require.Eventually(t, func() bool {
		select {
		case <-notificationStarted:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case <-projectionStarted:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	stop()
	assert.Never(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, 20*time.Millisecond, time.Millisecond, "multi-kind worker returned before notification batch drained")
	close(releaseNotification)

	require.NoError(t, <-done)
	assert.Equal(t, int32(1), atomic.LoadInt32(&notificationCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&projectionCalls))
}
