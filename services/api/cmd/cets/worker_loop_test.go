package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWorkerLoopStopsPollingAndDrainsInFlightOnShutdown(t *testing.T) {
	loopCtx, stop := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	var closeStarted sync.Once
	var calls int32

	go func() {
		done <- runWorkerLoop(loopCtx, workerLoopOptions{
			Logger:         testLogger(),
			PollInterval:   time.Millisecond,
			RequestTimeout: time.Second,
			ShutdownGrace:  100 * time.Millisecond,
			Process: func(context.Context) (int, error) {
				atomic.AddInt32(&calls, 1)
				closeStarted.Do(func() { close(started) })
				<-release
				return 1, nil
			},
		})
	}()

	require.Eventually(t, func() bool {
		select {
		case <-started:
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
	}, 20*time.Millisecond, time.Millisecond, "worker returned before the active batch drained")
	close(release)

	require.NoError(t, <-done)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestRunWorkerLoopCancelsInFlightAtShutdownGrace(t *testing.T) {
	loopCtx, stop := context.WithCancel(context.Background())
	started := make(chan struct{})
	cancelled := make(chan struct{})
	done := make(chan error, 1)
	var closeStarted sync.Once
	var closeCancelled sync.Once

	go func() {
		done <- runWorkerLoop(loopCtx, workerLoopOptions{
			Logger:         testLogger(),
			PollInterval:   time.Millisecond,
			RequestTimeout: time.Second,
			ShutdownGrace:  10 * time.Millisecond,
			Process: func(ctx context.Context) (int, error) {
				closeStarted.Do(func() { close(started) })
				<-ctx.Done()
				closeCancelled.Do(func() { close(cancelled) })
				return 0, ctx.Err()
			},
		})
	}()

	require.Eventually(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	stop()

	require.Eventually(t, func() bool {
		select {
		case <-cancelled:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	require.NoError(t, <-done)
}

func TestRunWorkerLoopWaitsForCancelledBatchToReleaseLease(t *testing.T) {
	loopCtx, stop := context.WithCancel(context.Background())
	started := make(chan struct{})
	cancelled := make(chan struct{})
	releaseLease := make(chan struct{})
	released := make(chan struct{})
	done := make(chan error, 1)
	var closeStarted sync.Once
	var closeCancelled sync.Once
	var closeReleased sync.Once

	go func() {
		done <- runWorkerLoop(loopCtx, workerLoopOptions{
			Logger:         testLogger(),
			PollInterval:   time.Millisecond,
			RequestTimeout: time.Second,
			ShutdownGrace:  5 * time.Millisecond,
			Process: func(ctx context.Context) (int, error) {
				closeStarted.Do(func() { close(started) })
				<-ctx.Done()
				closeCancelled.Do(func() { close(cancelled) })
				<-releaseLease
				closeReleased.Do(func() { close(released) })
				return 0, ctx.Err()
			},
		})
	}()

	require.Eventually(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	stop()
	require.Eventually(t, func() bool {
		select {
		case <-cancelled:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	assert.Never(t, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, 20*time.Millisecond, time.Millisecond, "worker returned before the cancelled batch released its lease")
	close(releaseLease)
	require.Eventually(t, func() bool {
		select {
		case <-released:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("worker did not return after cancelled batch released its lease")
	}
}

func TestRunWorkerBatchDoesNotStartAfterShutdownRequested(t *testing.T) {
	loopCtx, stop := context.WithCancel(context.Background())
	stop()
	var calls int32

	shutdown, err := runWorkerBatch(loopCtx, workerLoopOptions{
		Logger:         testLogger(),
		PollInterval:   time.Millisecond,
		RequestTimeout: time.Second,
		ShutdownGrace:  time.Millisecond,
		Process: func(context.Context) (int, error) {
			atomic.AddInt32(&calls, 1)
			return 1, nil
		},
	})

	require.NoError(t, err)
	assert.True(t, shutdown)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
}

func TestLogWorkerBatchResultRedactsUnsafeErrorDetails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	logWorkerBatchResult(logger, workerBatchResult{
		err: errors.New("smtp rejected e1001@cets.local for E1001 token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJF1001xyz.XyZ_12345678"),
	})

	output := logs.String()
	assert.Contains(t, output, `"msg":"outbox processing failed"`)
	assert.Contains(t, output, `"error":"smtp rejected [redacted email] for [redacted employee] token [redacted token]"`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "eyJhbGci")
}
