package observability

import (
	"context"
	"io"
	"strings"
)

const rateLimitScrapeErrorMetric = `cets_metrics_scrape_errors_total{collector="rate_limit"} 1`

func writeRateLimitMetrics(ctx context.Context, w io.Writer, db SQLMetricsDB) {
	writeLine(w, "# HELP cets_rate_limit_drop_total Booking attempts dropped by rate limiting.")
	writeLine(w, "# TYPE cets_rate_limit_drop_total counter")
	rows, err := db.Query(ctx, `/* booking_rate_limit_audit_totals */
		SELECT COALESCE(NULLIF(metadata->>'scope', ''), 'unknown') AS scope, count(*)::bigint
		FROM audit_logs
		WHERE action = 'booking.rate_limited'
		GROUP BY scope
		ORDER BY scope`)
	if err != nil {
		writeLine(w, rateLimitScrapeErrorMetric)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var scope string
		var total int64
		if err := rows.Scan(&scope, &total); err != nil {
			writeLine(w, rateLimitScrapeErrorMetric)
			return
		}
		writeFormat(w, "cets_rate_limit_drop_total{scope=\"%s\"} %d\n", escapeLabel(safeRateLimitScope(scope)), total)
	}
	if err := rows.Err(); err != nil {
		writeLine(w, rateLimitScrapeErrorMetric)
	}
}

func safeRateLimitScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "actor", "event":
		return scope
	default:
		return "unknown"
	}
}
