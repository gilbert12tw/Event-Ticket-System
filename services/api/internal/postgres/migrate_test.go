package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaIncludesTicketingCorrectnessConstraints(t *testing.T) {
	schema := strings.Join(SchemaStatements, "\n")

	required := []string{
		"CREATE TABLE IF NOT EXISTS events",
		"capacity_type TEXT NOT NULL DEFAULT 'limited'",
		"capacity INTEGER",
		"allows_family BOOLEAN NOT NULL DEFAULT false",
		"events_capacity_rules_check",
		"CREATE TABLE IF NOT EXISTS event_versions",
		"event_versions_capacity_rules_check",
		"CREATE TABLE IF NOT EXISTS event_assets",
		"CREATE TABLE IF NOT EXISTS eligibility_rules",
		"CREATE TABLE IF NOT EXISTS eligibility_rule_versions",
		"CREATE TABLE IF NOT EXISTS hr_sync_batches",
		"CREATE TABLE IF NOT EXISTS eligibility_impact_reviews",
		"UNIQUE (event_id, employee_id)",
		"idempotency_key TEXT NOT NULL UNIQUE",
		"CREATE TABLE IF NOT EXISTS booking_idempotency_results",
		"remaining_capacity INTEGER NOT NULL DEFAULT 0",
		"idx_booking_idempotency_results_registration",
		"cancel_idempotency_key TEXT",
		"registration_id TEXT NOT NULL UNIQUE",
		"sequence_number INTEGER NOT NULL DEFAULT 1",
		"expires_at TIMESTAMPTZ",
		"signed_token_hash TEXT NOT NULL UNIQUE",
		"ticket_id TEXT NOT NULL UNIQUE",
		"CREATE TABLE IF NOT EXISTS checkin_rejections",
		"idx_checkin_rejections_ticket",
		"CREATE TABLE IF NOT EXISTS no_show_records",
		"idx_no_show_records_employee",
		"CREATE TABLE IF NOT EXISTS audit_logs",
		"CREATE TABLE IF NOT EXISTS outbox_events",
		"attempts INTEGER NOT NULL DEFAULT 0",
		"CREATE TABLE IF NOT EXISTS notification_deliveries",
		"CREATE TABLE IF NOT EXISTS lottery_runs",
		"CREATE TABLE IF NOT EXISTS lottery_results",
		"UNIQUE (run_id, registration_id)",
		"idx_lottery_results_run",
		"idx_notification_deliveries_outbox_channel",
		"CREATE TABLE IF NOT EXISTS offline_checkin_batches",
		"package_signature TEXT NOT NULL DEFAULT ''",
		"CREATE TABLE IF NOT EXISTS report_exports",
		"ALTER TABLE events ALTER COLUMN capacity DROP NOT NULL",
		"ALTER TABLE events ADD CONSTRAINT events_capacity_rules_check",
		"family_count INTEGER NOT NULL DEFAULT 0",
		"registrations_family_count_check",
	}

	for _, fragment := range required {
		assert.Contains(t, schema, fragment)
	}
}

func TestMigrateAppliesToEmptyDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, cleanup := newMigrationTestPool(t, ctx, databaseURL)
	defer cleanup()

	require.NoError(t, dropSchema(ctx, pool))

	require.NoError(t, Migrate(ctx, pool))

	requiredTables := []string{
		"employees",
		"events",
		"registrations",
		"tickets",
		"booking_idempotency_results",
		"no_show_records",
		"eligibility_impact_reviews",
		"outbox_events",
		"notification_deliveries",
		"lottery_runs",
		"lottery_results",
		"checkin_rejections",
		"offline_checkin_batches",
		"report_exports",
	}
	for _, table := range requiredTables {
		var exists bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists))
		assert.True(t, exists, "expected table %s to exist after migration", table)
	}
}

func TestMigrateSerializesConcurrentCalls(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, cleanup := newMigrationTestPool(t, ctx, databaseURL)
	defer cleanup()

	require.NoError(t, dropSchema(ctx, pool))

	const workers = 4
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Migrate(ctx, pool)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		assert.NoError(t, err)
	}
}

