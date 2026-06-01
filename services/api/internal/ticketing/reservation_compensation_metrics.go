package ticketing

import (
	"context"
	"time"
)

// compensationMetricWriteTimeout bounds the best-effort metric UPSERT so a
// slow or unavailable PostgreSQL never stalls the compensation sweep. Metric
// emission is observability, not booking truth — losing a sample is safe.
const compensationMetricWriteTimeout = 500 * time.Millisecond

// RecordAction implements reservation.CompensationMetrics. It accumulates a
// monotonic total in reservation_compensation_metrics keyed by (action,
// result); the serve /metrics endpoint derives
// cets_reservation_compensation_total{action,result} from these rows. Best
// effort: failures are logged and swallowed so the sweep keeps running.
func (s *Service) RecordAction(ctx context.Context, action, result string) {
	s.incrementCompensationMetric(ctx, "compensation", action, result)
}

// RecordCounterDrift implements reservation.CompensationMetrics for the drift
// guard, feeding cets_reservation_counter_drift_total{result}. The action
// column is empty for this metric.
func (s *Service) RecordCounterDrift(ctx context.Context, result string) {
	s.incrementCompensationMetric(ctx, "counter_drift", "", result)
}

func (s *Service) incrementCompensationMetric(ctx context.Context, metric, action, result string) {
	if s == nil || s.db == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, compensationMetricWriteTimeout)
	defer cancel()
	_, err := s.db.Exec(writeCtx,
		`INSERT INTO reservation_compensation_metrics (metric, action, result, total)
		 VALUES ($1, $2, $3, 1)
		 ON CONFLICT (metric, action, result)
		 DO UPDATE SET total = reservation_compensation_metrics.total + 1, updated_at = now()`,
		metric, action, result,
	)
	if err != nil {
		s.logger.Warn("compensation metric write failed",
			"metric", metric,
			"action", action,
			"result", result,
			"error_class", "db_error")
	}
}
