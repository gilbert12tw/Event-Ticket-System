package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistrationActiveUniquenessAllowsHistoricalCancelledRows(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, cleanup := newMigrationTestPool(t, ctx, databaseURL)
	defer cleanup()

	require.NoError(t, Migrate(ctx, pool))
	require.NoError(t, seedRegistrationConstraintFixture(ctx, pool))

	var oldConstraintExists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'registrations_event_id_employee_id_key'
			  AND connamespace = current_schema()::regnamespace
		)`).Scan(&oldConstraintExists))
	assert.False(t, oldConstraintExists, "full event/employee uniqueness must be removed")

	_, err := pool.Exec(ctx, `INSERT INTO registrations
			(registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ('reg_cancelled', 'evt_reg_unique', 'E_REG_UNIQUE', 'cancelled', 'idem_cancelled')`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `INSERT INTO registrations
			(registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ('reg_confirmed', 'evt_reg_unique', 'E_REG_UNIQUE', 'confirmed', 'idem_confirmed')`)
	require.NoError(t, err, "cancelled history must not block a new active registration")

	_, err = pool.Exec(ctx, `INSERT INTO registrations
			(registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ('reg_waitlisted', 'evt_reg_unique', 'E_REG_UNIQUE', 'waitlisted', 'idem_waitlisted')`)
	assert.Error(t, err, "a second active registration for the same event/employee must be rejected")
}

func seedRegistrationConstraintFixture(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `INSERT INTO employees
			(employee_id, full_name, department, site, job_grade, employment_status)
		VALUES ('E_REG_UNIQUE', 'Registration Unique', 'Engineering', 'Taipei HQ', 5, 'active')`); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `INSERT INTO events
			(event_id, title, starts_at, ends_at, registration_start, registration_close, capacity_type, capacity, status, created_by)
		VALUES ('evt_reg_unique', 'Registration Unique', now() + interval '7 days', now() + interval '7 days' + interval '2 hours', now(), now() + interval '1 day', 'limited', 10, 'published', 'admin-1')`)
	return err
}
