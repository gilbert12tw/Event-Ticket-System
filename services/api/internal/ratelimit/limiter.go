// Package ratelimit implements the PH2-21 booking admission throttle.
// PostgreSQL remains booking truth; this package only decides whether a
// booking attempt should proceed into the business path.
package ratelimit

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

type OutageMode string

const (
	OutageModeDegrade OutageMode = "degrade"
	OutageModeFail    OutageMode = "fail"
	ScopeActor        Scope      = "actor"
	ScopeEvent        Scope      = "event"
)

var ErrUnavailable = errors.New("rate limiter unavailable")

type Scope string

type Decision struct {
	Allowed    bool
	Scope      Scope
	RetryAfter time.Duration
}

type Limiter interface {
	Enabled() bool
	Allow(ctx context.Context, eventID, actorHash string) (Decision, error)
}

type Store interface {
	Increment(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

type Config struct {
	Enabled             bool
	ActorLimitPerSecond int
	EventLimitPerSecond int
	OutageMode          OutageMode
	OperationTimeout    time.Duration
}

type RedisLimiter struct {
	store  Store
	cfg    Config
	logger *slog.Logger
	now    func() time.Time
}

type NoopLimiter struct{}

func (NoopLimiter) Enabled() bool { return false }

func (NoopLimiter) Allow(_ context.Context, _, _ string) (Decision, error) {
	return Decision{Allowed: true}, nil
}

func NewRedisLimiter(store Store, cfg Config, logger *slog.Logger) *RedisLimiter {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.OutageMode == "" {
		cfg.OutageMode = OutageModeDegrade
	}
	if cfg.OperationTimeout <= 0 {
		cfg.OperationTimeout = 150 * time.Millisecond
	}
	return &RedisLimiter{
		store:  store,
		cfg:    cfg,
		logger: logger,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (l *RedisLimiter) Enabled() bool {
	return l != nil && l.cfg.Enabled && (l.cfg.ActorLimitPerSecond > 0 || l.cfg.EventLimitPerSecond > 0)
}

func (l *RedisLimiter) Allow(ctx context.Context, eventID, actorHash string) (Decision, error) {
	if !l.Enabled() {
		return Decision{Allowed: true}, nil
	}
	window := l.now().Unix()
	if l.cfg.ActorLimitPerSecond > 0 {
		decision, err := l.check(ctx, actorKey(eventID, actorHash, window), ScopeActor, l.cfg.ActorLimitPerSecond)
		if err != nil || !decision.Allowed {
			return decision, err
		}
	}
	if l.cfg.EventLimitPerSecond > 0 {
		return l.check(ctx, eventKey(eventID, window), ScopeEvent, l.cfg.EventLimitPerSecond)
	}
	return Decision{Allowed: true}, nil
}

func (l *RedisLimiter) check(ctx context.Context, key string, scope Scope, limit int) (Decision, error) {
	opCtx, cancel := context.WithTimeout(ctx, l.cfg.OperationTimeout)
	defer cancel()
	count, err := l.store.Increment(opCtx, key, 2*time.Second)
	if err != nil {
		l.logger.Warn("rate limiter failed", "scope", scope, "error_class", classify(err), "outage_mode", l.cfg.OutageMode)
		if l.cfg.OutageMode == OutageModeFail {
			return Decision{}, ErrUnavailable
		}
		return Decision{Allowed: true}, nil
	}
	if count > int64(limit) {
		return Decision{Allowed: false, Scope: scope, RetryAfter: time.Second}, nil
	}
	return Decision{Allowed: true}, nil
}

func ParseOutageMode(value string) (OutageMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(OutageModeDegrade):
		return OutageModeDegrade, nil
	case string(OutageModeFail):
		return OutageModeFail, nil
	default:
		return "", errors.New("RATE_LIMIT_OUTAGE_MODE must be one of degrade|fail")
	}
}

func (c Config) Validate() error {
	if !c.Enabled || (c.ActorLimitPerSecond == 0 && c.EventLimitPerSecond == 0) {
		return nil
	}
	if c.ActorLimitPerSecond < 0 {
		return errors.New("BOOKING_RATE_LIMIT_RPS_PER_ACTOR must be non-negative")
	}
	if c.EventLimitPerSecond < 0 {
		return errors.New("BOOKING_RATE_LIMIT_RPS_PER_EVENT must be non-negative")
	}
	if c.OperationTimeout <= 0 {
		return errors.New("REDIS_OPERATION_TIMEOUT_MS must be positive when RATE_LIMIT_ENABLED=true")
	}
	switch c.OutageMode {
	case OutageModeDegrade, OutageModeFail:
	default:
		return errors.New("RATE_LIMIT_OUTAGE_MODE must be one of degrade|fail")
	}
	return nil
}

func actorKey(eventID, actorHash string, window int64) string {
	return keyPrefix + eventID + ":actor:" + actorHash + ":" + formatWindow(window)
}

func eventKey(eventID string, window int64) string {
	return keyPrefix + eventID + ":event:" + formatWindow(window)
}

func formatWindow(window int64) string {
	return time.Unix(window, 0).UTC().Format("20060102150405")
}

func classify(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "redis_error"
}
