package observability

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestObserveProjectionLagRecordsBucketsAndClampsNegative(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveProjectionLag(3 * time.Second)
	registry.ObserveProjectionLag(90 * time.Second)
	registry.ObserveProjectionLag(-5 * time.Second) // clock skew clamps to 0

	snapshot := registry.ProjectionSnapshot()
	assert.Equal(t, uint64(3), snapshot.LagCount)
	assert.InDelta(t, 93.0, snapshot.LagSum, 0.001)
	// Buckets are cumulative: le=1 catches only the clamped sample, le=5
	// catches the 3 s sample, le=120 onwards catch the 90 s sample too.
	assert.Equal(t, []uint64{1, 2, 2, 2, 2, 3, 3}, snapshot.LagBuckets)
}

func TestIncrementProjectionProcessedCountsAllOutcomes(t *testing.T) {
	registry := NewRegistry()

	registry.IncrementProjectionProcessed()
	registry.IncrementProjectionProcessed()

	assert.Equal(t, uint64(2), registry.ProjectionSnapshot().Processed)
}

func TestProjectionMetricsOnNilRegistryAreNoops(t *testing.T) {
	var registry *Registry

	registry.ObserveProjectionLag(time.Second)
	registry.IncrementProjectionProcessed()

	assert.Equal(t, ProjectionMetricsSnapshot{}, registry.ProjectionSnapshot())
}

func TestProjectionSnapshotCopiesBuckets(t *testing.T) {
	registry := NewRegistry()
	registry.ObserveProjectionLag(time.Second)

	snapshot := registry.ProjectionSnapshot()
	snapshot.LagBuckets[0] = 99

	assert.Equal(t, uint64(1), registry.ProjectionSnapshot().LagBuckets[0], "snapshot must not alias registry state")
}

func TestWriteProjectionMetricsRendersPrometheusSeries(t *testing.T) {
	registry := NewRegistry()
	registry.ObserveProjectionLag(2 * time.Second)
	registry.IncrementProjectionProcessed()

	var out strings.Builder
	registry.writeProjectionMetrics(&out)
	rendered := out.String()

	assert.Contains(t, rendered, "# TYPE cets_projection_worker_lag_seconds histogram")
	assert.Contains(t, rendered, "cets_projection_worker_events_processed_total 1")
	assert.Contains(t, rendered, `cets_projection_worker_lag_seconds_bucket{le="5"} 1`)
	assert.Contains(t, rendered, `cets_projection_worker_lag_seconds_bucket{le="+Inf"} 1`)
	assert.Contains(t, rendered, "cets_projection_worker_lag_seconds_sum 2")
	assert.Contains(t, rendered, "cets_projection_worker_lag_seconds_count 1")
}
