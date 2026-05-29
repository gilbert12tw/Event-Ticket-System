package observability

import (
	"context"
	"fmt"
	"io"
	"strconv"
)

var outboxLagHistogramBuckets = []float64{1, 5, 10, 30, 60, 180, 300, 600, 1800, 3600}

func writeOutboxLagHistogramMetrics(ctx context.Context, w io.Writer, db SQLMetricsDB) {
	rows, err := db.Query(ctx, `/* outbox_lag_histogram */
		WITH outbox_published AS (
			SELECT event_type,
				CASE
					WHEN event_type IN ('report.export.requested', 'report.export.requested.v2') THEN 'export'
					WHEN event_type = 'reporting.projection.update_required.v2' THEN 'projection'
					WHEN event_type LIKE 'reservation.compensation.%' THEN 'compensation'
					ELSE 'notification'
				END AS worker_kind,
				GREATEST(EXTRACT(EPOCH FROM published_at - created_at)::float8, 0::float8) AS lag_seconds
			FROM outbox_events
			WHERE publish_status = 'published'
				AND published_at IS NOT NULL
		)
		SELECT event_type, worker_kind,
			count(*) FILTER (WHERE lag_seconds <= 1),
			count(*) FILTER (WHERE lag_seconds <= 5),
			count(*) FILTER (WHERE lag_seconds <= 10),
			count(*) FILTER (WHERE lag_seconds <= 30),
			count(*) FILTER (WHERE lag_seconds <= 60),
			count(*) FILTER (WHERE lag_seconds <= 180),
			count(*) FILTER (WHERE lag_seconds <= 300),
			count(*) FILTER (WHERE lag_seconds <= 600),
			count(*) FILTER (WHERE lag_seconds <= 1800),
			count(*) FILTER (WHERE lag_seconds <= 3600),
			count(*),
			COALESCE(sum(lag_seconds), 0)::float8
		FROM outbox_published
		GROUP BY event_type, worker_kind
		ORDER BY worker_kind, event_type`)
	if err != nil {
		writeLine(w, "cets_metrics_scrape_errors_total{collector=\"outbox_lag_histogram\"} 1")
		return
	}
	defer rows.Close()

	aggregates := map[outboxLagHistogramKey]outboxLagHistogramAggregate{}
	for rows.Next() {
		var eventType, workerKind string
		var bucket1, bucket5, bucket10, bucket30, bucket60 int64
		var bucket180, bucket300, bucket600, bucket1800, bucket3600 int64
		var count int64
		var sum float64
		if err := rows.Scan(
			&eventType,
			&workerKind,
			&bucket1,
			&bucket5,
			&bucket10,
			&bucket30,
			&bucket60,
			&bucket180,
			&bucket300,
			&bucket600,
			&bucket1800,
			&bucket3600,
			&count,
			&sum,
		); err != nil {
			writeLine(w, "cets_metrics_scrape_errors_total{collector=\"outbox_lag_histogram\"} 1")
			return
		}
		key := normalizeOutboxLagHistogramKey(eventType, workerKind)
		addOutboxLagHistogramAggregate(aggregates, key, []int64{
			bucket1,
			bucket5,
			bucket10,
			bucket30,
			bucket60,
			bucket180,
			bucket300,
			bucket600,
			bucket1800,
			bucket3600,
		}, count, sum)
	}
	if err := rows.Err(); err != nil {
		writeLine(w, "cets_metrics_scrape_errors_total{collector=\"outbox_lag_histogram\"} 1")
		return
	}
	for _, aggregate := range sortedOutboxLagHistogramAggregates(aggregates) {
		writeOutboxLagHistogramRow(w, aggregate.Key.EventType, aggregate.Key.WorkerKind, aggregate.Buckets, aggregate.Count, aggregate.Sum)
	}
}

func writeOutboxLagHistogramRow(w io.Writer, eventType, workerKind string, bucketCounts []int64, count int64, sum float64) {
	eventType = safeOutboxMetricEventType(eventType)
	workerKind = safeOutboxMetricWorkerKind(eventType, workerKind)
	labels := fmt.Sprintf(`event_type="%s",worker_kind="%s"`, escapeLabel(eventType), escapeLabel(workerKind))
	for i, bucket := range outboxLagHistogramBuckets {
		writeFormat(w, "cets_outbox_lag_seconds_bucket{%s,le=\"%s\"} %d\n", labels, formatBucket(bucket), bucketCounts[i])
	}
	writeFormat(w, "cets_outbox_lag_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, count)
	writeFormat(w, "cets_outbox_lag_seconds_sum{%s} %s\n", labels, strconv.FormatFloat(sum, 'f', -1, 64))
	writeFormat(w, "cets_outbox_lag_seconds_count{%s} %d\n", labels, count)
}
