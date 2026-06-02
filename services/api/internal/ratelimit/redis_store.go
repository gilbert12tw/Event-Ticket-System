package ratelimit

import (
	"context"
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
	pipe := s.client.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return count.Val(), nil
}
