package observability

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

func writePoolMetrics(w io.Writer, db any) {
	if source, ok := db.(PoolMetricsSource); ok {
		writeLabeledPoolMetrics(w, source.PoolStats())
		return
	}
	stater, ok := db.(PoolStater)
	if !ok {
		return
	}
	stat := stater.Stat()
	writeLine(w, "# HELP cets_db_pool_acquire_wait_seconds_total Total time spent waiting for PostgreSQL pool acquires.")
	writeLine(w, "# TYPE cets_db_pool_acquire_wait_seconds_total counter")
	writeFormat(w, "cets_db_pool_acquire_wait_seconds_total %s\n", strconv.FormatFloat(stat.AcquireDuration().Seconds(), 'f', -1, 64))
	writeLine(w, "# HELP cets_db_pool_acquire_count_total Total PostgreSQL pool acquire calls.")
	writeLine(w, "# TYPE cets_db_pool_acquire_count_total counter")
	writeFormat(w, "cets_db_pool_acquire_count_total %d\n", stat.AcquireCount())
	writeLine(w, "# HELP cets_db_pool_conns Current PostgreSQL pool connections by state.")
	writeLine(w, "# TYPE cets_db_pool_conns gauge")
	writeFormat(w, "cets_db_pool_conns{state=\"acquired\"} %d\n", stat.AcquiredConns())
	writeFormat(w, "cets_db_pool_conns{state=\"idle\"} %d\n", stat.IdleConns())
	writeFormat(w, "cets_db_pool_conns{state=\"total\"} %d\n", stat.TotalConns())
}

func writeLabeledPoolMetrics(w io.Writer, pools map[string]PoolStater) {
	writeLine(w, "# HELP cets_db_pool_acquire_wait_seconds_total Total time spent waiting for PostgreSQL pool acquires.")
	writeLine(w, "# TYPE cets_db_pool_acquire_wait_seconds_total counter")
	writeLine(w, "# HELP cets_db_pool_acquire_count_total Total PostgreSQL pool acquire calls.")
	writeLine(w, "# TYPE cets_db_pool_acquire_count_total counter")
	writeLine(w, "# HELP cets_db_pool_conns Current PostgreSQL pool connections by state.")
	writeLine(w, "# TYPE cets_db_pool_conns gauge")
	names := make([]string, 0, len(pools))
	for name := range pools {
		names = append(names, boundedPoolName(name))
	}
	sort.Strings(names)
	seen := map[string]struct{}{}
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		stater := pools[name]
		if stater == nil {
			continue
		}
		stat := stater.Stat()
		label := fmt.Sprintf(`pool="%s"`, escapeLabel(name))
		writeFormat(w, "cets_db_pool_acquire_wait_seconds_total{%s} %s\n", label, strconv.FormatFloat(stat.AcquireDuration().Seconds(), 'f', -1, 64))
		writeFormat(w, "cets_db_pool_acquire_count_total{%s} %d\n", label, stat.AcquireCount())
		writeFormat(w, "cets_db_pool_conns{%s,state=\"acquired\"} %d\n", label, stat.AcquiredConns())
		writeFormat(w, "cets_db_pool_conns{%s,state=\"idle\"} %d\n", label, stat.IdleConns())
		writeFormat(w, "cets_db_pool_conns{%s,state=\"total\"} %d\n", label, stat.TotalConns())
	}
}

func writeSQLMetrics(ctx context.Context, w io.Writer, db any) {
	sqlDB, ok := db.(SQLMetricsDB)
	if !ok {
		return
	}
	writeLockWaitMetric(ctx, w, sqlDB)
	writeOutboxMetrics(ctx, w, sqlDB)
	writeWorkerOutcomeMetrics(ctx, w, sqlDB)
	writeReservationCompensationMetrics(ctx, w, sqlDB)
	writeRateLimitMetrics(ctx, w, sqlDB)
}

func boundedPoolName(name string) string {
	switch strings.TrimSpace(name) {
	case "read":
		return "read"
	case "write":
		return "write"
	default:
		return "unknown"
	}
}

func writeLockWaitMetric(ctx context.Context, w io.Writer, db SQLMetricsDB) {
	writeLine(w, "# HELP cets_db_lock_waiting_sessions PostgreSQL sessions currently waiting on locks.")
	writeLine(w, "# TYPE cets_db_lock_waiting_sessions gauge")
	var waiting int64
	err := db.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock'`).Scan(&waiting)
	if err != nil {
		writeLine(w, "cets_metrics_scrape_errors_total{collector=\"db_lock\"} 1")
		return
	}
	writeFormat(w, "cets_db_lock_waiting_sessions %d\n", waiting)
}

