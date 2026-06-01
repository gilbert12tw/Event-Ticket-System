package reservation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// BookingStatus is the PostgreSQL-derived state of a booking, returned by
// the BookingLookup the Compensator depends on.
type BookingStatus struct {
	Found     bool // a booking_idempotency_results row exists for this (event_id, idempotency_hash)
	Completed bool // completed_at IS NOT NULL
	Confirmed bool // registration_status = 'confirmed'
}

// BookingLookup is the contract the Compensator needs from the ticketing
// layer. PostgreSQL is the only source of truth — the Compensator never
// decides ownership of a slot from Redis state alone.
type BookingLookup interface {
	BookingByHash(ctx context.Context, eventID, idempotencyHash string) (BookingStatus, error)
	RemainingCapacity(ctx context.Context, eventID string) (int, error)
}

// CompensationMetrics is the metric sink the Compensator calls. The worker
// wires a PostgreSQL-backed sink (ticketing.Service) so the serve /metrics
// endpoint can derive cets_reservation_compensation_total{action,result} and
// cets_reservation_counter_drift_total{result}; LogCompensationMetrics is the
// structured-log fallback used in tests and when no DB sink is supplied.
type CompensationMetrics interface {
	RecordAction(ctx context.Context, action, result string)
	RecordCounterDrift(ctx context.Context, result string)
}

// CompensationConfig controls how aggressively the Compensator scans Redis.
// All values come from environment variables in the worker bootstrap.
type CompensationConfig struct {
	GraceTTL         time.Duration // only consider pending members older than now-grace
	BatchSize        int           // max pending members processed per event per sweep
	MaxEvents        int           // max events scanned per sweep
	DriftMarkerTTL   time.Duration // TTL of the cets:v1:resv:{event_id}:drift observability marker
	OperationTimeout time.Duration // per Redis/PG call deadline
}

// Compensator scans `cets:v1:resv:{event_id}:pending` for expired members
// and reconciles them with PostgreSQL: confirmed bookings drop the hold
// without returning the slot, anything else releases the hold and returns
// the slot at most once. It also enforces that the advisory counter never
// exceeds DB-derived remaining capacity (drift guard).
//
// Safe to run repeatedly: every Lua script tolerates partial state, and
// every action is keyed by idempotency_hash so duplicate sweeps converge.
type Compensator struct {
	client  redis.UniversalClient
	cfg     CompensationConfig
	lookup  BookingLookup
	logger  *slog.Logger
	metrics CompensationMetrics
	release *redis.Script
	drop    *redis.Script
	cap     *redis.Script
}

// NewCompensator wires the Lua scripts and applies safe defaults to the
// config. Callers must supply a BookingLookup; logger and metrics fall back
// to default/no-op when nil.
func NewCompensator(client redis.UniversalClient, cfg CompensationConfig, lookup BookingLookup, logger *slog.Logger, metrics CompensationMetrics) *Compensator {
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = LogCompensationMetrics{Logger: logger}
	}
	if cfg.OperationTimeout <= 0 {
		cfg.OperationTimeout = 500 * time.Millisecond
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxEvents <= 0 {
		cfg.MaxEvents = 64
	}
	return &Compensator{
		client:  client,
		cfg:     cfg,
		lookup:  lookup,
		logger:  logger,
		metrics: metrics,
		release: redis.NewScript(compensationReleaseScript),
		drop:    redis.NewScript(compensationDropScript),
		cap:     redis.NewScript(compensationCapScript),
	}
}

// Sweep enumerates active events via SCAN and reconciles each. Errors on a
// single event do not abort the whole sweep — the worker should call Sweep
// on every interval regardless.
func (c *Compensator) Sweep(ctx context.Context) error {
	if c == nil || c.lookup == nil {
		return errors.New("compensator: lookup is nil")
	}
	events, err := c.activeEvents(ctx)
	if err != nil {
		return err
	}
	for _, eventID := range events {
		if err := c.SweepEvent(ctx, eventID); err != nil {
			c.logger.Warn("compensation sweep event failed",
				"event_id", eventID,
				"error_class", classify(err))
			c.metrics.RecordAction(ctx, "sweep_event", "error")
		}
	}
	return nil
}

// SweepEvent processes one event: drift cap, then up to BatchSize expired
// pending members. Splitting this out makes the worker controllable in
// tests and lets future work target a single event for manual reconciliation.
func (c *Compensator) SweepEvent(ctx context.Context, eventID string) error {
	if err := c.capCounter(ctx, eventID); err != nil {
		c.logger.Warn("compensation cap failed",
			"event_id", eventID,
			"error_class", classify(err))
	}
	threshold := time.Now().UTC().Add(-c.cfg.GraceTTL).Unix()
	members, err := c.expiredPendingMembers(ctx, eventID, threshold)
	if err != nil {
		return err
	}
	for _, hash := range members {
		c.reconcile(ctx, eventID, hash)
	}
	return nil
}

