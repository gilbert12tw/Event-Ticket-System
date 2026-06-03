package ticketing

import (
	"sync"
	"time"
)

// projectionLagBuckets are the histogram bucket boundaries (in seconds) for
// projection_worker_lag_seconds. Chosen to match the spec (1, 5, 15, 30, 60,
// 120, 180) while being compatible with the PH2-14/PH2-15 observability layer.
var projectionLagBuckets = []float64{1, 5, 15, 30, 60, 120, 180}

// projectionMetrics accumulates in-process histograms and counters for the
// projection worker.  These are intentionally kept in the ticketing package
// (not in the observability package) so they stay co-located with the worker
// that emits them, matching the compensation metrics pattern.
type projectionMetrics struct {
	mu         sync.Mutex
	lagBuckets []uint64 // parallel to projectionLagBuckets
	lagCount   uint64
	lagSum     float64
	processed  uint64 // projection_events_processed_total
}

var globalProjectionMetrics = &projectionMetrics{
	lagBuckets: make([]uint64, len(projectionLagBuckets)),
}

// observeLag records one lag observation.
func (m *projectionMetrics) observeLag(lagSeconds float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, bound := range projectionLagBuckets {
		if lagSeconds <= bound {
			m.lagBuckets[i]++
		}
	}
	m.lagCount++
	m.lagSum += lagSeconds
	m.processed++
}

// recordProjectionLag measures the lag between when the outbox event was
// created and now, then records it in the global histogram.
func recordProjectionLag(createdAt time.Time) {
	lagSeconds := time.Since(createdAt).Seconds()
	if lagSeconds < 0 {
		lagSeconds = 0
	}
	globalProjectionMetrics.observeLag(lagSeconds)
}

// ProjectionMetricsSnapshot captures a point-in-time view of the metrics for
// use in tests or scrape endpoints.
type ProjectionMetricsSnapshot struct {
	LagBuckets []uint64
	LagCount   uint64
	LagSum     float64
	Processed  uint64
}

// Snapshot returns a copy of the current metric state.
func (m *projectionMetrics) Snapshot() ProjectionMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ProjectionMetricsSnapshot{
		LagBuckets: append([]uint64(nil), m.lagBuckets...),
		LagCount:   m.lagCount,
		LagSum:     m.lagSum,
		Processed:  m.processed,
	}
}