func writeOutboxMetrics(ctx context.Context, w io.Writer, db SQLMetricsDB) {
	writeLine(w, "# HELP cets_outbox_pending_total Outbox rows still requiring worker attention.")
	writeLine(w, "# TYPE cets_outbox_pending_total gauge")
	writeLine(w, "# HELP cets_outbox_lag_seconds Outbox publish latency distribution for rows that reached published state.")
	writeLine(w, "# TYPE cets_outbox_lag_seconds histogram")
	writeLine(w, "# HELP cets_outbox_oldest_lag_seconds Age of the oldest outbox row still requiring worker attention.")
	writeLine(w, "# TYPE cets_outbox_oldest_lag_seconds gauge")
	writeLine(w, "# HELP cets_outbox_retry_count Retry attempts recorded on outbox rows still requiring worker attention.")
	writeLine(w, "# TYPE cets_outbox_retry_count gauge")
	writeLine(w, "# HELP cets_outbox_dead_letter_total Outbox rows currently in dead-letter state.")
	writeLine(w, "# TYPE cets_outbox_dead_letter_total gauge")
	writeLine(w, "# HELP cets_outbox_lease_held_seconds Oldest held lease age for processing outbox rows.")
	writeLine(w, "# TYPE cets_outbox_lease_held_seconds gauge")
	rows, err := db.Query(ctx, `WITH outbox_attention AS (
			SELECT event_type,
				CASE
					WHEN event_type IN ('report.export.requested', 'report.export.requested.v2') THEN 'export'
					WHEN event_type = 'reporting.projection.update_required.v2' THEN 'projection'
					WHEN event_type LIKE 'reservation.compensation.%' THEN 'compensation'
					ELSE 'notification'
				END AS worker_kind,
				publish_status,
				retry_count,
				lease_started_at,
				created_at
			FROM outbox_events
			WHERE publish_status IN ('pending', 'processing', 'dead_letter')
		)
		SELECT event_type, worker_kind, publish_status, count(*),
			COALESCE(EXTRACT(EPOCH FROM now() - min(created_at)), 0),
			COALESCE(sum(retry_count), 0),
			count(*) FILTER (WHERE publish_status = 'dead_letter'),
			COALESCE(EXTRACT(EPOCH FROM now() - min(lease_started_at)), 0)
		FROM outbox_attention
		GROUP BY event_type, worker_kind, publish_status
		ORDER BY worker_kind, event_type, publish_status`)
	if err != nil {
		writeLine(w, outboxScrapeErrorMetric)
		return
	}
	defer rows.Close()

	aggregates := map[outboxMetricKey]outboxMetricAggregate{}
	for rows.Next() {
		var eventType, workerKind, status string
		var count, retryCount, deadLetterCount int64
		var oldestLag, leaseHeld float64
		if err := rows.Scan(&eventType, &workerKind, &status, &count, &oldestLag, &retryCount, &deadLetterCount, &leaseHeld); err != nil {
			writeLine(w, outboxScrapeErrorMetric)
			return
		}
		key := normalizeOutboxMetricKey(eventType, workerKind, status)
		addOutboxMetricAggregate(aggregates, key, count, oldestLag, retryCount, deadLetterCount, leaseHeld)
	}
	if err := rows.Err(); err != nil {
		writeLine(w, outboxScrapeErrorMetric)
		return
	}

	for _, aggregate := range sortedOutboxMetricAggregates(aggregates) {
		labels := fmt.Sprintf(`event_type="%s",worker_kind="%s",status="%s"`,
			escapeLabel(aggregate.Key.EventType), escapeLabel(aggregate.Key.WorkerKind), escapeLabel(aggregate.Key.Status))
		writeFormat(w, "cets_outbox_pending_total{%s} %d\n", labels, aggregate.Count)
		writeFormat(w, "cets_outbox_oldest_lag_seconds{%s} %s\n", labels, strconv.FormatFloat(aggregate.OldestLag, 'f', -1, 64))
		writeFormat(w, "cets_outbox_retry_count{%s} %d\n", labels, aggregate.RetryCount)
		writeFormat(w, "cets_outbox_lease_held_seconds{%s} %s\n", labels, strconv.FormatFloat(aggregate.LeaseHeld, 'f', -1, 64))
		if aggregate.DeadLetterCount > 0 {
			writeFormat(w, "cets_outbox_dead_letter_total{event_type=\"%s\",worker_kind=\"%s\"} %d\n",
				escapeLabel(aggregate.Key.EventType), escapeLabel(aggregate.Key.WorkerKind), aggregate.DeadLetterCount)
		}
	}
	writeOutboxLagHistogramMetrics(ctx, w, db)
}

func safeOutboxMetricEventType(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if _, ok := allowedOutboxMetricEventTypes[eventType]; ok {
		return eventType
	}
	return "unknown"
}