func (c *Compensator) reconcile(ctx context.Context, eventID, idempotencyHash string) {
	status, err := c.lookup.BookingByHash(ctx, eventID, idempotencyHash)
	if err != nil {
		c.logger.Warn("compensation lookup failed",
			"event_id", eventID,
			"error_class", classify(err))
		c.metrics.RecordAction(ctx, "lookup", "error")
		return
	}
	if !status.Found || !status.Completed {
		// Nothing or unfinished: the Redis hold is genuinely orphaned.
		c.releaseHold(ctx, eventID, idempotencyHash, "no_db_row")
		return
	}
	if status.Confirmed {
		// DB confirmed — the slot is owned by truth now. Drop the hold,
		// do NOT increment the counter.
		c.dropHold(ctx, eventID, idempotencyHash, "confirmed_elsewhere")
		return
	}
	// Completed but not confirmed (waitlisted/cancelled): the booking did
	// not consume capacity, return the advisory slot.
	c.releaseHold(ctx, eventID, idempotencyHash, "settled_non_confirmed")
}

func (c *Compensator) releaseHold(ctx context.Context, eventID, idempotencyHash, reason string) {
	capacity, err := c.lookup.RemainingCapacity(ctx, eventID)
	if err != nil {
		c.logger.Warn("compensation capacity probe failed",
			"event_id", eventID,
			"error_class", classify(err))
		c.metrics.RecordAction(ctx, "release", "error")
		return
	}
	opCtx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	raw, err := c.release.Run(opCtx, c.client,
		[]string{remainingKey(eventID), holdKey(eventID, idempotencyHash), pendingKey(eventID)},
		idempotencyHash, capacity,
	).Result()
	if err != nil {
		c.logger.Warn("compensation release lua failed",
			"event_id", eventID,
			"error_class", classify(err))
		c.metrics.RecordAction(ctx, "release", "error")
		return
	}
	outcome, _ := raw.(string)
	c.logger.Info("reservation compensation",
		"event_id", eventID,
		"action", "release",
		"reason", reason,
		"result", outcome)
	c.metrics.RecordAction(ctx, "release", outcome)
}

func (c *Compensator) dropHold(ctx context.Context, eventID, idempotencyHash, reason string) {
	opCtx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	raw, err := c.drop.Run(opCtx, c.client,
		[]string{holdKey(eventID, idempotencyHash), pendingKey(eventID)},
		idempotencyHash,
	).Result()
	if err != nil {
		c.logger.Warn("compensation drop lua failed",
			"event_id", eventID,
			"error_class", classify(err))
		c.metrics.RecordAction(ctx, "drop", "error")
		return
	}
	outcome, _ := raw.(string)
	c.logger.Info("reservation compensation",
		"event_id", eventID,
		"action", "drop",
		"reason", reason,
		"result", outcome)
	c.metrics.RecordAction(ctx, "drop", outcome)
}

func (c *Compensator) capCounter(ctx context.Context, eventID string) error {
	capacity, err := c.lookup.RemainingCapacity(ctx, eventID)
	if err != nil {
		return err
	}
	opCtx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	ttlSecs := int64(c.cfg.DriftMarkerTTL / time.Second)
	if ttlSecs <= 0 {
		ttlSecs = 60
	}
	raw, err := c.cap.Run(opCtx, c.client,
		[]string{remainingKey(eventID), driftKey(eventID)},
		capacity, ttlSecs,
	).Result()
	if err != nil {
		return err
	}
	outcome, _ := raw.(string)
	if outcome == "capped" {
		c.logger.Warn("reservation counter drift capped",
			"event_id", eventID,
			"capacity", capacity)
	}
	c.metrics.RecordCounterDrift(ctx, outcome)
	return nil
}

func (c *Compensator) activeEvents(ctx context.Context) ([]string, error) {
	opCtx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	pattern := keyPrefix + "*:pending"
	var eventIDs []string
	iter := c.client.Scan(opCtx, 0, pattern, int64(c.cfg.MaxEvents)).Iterator()
	for iter.Next(opCtx) {
		key := iter.Val()
		if id, ok := eventIDFromPendingKey(key); ok {
			eventIDs = append(eventIDs, id)
			if len(eventIDs) >= c.cfg.MaxEvents {
				break
			}
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan pending keys: %w", err)
	}
	return eventIDs, nil
}

func (c *Compensator) expiredPendingMembers(ctx context.Context, eventID string, threshold int64) ([]string, error) {
	opCtx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	return c.client.ZRangeByScore(opCtx, pendingKey(eventID), &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", threshold),
		Count: int64(c.cfg.BatchSize),
	}).Result()
}

func eventIDFromPendingKey(key string) (string, bool) {
	if !strings.HasPrefix(key, keyPrefix) || !strings.HasSuffix(key, ":pending") {
		return "", false
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(key, keyPrefix), ":pending")
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func driftKey(eventID string) string { return keyPrefix + eventID + ":drift" }

// LogCompensationMetrics is the structured-log fallback CompensationMetrics
// used in tests and when no DB sink is supplied. The worker wires the
// PostgreSQL-backed sink (ticketing.Service) so the serve /metrics endpoint
// can derive the Prometheus counters from durable rows.
type LogCompensationMetrics struct{ Logger *slog.Logger }

func (m LogCompensationMetrics) RecordAction(_ context.Context, action, result string) {
	if m.Logger == nil {
		return
	}
	m.Logger.Info("metric cets_reservation_compensation_total", "action", action, "result", result, "delta", 1)
}

func (m LogCompensationMetrics) RecordCounterDrift(_ context.Context, result string) {
	if m.Logger == nil {
		return
	}
	m.Logger.Info("metric cets_reservation_counter_drift_total", "result", result, "delta", 1)
}
