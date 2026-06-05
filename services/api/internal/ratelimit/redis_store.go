package ratelimit

import (
	"context"
	"event-ticket-system/internal/observability"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "cets:v1:rate:booking:"

type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(client redis.UniversalClient) RedisStore {
	return RedisStore{client: client}
}

func (s RedisStore) Increment(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	ctx, span := observability.StartDependencySpan(ctx, observability.DependencySpanConfig{
		System:      "redis",
		ServiceName: "redis",
		Operation:   "rate_limit_increment",
	})
	pipe := s.client.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		observability.EndDependencySpan(span, err)
		return 0, err
	}
	observability.EndDependencySpan(span, nil)
	return count.Val(), nil
}
