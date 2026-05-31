package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/postgres"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestOpsCommandsRunAgainstDatabase(t *testing.T) {
	cfg := newOpsIntegrationConfig(t)

	require.NoError(t, ops(cfg, testLogger(), []string{"outbox-stats"}))
	require.NoError(t, ops(cfg, testLogger(), []string{
		"replay",
		"--kind=notification",
		"--from=2026-05-28T10:00:00Z",
		"--to=2026-05-28T11:00:00Z",
	}))
}

func newOpsIntegrationConfig(t *testing.T) config.Config {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	adminPool, err := postgres.Connect(ctx, databaseURL)
	require.NoError(t, err)
	schema := fmt.Sprintf("ops_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchema))
	require.NoError(t, err)
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dropCancel()
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchema))
		adminPool.Close()
	})

	cfg := validCommandConfig()
	cfg.DatabaseURL = databaseURLWithSearchPath(t, databaseURL, schema)
	cfg.DatabaseTimeout = 10 * time.Second
	migrationPool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	require.NoError(t, err)
	defer migrationPool.Close()
	require.NoError(t, postgres.Migrate(ctx, migrationPool))
	return cfg
}

func databaseURLWithSearchPath(t *testing.T, databaseURL string, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	require.NoError(t, err)
	values := parsed.Query()
	values.Set("search_path", schema)
	parsed.RawQuery = values.Encode()
	return parsed.String()
}
