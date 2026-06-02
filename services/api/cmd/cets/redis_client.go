package main

import (
	"errors"
	"fmt"
	"strings"

	"event-ticket-system/internal/config"

	"github.com/redis/go-redis/v9"
)

// redisClientCloser is a thin wrapper that gives both `serve` and `worker`
// a typed way to close their Redis client deferred. Adding the wrapper type
// keeps the io.Closer contract obvious in the call sites without leaking
// `*redis.Client` into other files.
type redisClientCloser struct {
	client *redis.Client
}

func (c *redisClientCloser) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// newWorkerRedisClient connects the compensation worker to Redis. It uses
// the same REDIS_URL the serve process uses; the worker does not need any
// reservation-specific secrets because the compensator never recomputes the
// HMAC — it looks up by the hash already stored in PostgreSQL.
func newWorkerRedisClient(cfg config.Config) (*redisClientCloser, error) {
	if strings.TrimSpace(cfg.RedisURL) == "" {
		return nil, errors.New("REDIS_URL is required when compensation worker is enabled")
	}
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)
	return &redisClientCloser{client: client}, nil
}
