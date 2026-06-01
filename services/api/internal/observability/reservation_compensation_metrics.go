package observability

import (
	"context"
	"io"
	"strings"
)

// writeReservationCompensationMetrics derives the PH2-23 compensation counters
// from the reservation_compensation_metrics table the worker accumulates. The
// compensation sweep runs in the worker process (never scraped directly), so
// the durable totals are surfaced here on the serve /metrics path, matching
// the existing DB-derived worker-outcome pattern.
func writeReservationCompensationMetrics(ctx context.Context, w io.Writer, db SQLMetricsDB) {
	writeLine(w, "# HELP cets_reservation_compensation_total Reservation compensation sweep actions by action and result.")
	writeLine(w, "# TYPE cets_reservation_compensation_total counter")
	writeLine(w, "# HELP cets_reservation_counter_drift_total Reservation advisory counter drift-guard outcomes by result.")
	writeLine(w, "# TYPE cets_reservation_counter_drift_total counter")
	rows, err := db.Query(ctx, `/* reservation_compensation_metric_totals */
		SELECT metric, action, result, total
		FROM reservation_compensation_metrics
		ORDER BY metric, action, result`)
	if err != nil {
		writeLine(w, "cets_metrics_scrape_errors_total{collector=\"reservation_compensation\"} 1")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var metric, action, result string
		var total int64
		if err := rows.Scan(&metric, &action, &result, &total); err != nil {
			writeLine(w, "cets_metrics_scrape_errors_total{collector=\"reservation_compensation\"} 1")
			return
		}
		switch metric {
		case "compensation":
			writeFormat(w, "cets_reservation_compensation_total{action=\"%s\",result=\"%s\"} %d\n",
				escapeLabel(safeCompensationAction(action)), escapeLabel(safeCompensationResult(result)), total)
		case "counter_drift":
			writeFormat(w, "cets_reservation_counter_drift_total{result=\"%s\"} %d\n",
				escapeLabel(safeDriftResult(result)), total)
		}
	}
	if err := rows.Err(); err != nil {
		writeLine(w, "cets_metrics_scrape_errors_total{collector=\"reservation_compensation\"} 1")
	}
}

// safeCompensationAction bounds the action label to the fixed set the
// Compensator emits, collapsing anything unexpected to "unknown".
func safeCompensationAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "sweep_event", "lookup", "release", "drop":
		return action
	default:
		return "unknown"
	}
}

// safeCompensationResult bounds the result label to the Lua script outcomes
// plus the "error" sentinel the Go layer records on failure.
func safeCompensationResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "released", "released_missing_hold", "missing_pending", "dropped", "missing", "error":
		return result
	default:
		return "unknown"
	}
}

// safeDriftResult bounds the drift result label to the compensation_cap.lua
// outcomes.
func safeDriftResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "no_counter", "unchanged", "capped":
		return result
	default:
		return "unknown"
	}
}
