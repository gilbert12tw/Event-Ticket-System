package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"event-ticket-system/internal/observability"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/ratelimit"
	"event-ticket-system/internal/reservation"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBookingReservationGateBuildsRedisGate(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not set; skipping Redis-backed reservation gate test")
	}
	cfg := validCommandConfig()
	cfg.BookingPreadmission = true
	cfg.RedisURL = redisURL
	cfg.BookingReservationHashSecret = "reservation-hash-secret"
	cfg.ReservationOutageMode = "fail"
	cfg.ReservationTTL = 20 * time.Second
	cfg.ReservationGraceTTL = 5 * time.Second
	cfg.ReservationOperationTimeout = 150 * time.Millisecond

	gate, client, err := newBookingReservationGate(cfg, testLogger())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	require.NoError(t, client.Ping(context.Background()).Err())

	assert.IsType(t, &reservation.RedisGate{}, gate)
	assert.NotNil(t, client)
}

func TestNewBookingRateLimiterBuildsRedisLimiter(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not set; skipping Redis-backed rate limiter test")
	}
	cfg := validCommandConfig()
	cfg.RateLimitEnabled = true
	cfg.BookingRateLimitPerActor = 3
	cfg.BookingRateLimitPerEvent = 5
	cfg.RateLimitOutageMode = "fail"
	cfg.BookingRateLimitHashSecret = "rate-limit-hash-secret"
	cfg.ReservationOperationTimeout = 150 * time.Millisecond
	cfg.RedisURL = redisURL

	limiter, client, err := newBookingRateLimiter(cfg, testLogger())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})
	require.NoError(t, client.Ping(context.Background()).Err())

	assert.IsType(t, &ratelimit.RedisLimiter{}, limiter)
	assert.NotNil(t, client)
}

func TestConnectReadPoolUsesFallbackForBlankOrMatchingReadURL(t *testing.T) {
	cfg := validCommandConfig()
	cfg.DatabaseURL = "postgresql://write.example/cets"
	fallback := &pgxpool.Pool{}

	got, err := connectReadPool(context.Background(), cfg, testLogger(), fallback)
	require.NoError(t, err)
	assert.Same(t, fallback, got)

	cfg.DatabaseReadURL = " postgresql://write.example/cets "
	got, err = connectReadPool(context.Background(), cfg, testLogger(), fallback)
	require.NoError(t, err)
	assert.Same(t, fallback, got)
}

func TestConnectReadPoolConnectsDistinctReadURL(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	cfg := validCommandConfig()
	cfg.DatabaseURL = "postgresql://write.example/cets"
	cfg.DatabaseReadURL = databaseURL
	cfg.DatabaseTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	pool, err := connectReadPool(ctx, cfg, testLogger(), nil)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))
}

func TestAutoMigrateIfEnabledUsesConfiguredSchema(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	schemaURL := commandTestSchemaURL(t, databaseURL, "automigrate_test")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Connect(ctx, schemaURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	cfg := validCommandConfig()
	cfg.AutoMigrate = false
	require.NoError(t, autoMigrateIfEnabled(ctx, cfg, testLogger(), nil))

	cfg.AutoMigrate = true
	require.NoError(t, autoMigrateIfEnabled(ctx, cfg, testLogger(), pool))

	var exists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'events'
	)`).Scan(&exists))
	assert.True(t, exists)
}

func TestWorkerMetricsServerStartsAndShutsDown(t *testing.T) {
	cfg := validCommandConfig()
	cfg.WorkerMetricsPort = 0
	cfg.ShutdownTimeout = time.Second

	server := startWorkerMetricsServer(cfg, observability.NewRegistry(), nil, testLogger())

	require.NotNil(t, server)
	shutdownWorkerMetricsServer(server, cfg.ShutdownTimeout, testLogger())
}

func TestRunHTTPServerReturnsListenError(t *testing.T) {
	server := &http.Server{
		Addr:              "bad address",
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: time.Second,
	}
	cfg := validCommandConfig()
	cfg.AppAddr = server.Addr

	err := runHTTPServer(server, cfg, testLogger())

	require.Error(t, err)
}

func TestAdminCmdRejectsUnknownSubcommands(t *testing.T) {
	err := adminCmd(validCommandConfig(), testLogger(), nil)
	require.ErrorContains(t, err, "unknown admin subcommand")

	err = adminCmd(validCommandConfig(), testLogger(), []string{"wipe-production"})
	require.ErrorContains(t, err, "unknown admin subcommand")
	assert.NotContains(t, err.Error(), "production data")
}

func commandTestSchemaURL(t *testing.T, databaseURL string, prefix string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	adminPool, err := postgres.Connect(ctx, databaseURL)
	require.NoError(t, err)

	schema := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchema))
	require.NoError(t, err)
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dropCancel()
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchema))
		adminPool.Close()
	})
	return databaseURLWithSearchPath(t, databaseURL, schema)
}
