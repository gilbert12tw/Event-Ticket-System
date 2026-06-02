package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/reservation"
	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubLookup satisfies reservation.BookingLookup; the build-time tests never
// invoke it because they only exercise the early-return wiring decisions.
type stubLookup struct{}

func (stubLookup) BookingByHash(context.Context, string, string) (reservation.BookingStatus, error) {
	return reservation.BookingStatus{}, nil
}
func (stubLookup) RemainingCapacity(context.Context, string) (int, error) { return 0, nil }
func (stubLookup) RecordAction(context.Context, string, string)           {}
func (stubLookup) RecordCounterDrift(context.Context, string)             {}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBuildWorkerCompensatorSkippedWhenKindDisabled(t *testing.T) {
	cfg := config.Config{BookingPreadmission: true, RedisURL: "redis://localhost:6379/0"}
	compensator, client, err := buildWorkerCompensator(cfg, stubLookup{}, stubLookup{}, quietLogger(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compensator != nil || client != nil {
		t.Fatalf("expected nil compensator and client when kind disabled, got %v / %v", compensator, client)
	}
}

// Regression for PR #60 review blocker 1: a DB-only worker with preadmission
// intentionally off (the default) must start cleanly even when REDIS_URL is
// unset, because no reservation holds can exist to reconcile.
func TestBuildWorkerCompensatorSkippedWhenPreadmissionOff(t *testing.T) {
	cfg := config.Config{BookingPreadmission: false, RedisURL: ""}
	compensator, client, err := buildWorkerCompensator(cfg, stubLookup{}, stubLookup{}, quietLogger(), true)
	if err != nil {
		t.Fatalf("expected no error with preadmission off and empty REDIS_URL, got %v", err)
	}
	if compensator != nil || client != nil {
		t.Fatalf("expected compensation skipped when preadmission off, got %v / %v", compensator, client)
	}
}

func TestBuildWorkerCompensatorRequiresRedisWhenPreadmissionOn(t *testing.T) {
	cfg := config.Config{BookingPreadmission: true, RedisURL: ""}
	compensator, client, err := buildWorkerCompensator(cfg, stubLookup{}, stubLookup{}, quietLogger(), true)
	if err == nil {
		t.Fatalf("expected error when preadmission on but REDIS_URL unset")
	}
	if compensator != nil || client != nil {
		t.Fatalf("expected nil compensator and client on error, got %v / %v", compensator, client)
	}
}

func TestSplitWorkerKinds(t *testing.T) {
	outbox, compensation := splitWorkerKinds([]string{
		config.WorkerKindNotification,
		config.WorkerKindCompensation,
		config.WorkerKindExport,
	})
	if !compensation {
		t.Fatalf("expected compensation flag true")
	}
	if len(outbox) != 2 || outbox[0] != config.WorkerKindNotification || outbox[1] != config.WorkerKindExport {
		t.Fatalf("unexpected outbox kinds: %v", outbox)
	}

	outbox, compensation = splitWorkerKinds([]string{config.WorkerKindNotification})
	if compensation {
		t.Fatalf("expected compensation flag false")
	}
	if len(outbox) != 1 {
		t.Fatalf("unexpected outbox kinds: %v", outbox)
	}
}

func TestRunCompensationLoopSkipsNilCompensatorUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runCompensationLoop(ctx, compensationLoopOptions{
		Logger:      quietLogger(),
		Compensator: nil,
	})

	require.NoError(t, err)
}

func TestClassifyCompensationError(t *testing.T) {
	assert.Equal(t, "", classifyCompensationError(nil))
	assert.Equal(t, "timeout", classifyCompensationError(context.Canceled))
	assert.Equal(t, "timeout", classifyCompensationError(context.DeadlineExceeded))
	assert.Equal(t, "redis_error", classifyCompensationError(errors.New("connection refused")))
}

func TestRedisClientCloserHandlesNilClients(t *testing.T) {
	require.NoError(t, (*redisClientCloser)(nil).Close())
	require.NoError(t, (&redisClientCloser{}).Close())
}

func TestNewWorkerRedisClientParsesValidURL(t *testing.T) {
	client, err := newWorkerRedisClient(config.Config{RedisURL: "redis://localhost:6379/0"})

	require.NoError(t, err)
	require.NotNil(t, client)
	require.NoError(t, client.Close())
}

func TestNewWorkerRedisClientRejectsInvalidURL(t *testing.T) {
	client, err := newWorkerRedisClient(config.Config{RedisURL: "://bad redis url"})

	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "invalid REDIS_URL")
}

func TestRunConfiguredWorkerLoopsReturnsWhenNoLoopsAreActive(t *testing.T) {
	err := runConfiguredWorkerLoops(context.Background(), configuredWorkerLoops{
		Config: config.Config{
			WorkerPollInterval: 1,
			WorkerConcurrency:  map[string]int{},
		},
		Logger: quietLogger(),
	})

	require.NoError(t, err)
}

func TestConfiguredWorkerLoopsProcessesOutboxKindThroughInterface(t *testing.T) {
	processor := &recordingOutboxProcessor{}
	loops := configuredWorkerLoops{
		Config: config.Config{
			WorkerBatchSize:   7,
			OutboxLeaseTTL:    3 * time.Second,
			OutboxRetryMax:    5,
			OutboxBackoffBase: 11 * time.Millisecond,
			OutboxBackoffMax:  29 * time.Millisecond,
		},
		Service: processor,
	}

	processed, err := loops.processOutboxKind(context.Background(), workerKindLoopSpec{Kind: "export"})

	require.NoError(t, err)
	assert.Equal(t, 2, processed)
	require.Len(t, processor.options, 1)
	options := processor.options[0]
	assert.Equal(t, 7, options.BatchSize)
	assert.Equal(t, []string{"export"}, options.WorkerKinds)
	assert.Equal(t, loops.Config.OutboxLeaseTTL, options.LeaseTTL)
	require.NotNil(t, options.RetryPolicy)
	assert.Equal(t, 5, options.RetryPolicy.MaxAttempts)
	assert.Equal(t, loops.Config.OutboxBackoffBase, options.RetryPolicy.BackoffBase)
	assert.Equal(t, loops.Config.OutboxBackoffMax, options.RetryPolicy.BackoffMax)
}

func TestRunConfiguredWorkerLoopsRunsOutboxLoopUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{})
	var closeCalled sync.Once
	processor := &recordingOutboxProcessor{onCall: func() {
		closeCalled.Do(func() { close(called) })
		cancel()
	}}
	done := make(chan error, 1)

	go func() {
		done <- runConfiguredWorkerLoops(ctx, configuredWorkerLoops{
			Config: config.Config{
				WorkerPollInterval:  time.Millisecond,
				RequestTimeout:      time.Second,
				WorkerShutdownGrace: time.Millisecond,
				WorkerConcurrency:   map[string]int{"notification": 1},
			},
			Logger:      quietLogger(),
			Service:     processor,
			OutboxKinds: []string{"notification"},
		})
	}()

	require.Eventually(t, func() bool {
		select {
		case <-called:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("worker loops did not stop after cancellation")
	}
}

type recordingOutboxProcessor struct {
	options []ticketing.OutboxProcessorOptions
	onCall  func()
}

func (p *recordingOutboxProcessor) ProcessOutboxOnceWithOptions(ctx context.Context, options ticketing.OutboxProcessorOptions) (int, error) {
	p.options = append(p.options, options)
	if p.onCall != nil {
		p.onCall()
	}
	return 2, ctx.Err()
}
