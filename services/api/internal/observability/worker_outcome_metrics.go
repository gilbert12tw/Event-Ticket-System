package observability

import (
	"context"
	"io"
	"strings"
)

const workerOutcomesScrapeErrorMetric = "cets_metrics_scrape_errors_total{collector=\"worker_outcomes\"} 1"

func writeWorkerOutcomeMetrics(ctx context.Context, w io.Writer, db SQLMetricsDB) {
	writeLine(w, "# HELP cets_worker_retry_total Worker retry attempts recorded in durable audit logs.")
	writeLine(w, "# TYPE cets_worker_retry_total counter")
	writeLine(w, "# HELP cets_worker_deadletter_total Worker dead-letter outcomes recorded in durable audit logs.")
	writeLine(w, "# TYPE cets_worker_deadletter_total counter")
	rows, err := db.Query(ctx, `/* worker_outbox_audit_totals */
		WITH worker_outcomes AS (
			SELECT action,
				COALESCE(NULLIF(metadata->>'worker_kind', ''), 'unknown') AS worker_kind,
				CASE
					WHEN action = 'outbox.retry_scheduled' THEN COALESCE(NULLIF(metadata->>'reason', ''), 'unknown')
					ELSE ''
				END AS reason
			FROM audit_logs
			WHERE action IN ('outbox.retry_scheduled', 'outbox.dead_letter')
		)
		SELECT action, worker_kind, reason, count(*)
		FROM worker_outcomes
		GROUP BY action, worker_kind, reason
		ORDER BY action, worker_kind, reason`)
	if err != nil {
		writeLine(w, workerOutcomesScrapeErrorMetric)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var action, workerKind, reason string
		var count int64
		if err := rows.Scan(&action, &workerKind, &reason, &count); err != nil {
			writeLine(w, workerOutcomesScrapeErrorMetric)
			return
		}
		writeWorkerOutcomeMetricRow(w, action, workerKind, reason, count)
	}
	if err := rows.Err(); err != nil {
		writeLine(w, workerOutcomesScrapeErrorMetric)
	}
}

func writeWorkerOutcomeMetricRow(w io.Writer, action, workerKind, reason string, count int64) {
	workerKind = safeWorkerMetricKind(workerKind)
	switch action {
	case "outbox.retry_scheduled":
		reason = safeWorkerMetricReason(reason)
		writeFormat(w, "cets_worker_retry_total{worker_kind=\"%s\",reason=\"%s\"} %d\n",
			escapeLabel(workerKind), escapeLabel(reason), count)
	case "outbox.dead_letter":
		writeFormat(w, "cets_worker_deadletter_total{worker_kind=\"%s\"} %d\n", escapeLabel(workerKind), count)
	}
}

func safeWorkerMetricKind(workerKind string) string {
	workerKind = strings.ToLower(strings.TrimSpace(workerKind))
	switch workerKind {
	case "compensation", "export", "notification", "projection":
		return workerKind
	default:
		return "unknown"
	}
}

func safeWorkerMetricReason(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	switch reason {
	case "invalid_payload", "retry_exhausted", "retryable_failure", "unknown_schema_version":
		return reason
	default:
		return "unknown"
	}
}