func TestMigrateAddsCapacityTypeColumnsToExistingEvents(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, cleanup := newMigrationTestPool(t, ctx, databaseURL)
	defer cleanup()

	_, err := pool.Exec(ctx, `CREATE TABLE events (
		event_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		location TEXT NOT NULL DEFAULT '',
		starts_at TIMESTAMPTZ NOT NULL,
		registration_start TIMESTAMPTZ NOT NULL,
		registration_close TIMESTAMPTZ NOT NULL,
		capacity INTEGER NOT NULL CHECK (capacity > 0),
		status TEXT NOT NULL CHECK (status IN ('draft', 'published', 'closed', 'cancelled')),
		allocation_mode TEXT NOT NULL DEFAULT 'fcfs',
		created_by TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO events
		(event_id, title, starts_at, registration_start, registration_close, capacity, status, created_by)
		VALUES ('evt_legacy', 'Legacy', now() + interval '7 days', now(), now() + interval '1 day', 25, 'published', 'admin-1')`)
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, pool))

	var capacityType string
	var capacity int
	var allowsFamily bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT capacity_type, capacity, allows_family FROM events WHERE event_id = 'evt_legacy'`).
		Scan(&capacityType, &capacity, &allowsFamily))
	assert.Equal(t, "limited", capacityType)
	assert.Equal(t, 25, capacity)
	assert.False(t, allowsFamily)
}

func TestEventCapacityConstraintsAcceptUnlimitedAndRejectInvalidRows(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, cleanup := newMigrationTestPool(t, ctx, databaseURL)
	defer cleanup()

	require.NoError(t, Migrate(ctx, pool))

	insertEvent := func(eventID string, capacityType string, capacity interface{}, allowsFamily bool) error {
		_, err := pool.Exec(ctx, `INSERT INTO events
			(event_id, title, starts_at, registration_start, registration_close, capacity_type, capacity, allows_family, status, created_by)
			VALUES ($1, 'Capacity Test', now() + interval '7 days', now(), now() + interval '1 day', $2, $3, $4, 'published', 'admin-1')`,
			eventID, capacityType, capacity, allowsFamily)
		return err
	}

	require.NoError(t, insertEvent("evt_unlimited", "unlimited", nil, true), "unlimited insert failed")
	for _, tt := range []struct {
		name         string
		eventID      string
		capacityType string
		capacity     interface{}
		allowsFamily bool
	}{
		{"limited null capacity", "evt_limited_null", "limited", nil, false},
		{"limited non-positive capacity", "evt_limited_zero", "limited", 0, false},
		{"limited with family", "evt_limited_family", "limited", 10, true},
		{"unlimited with capacity", "evt_unlimited_capacity", "unlimited", 10, false},
	} {
		assert.Error(t, insertEvent(tt.eventID, tt.capacityType, tt.capacity, tt.allowsFamily), "%s insert unexpectedly succeeded", tt.name)
	}
}

func dropSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `DROP TABLE IF EXISTS
		report_exports,
		offline_checkin_scans,
		offline_checkin_batches,
		lottery_results,
		lottery_runs,
		notification_deliveries,
		notification_preferences,
		outbox_events,
		audit_logs,
		checkin_rejections,
		no_show_records,
		checkin_records,
		eligibility_impact_reviews,
		tickets,
		registrations,
		hr_sync_batches,
		eligibility_rule_versions,
		eligibility_rules,
		event_assets,
		event_versions,
		events,
		employees
		CASCADE`)
	return err
}

func newMigrationTestPool(t *testing.T, ctx context.Context, databaseURL string) (*pgxpool.Pool, func()) {
	t.Helper()

	adminPool, err := Connect(ctx, databaseURL)
	require.NoError(t, err)
	adminClosed := false
	t.Cleanup(func() {
		if !adminClosed {
			adminPool.Close()
		}
	})

	schema := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchema))
	require.NoError(t, err)

	cfg, err := pgxpool.ParseConfig(databaseURL)
	require.NoError(t, err)
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	poolClosed := false
	t.Cleanup(func() {
		if !poolClosed {
			pool.Close()
		}
	})
	require.NoError(t, pool.Ping(ctx))

	return pool, func() {
		pool.Close()
		poolClosed = true
		dropCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchema))
		adminPool.Close()
		adminClosed = true
	}
}
