package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/reservation"
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
