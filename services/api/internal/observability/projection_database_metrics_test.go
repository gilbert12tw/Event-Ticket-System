package observability

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectionMetricsClampLagAndExposeProcessedCounter(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveProjectionLag(-time.Second)
	registry.ObserveProjectionLag(75 * time.Second)
	registry.IncrementProjectionProcessed()
	registry.IncrementProjectionProcessed()

	snapshot := registry.ProjectionSnapshot()
	require.Len(t, snapshot.LagBuckets, len(projectionBuckets))
	assert.Equal(t, uint64(2), snapshot.LagCount)
	assert.Equal(t, float64(75), snapshot.LagSum)
	assert.Equal(t, uint64(2), snapshot.Processed)
	assert.Equal(t, uint64(1), snapshot.LagBuckets[0], "negative lag should be clamped into the first bucket")
	assert.Equal(t, uint64(2), snapshot.LagBuckets[5], "projection lag buckets are cumulative")
	assert.Equal(t, uint64(2), snapshot.LagBuckets[6], "projection lag buckets are cumulative")

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_projection_worker_events_processed_total 2`)
	assert.Contains(t, metrics, `cets_projection_worker_lag_seconds_bucket{le="1"} 1`)
	assert.Contains(t, metrics, `cets_projection_worker_lag_seconds_bucket{le="120"} 2`)
	assert.Contains(t, metrics, `cets_projection_worker_lag_seconds_sum 75`)
	assert.Contains(t, metrics, `cets_projection_worker_lag_seconds_count 2`)
}

func TestProjectionMetricsNilRegistryIsSafe(t *testing.T) {
	var registry *Registry

	registry.ObserveProjectionLag(time.Second)
	registry.IncrementProjectionProcessed()

	assert.Empty(t, registry.ProjectionSnapshot())
}

func TestReservationCompensationMetricsExposeBoundedLabels(t *testing.T) {
	db := fakeSQLMetricsDB{
		lockWaitCount: 0,
		reservationCompensationRows: [][]any{
			{"compensation", "release", "released", int64(3)},
			{"compensation", "E1001", "token-leaked", int64(2)},
			{"counter_drift", "", "capped", int64(1)},
			{"counter_drift", "", "E2002", int64(4)},
		},
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `# TYPE cets_reservation_compensation_total counter`)
	assert.Contains(t, metrics, `cets_reservation_compensation_total{action="release",result="released"} 3`)
	assert.Contains(t, metrics, `cets_reservation_compensation_total{action="unknown",result="unknown"} 2`)
	assert.Contains(t, metrics, `cets_reservation_counter_drift_total{result="capped"} 1`)
	assert.Contains(t, metrics, `cets_reservation_counter_drift_total{result="unknown"} 4`)
	assert.NotContains(t, metrics, "E1001")
	assert.NotContains(t, metrics, "E2002")
	assert.NotContains(t, metrics, "token-leaked")
}

func TestDatabaseMetricsExposeLabeledPoolStatsAndBoundPoolNames(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(ctx))

	metricsDB := DatabaseMetrics{
		Write: pool,
		Read:  pool,
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(ctx, &body, metricsDB)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_db_pool_acquire_wait_seconds_total{pool="read"}`)
	assert.Contains(t, metrics, `cets_db_pool_acquire_wait_seconds_total{pool="write"}`)
	assert.Contains(t, metrics, `cets_db_pool_conns{pool="read",state="total"}`)
	assert.Contains(t, metrics, `cets_db_pool_conns{pool="write",state="total"}`)
	assert.Equal(t, "read", boundedPoolName(" read "))
	assert.Equal(t, "write", boundedPoolName("write"))
	assert.Equal(t, "unknown", boundedPoolName("replica-token"))
}
