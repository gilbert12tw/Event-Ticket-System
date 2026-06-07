package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLotteryRunsKeepSingleCompletedEventDatabaseGuard(t *testing.T) {
	ctx, pool := newMigrationTest(t, 10*time.Second)

	require.NoError(t, Migrate(ctx, pool))

	var indexDefinition string
	require.NoError(t, pool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname = 'idx_lottery_runs_one_completed_event'`).
		Scan(&indexDefinition))
	assert.Contains(t, indexDefinition, "UNIQUE INDEX")
	assert.Contains(t, indexDefinition, "WHERE (status = 'completed'")

	_, err := pool.Exec(ctx, `INSERT INTO events
		(event_id, title, starts_at, ends_at, registration_start, registration_close, capacity_type, capacity, allows_family, status, allocation_mode, created_by)
		VALUES ('evt_lottery_guard', 'Lottery Guard', now() + interval '7 days', now() + interval '7 days' + interval '2 hours', now() - interval '1 day', now() - interval '1 hour', 'limited', 2, false, 'published', 'lottery', 'admin-1')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO lottery_runs (run_id, event_id, seed, status, created_by)
		VALUES ('lottery_guard_a', 'evt_lottery_guard', 'seed-a', 'completed', 'admin-1')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO lottery_runs (run_id, event_id, seed, status, created_by)
		VALUES ('lottery_guard_b', 'evt_lottery_guard', 'seed-b', 'completed', 'admin-1')`)
	require.Error(t, err, "database must reject a second completed lottery run for the same event")
}

func TestMigrateSupersedesLegacyDuplicateCompletedLotteryRuns(t *testing.T) {
	ctx, pool := newMigrationTest(t, 10*time.Second)

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
	_, err = pool.Exec(ctx, `CREATE TABLE lottery_runs (
		run_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		seed TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('completed')),
		created_by TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO events
		(event_id, title, starts_at, registration_start, registration_close, capacity, status, allocation_mode, created_by)
		VALUES ('evt_legacy_lottery', 'Legacy Lottery', now() + interval '7 days', now() - interval '2 days', now() - interval '1 day', 2, 'closed', 'lottery', 'admin-1')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO lottery_runs (run_id, event_id, seed, status, created_by, created_at)
		VALUES
			('lottery_legacy_old', 'evt_legacy_lottery', 'seed-old', 'completed', 'admin-1', '2026-05-01T10:00:00Z'),
			('lottery_legacy_new', 'evt_legacy_lottery', 'seed-new', 'completed', 'admin-1', '2026-05-02T10:00:00Z')`)
	require.NoError(t, err)

	require.NoError(t, Migrate(ctx, pool))

	var oldStatus, newStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM lottery_runs WHERE run_id = 'lottery_legacy_old'`).Scan(&oldStatus))
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM lottery_runs WHERE run_id = 'lottery_legacy_new'`).Scan(&newStatus))
	assert.Equal(t, "superseded", oldStatus)
	assert.Equal(t, "completed", newStatus)
	_, err = pool.Exec(ctx, `INSERT INTO lottery_runs (run_id, event_id, seed, status, created_by)
		VALUES ('lottery_legacy_third', 'evt_legacy_lottery', 'seed-third', 'completed', 'admin-1')`)
	require.Error(t, err, "database guard should apply after legacy duplicate remediation")
}
