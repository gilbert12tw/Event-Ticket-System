package main

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"event-ticket-system/internal/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetDemoDBResetsPostgresAndRedis(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL is not set")
	}
	cfg := newOpsIntegrationConfig(t)
	cfg.RedisURL = redisURLWithDB(t, redisURL, "15")
	cfg.DatabaseTimeout = 15 * time.Second
	ctx := context.Background()
	client := redisClientForTest(t, cfg.RedisURL)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	otherKey := "outside:reset-demo-db:keep"
	require.NoError(t, client.Set(ctx, "cets:v1:resv:stale:remaining", "1", time.Hour).Err())
	require.NoError(t, client.Set(ctx, "cets:v1:rate:booking:stale:event:20260604120000", "1", time.Hour).Err())
	require.NoError(t, client.Set(ctx, otherKey, "keep", time.Hour).Err())
	t.Cleanup(func() { _ = client.Del(context.Background(), otherKey).Err() })

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
		VALUES ('E_STALE', 'Stale User', 'Ops', 'Taipei HQ', 1, 'active')`)
	require.NoError(t, err)

	require.NoError(t, resetDemoDB(cfg, testLogger()))

	assertTableCount(t, pool, "events", 2)
	assertTableCount(t, pool, "tickets", 2)
	assertTableCount(t, pool, "registrations", 2)
	assertTableCount(t, pool, "employees", 4)
	assertTableCount(t, pool, "reporting_projection_offsets", 1)
	assertRedisKeyAbsent(t, client, "cets:v1:resv:stale:remaining")
	assertRedisKeyAbsent(t, client, "cets:v1:rate:booking:stale:event:20260604120000")
	exists, err := client.Exists(ctx, otherKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)
}

func TestResetDemoDBFailsWhenRedisUnavailable(t *testing.T) {
	cfg := newOpsIntegrationConfig(t)
	cfg.RedisURL = "redis://127.0.0.1:1/0"
	cfg.DatabaseTimeout = 3 * time.Second

	err := resetDemoDB(cfg, testLogger())

	require.ErrorContains(t, err, "redis ping failed")
}

func redisURLWithDB(t *testing.T, redisURL string, db string) string {
	t.Helper()
	parsed, err := url.Parse(redisURL)
	require.NoError(t, err)
	parsed.Path = "/" + db
	return parsed.String()
}

func redisClientForTest(t *testing.T, redisURL string) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		t.Skipf("Redis DB in %s is not available: %v", redisURL, err)
	}
	return client
}

func assertTableCount(t *testing.T, pool *pgxpool.Pool, table string, want int) {
	t.Helper()
	var got int
	require.NoError(t, pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&got))
	assert.Equal(t, want, got, "row count for %s", table)
}

func assertRedisKeyAbsent(t *testing.T, client *redis.Client, key string) {
	t.Helper()
	exists, err := client.Exists(context.Background(), key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "expected %s to be deleted", key)
}
