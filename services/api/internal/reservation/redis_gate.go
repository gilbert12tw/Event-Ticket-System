package reservation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "cets:v1:resv:"

// RedisGate is the production Gate. It loads the Lua scripts once and uses
// EVALSHA for the hot path.
type RedisGate struct {
	client  redis.UniversalClient
	cfg     Config
	logger  *slog.Logger
	reserve *redis.Script
	release *redis.Script
	commit  *redis.Script
}

type reserveCapacityInput struct {
	eventID         string
	idempotencyHash string
	actorHash       string
	reservationID   string
	remaining       int
	version         int64
	ttlSecs         int64
}

func NewRedisGate(client redis.UniversalClient, cfg Config, logger *slog.Logger) *RedisGate {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.OutageMode == "" {
		cfg.OutageMode = OutageModeDegrade
	}
	if cfg.OperationTimeout <= 0 {
		cfg.OperationTimeout = 150 * time.Millisecond
	}
	return &RedisGate{
		client:  client,
		cfg:     cfg,
		logger:  logger,
		reserve: redis.NewScript(reserveScript),
		release: redis.NewScript(releaseScript),
		commit:  redis.NewScript(commitScript),
	}
}

func (g *RedisGate) Enabled() bool { return g.cfg.Enabled }

func (g *RedisGate) Reserve(ctx context.Context, eventID, idempotencyHash, actorHash string, probe CapacityProbe) (Hold, error) {
	if !g.cfg.Enabled {
		return Hold{Outcome: OutcomeGranted}, nil
	}
	reservationID, err := newReservationID()
	if err != nil {
		return Hold{}, err
	}
	ttlSecs := int64(g.cfg.TTL / time.Second)
	if ttlSecs <= 0 {
		ttlSecs = 20
	}
	hold, err := g.reserveWithCapacity(ctx, reserveCapacityInput{
		eventID:         eventID,
		idempotencyHash: idempotencyHash,
		actorHash:       actorHash,
		reservationID:   reservationID,
		remaining:       -1,
		version:         -1,
		ttlSecs:         ttlSecs,
	})
	if err != nil {
		return Hold{}, err
	}
	if hold.Outcome != outcomeNeedsProbe {
		return hold, nil
	}
	remaining, version, err := probe(ctx)
	if err != nil {
		return Hold{}, fmt.Errorf("reservation probe: %w", err)
	}
	if remaining < 0 {
		remaining = 0
	}
	return g.reserveWithCapacity(ctx, reserveCapacityInput{
		eventID:         eventID,
		idempotencyHash: idempotencyHash,
		actorHash:       actorHash,
		reservationID:   reservationID,
		remaining:       remaining,
		version:         version,
		ttlSecs:         ttlSecs,
	})
}

func (g *RedisGate) reserveWithCapacity(ctx context.Context, input reserveCapacityInput) (Hold, error) {
	opCtx, cancel := context.WithTimeout(ctx, g.cfg.OperationTimeout)
	defer cancel()
	now := time.Now().UTC().Unix()
	raw, err := g.reserve.Run(opCtx, g.client,
		[]string{
			remainingKey(input.eventID),
			holdKey(input.eventID, input.idempotencyHash),
			pendingKey(input.eventID),
			versionKey(input.eventID),
		},
		input.idempotencyHash, input.remaining, input.version, input.ttlSecs, now, input.actorHash, input.eventID, input.reservationID,
	).Result()
	if err != nil {
		return g.handleOutage(err, "reserve")
	}
	outcome, id, exp, holdVersion, parseErr := parseReserveResult(raw)
	if parseErr != nil {
		return Hold{}, parseErr
	}
	switch outcome {
	case OutcomeGranted, OutcomeDuplicate, OutcomeExhausted, outcomeNeedsProbe:
	case OutcomeMisconfigured:
		g.logger.Warn("reservation lua misconfigured", "op", "reserve", "error_class", "misconfigured")
		return Hold{}, ErrUnavailable
	default:
		return Hold{}, fmt.Errorf("unexpected reservation outcome %q", outcome)
	}
	return Hold{
		Outcome:         outcome,
		ReservationID:   id,
		IdempotencyHash: input.idempotencyHash,
		CapacityVersion: holdVersion,
		ExpiresAt:       time.Unix(exp, 0).UTC(),
	}, nil
}

func (g *RedisGate) Confirm(ctx context.Context, eventID, idempotencyHash string) error {
	if !g.cfg.Enabled {
		return nil
	}
	opCtx, cancel := context.WithTimeout(ctx, g.cfg.OperationTimeout)
	defer cancel()
	_, err := g.commit.Run(opCtx, g.client,
		[]string{holdKey(eventID, idempotencyHash), pendingKey(eventID)},
		idempotencyHash,
	).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		g.logger.Warn("reservation commit failed", "event_id", eventID, "error_class", classify(err))
		if g.cfg.OutageMode == OutageModeFail {
			return ErrUnavailable
		}
	}
	return nil
}

func (g *RedisGate) Release(ctx context.Context, eventID, idempotencyHash string) error {
	if !g.cfg.Enabled {
		return nil
	}
	opCtx, cancel := context.WithTimeout(ctx, g.cfg.OperationTimeout)
	defer cancel()
	_, err := g.release.Run(opCtx, g.client,
		[]string{remainingKey(eventID), holdKey(eventID, idempotencyHash), pendingKey(eventID)},
		idempotencyHash,
	).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		g.logger.Warn("reservation release failed", "event_id", eventID, "error_class", classify(err))
		if g.cfg.OutageMode == OutageModeFail {
			return ErrUnavailable
		}
	}
	return nil
}

func (g *RedisGate) handleOutage(err error, op string) (Hold, error) {
	g.logger.Warn("reservation lua failed", "op", op, "error_class", classify(err), "outage_mode", g.cfg.OutageMode)
	if g.cfg.OutageMode == OutageModeFail {
		return Hold{}, ErrUnavailable
	}
	return Hold{Outcome: OutcomeGranted}, nil
}

func parseReserveResult(raw interface{}) (Outcome, string, int64, int64, error) {
	arr, ok := raw.([]interface{})
	if !ok || len(arr) != 3 {
		if !ok || len(arr) != 4 {
			return "", "", 0, 0, fmt.Errorf("unexpected reserve result type: %T", raw)
		}
	}
	outcome, _ := arr[0].(string)
	id, _ := arr[1].(string)
	expStr, _ := arr[2].(string)
	exp, _ := strconv.ParseInt(expStr, 10, 64)
	version := int64(0)
	if len(arr) > 3 {
		versionStr, _ := arr[3].(string)
		version, _ = strconv.ParseInt(versionStr, 10, 64)
	}
	return Outcome(outcome), id, exp, version, nil
}

func remainingKey(eventID string) string { return keyPrefix + eventID + ":remaining" }

func holdKey(eventID, idempotencyHash string) string {
	return keyPrefix + eventID + ":hold:" + idempotencyHash
}

func pendingKey(eventID string) string { return keyPrefix + eventID + ":pending" }

func versionKey(eventID string) string { return keyPrefix + eventID + ":version" }

func newReservationID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "resv_" + hex.EncodeToString(b[:]), nil
}

func classify(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, redis.Nil) {
		return "nil"
	}
	return "redis_error"
}
