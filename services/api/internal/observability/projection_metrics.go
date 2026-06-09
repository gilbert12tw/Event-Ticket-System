package observability

import (
	"io"
	"strconv"
	"time"
)

// projectionBuckets covers the projection freshness SLA: p95 < 60 s, max < 180 s.
// These must not be shared with httpBuckets whose max finite bucket is 10 s.
var projectionBuckets = []float64{1, 5, 15, 30, 60, 120, 180}

// ObserveProjectionLag records the lag between when an outbox event was
// created and when the projection worker processed it.
// Negative durations (clock skew) are clamped to zero.
func (r *Registry) ObserveProjectionLag(duration time.Duration) {
	if r == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	observeProjectionDuration(r.projection, duration.Seconds())
}

// IncrementProjectionProcessed records that one projection outbox event was
// fully handled (including decode-failed and unknown-inner-type skips).
// It must be called for every event that exits the projection pipeline,
// independently of whether lag is observed.
func (r *Registry) IncrementProjectionProcessed() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processedTotal++
}

// ProjectionMetricsSnapshot is a point-in-time copy of projection metrics.
type ProjectionMetricsSnapshot struct {
	LagBuckets []uint64
	LagCount   uint64
	LagSum     float64
	Processed  uint64
}

// ProjectionSnapshot returns a consistent snapshot of projection metrics.
func (r *Registry) ProjectionSnapshot() ProjectionMetricsSnapshot {
	if r == nil {
		return ProjectionMetricsSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return ProjectionMetricsSnapshot{
		LagBuckets: append([]uint64(nil), r.projection.Buckets...),
		LagCount:   r.projection.Count,
		LagSum:     r.projection.Sum,
		Processed:  r.processedTotal,
	}
}

// observeProjectionDuration records a lag sample using projectionBuckets
// (1 s … 180 s) instead of the HTTP-oriented httpBuckets (max 10 s).
func observeProjectionDuration(h *histogram, seconds float64) {
	for i, bucket := range projectionBuckets {
		if seconds <= bucket {
			h.Buckets[i]++
		}
	}
	h.Count++
	h.Sum += seconds
}

func (r *Registry) writeProjectionMetrics(w io.Writer) {
	writeLine(w, "# HELP cets_projection_worker_lag_seconds Projection worker lag in seconds.")
	writeLine(w, "# TYPE cets_projection_worker_lag_seconds histogram")
	writeLine(w, "# HELP cets_projection_worker_events_processed_total Projection worker processed events (all outcomes including skips).")
	writeLine(w, "# TYPE cets_projection_worker_events_processed_total counter")

	r.mu.Lock()
	h := histogram{
		Buckets: append([]uint64(nil), r.projection.Buckets...),
		Count:   r.projection.Count,
		Sum:     r.projection.Sum,
	}
	processed := r.processedTotal
	r.mu.Unlock()

	writeFormat(w, "cets_projection_worker_events_processed_total %d\n", processed)
	for i, bucket := range projectionBuckets {
		writeFormat(w, "cets_projection_worker_lag_seconds_bucket{le=%q} %d\n", formatBucket(bucket), h.Buckets[i])
	}
	writeFormat(w, "cets_projection_worker_lag_seconds_bucket{le=\"+Inf\"} %d\n", h.Count)
	writeFormat(w, "cets_projection_worker_lag_seconds_sum %s\n", strconv.FormatFloat(h.Sum, 'f', -1, 64))
	writeFormat(w, "cets_projection_worker_lag_seconds_count %d\n", h.Count)
}
