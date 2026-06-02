package observability

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestHTTPMetricsExposeREDSignalsWithBoundedLabels(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest("/api/v1/events/{event_id}", "GET", 201, 80*time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_http_requests_total{route="/api/v1/events/{event_id}",method="GET",status_class="2xx"} 1`)
	assert.Contains(t, metrics, `cets_http_request_seconds_bucket{route="/api/v1/events/{event_id}",method="GET",status_class="2xx",le="0.1"} 1`)
	assert.Contains(t, metrics, `cets_http_request_seconds_count{route="/api/v1/events/{event_id}",method="GET",status_class="2xx"} 1`)
	assert.NotContains(t, metrics, "evt_secret")
}

func TestHTTPMetricsCollapseUnknownMethodsToBoundedLabel(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest("/unknown", "FOOBAR-SCANNER-TOKEN", 404, 5*time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_http_requests_total{route="/unknown",method="UNKNOWN",status_class="4xx"} 1`)
	assert.NotContains(t, metrics, "FOOBAR-SCANNER-TOKEN")
}

func TestHTTPMetricsNormalizeKnownMethodCase(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest("/healthz", "get", 200, time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)

	assert.Contains(t, body.String(), `method="GET"`)
}

func TestHTTPMetricsEscapeLabels(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest(`/api/"quoted"`, "GET", 500, time.Second)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)

	assert.Contains(t, body.String(), `route="/api/\"quoted\""`)
	assert.Contains(t, body.String(), `status_class="5xx"`)
}

func TestBookingMetricsExposeBoundedStageAndReservationSignals(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveBookingStage("event_lock", "confirmed", 120*time.Millisecond)
	registry.ObserveBookingStage("evt_secret", "E1001", 250*time.Millisecond)
	registry.ObserveReservationAttempt("granted", "limited", "degrade", 15*time.Millisecond)
	registry.ObserveReservationAttempt("raw-token", "email@example.test", "redis://secret", 20*time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_booking_stage_seconds_bucket{stage="event_lock",outcome="confirmed",le="0.25"} 1`)
	assert.Contains(t, metrics, `cets_booking_stage_seconds_bucket{stage="unknown",outcome="unknown",le="0.25"} 1`)
	assert.Contains(t, metrics, `cets_reservation_attempt_total{outcome="granted",capacity_type="limited",outage_mode="degrade"} 1`)
	assert.Contains(t, metrics, `cets_booking_preadmission_seconds_bucket{outcome="granted",capacity_type="limited",outage_mode="degrade",le="0.025"} 1`)
	assert.Contains(t, metrics, `cets_reservation_attempt_total{outcome="unknown",capacity_type="unknown",outage_mode="unknown"} 1`)
	assert.NotContains(t, metrics, "evt_secret")
	assert.NotContains(t, metrics, "E1001")
	assert.NotContains(t, metrics, "email@example.test")
	assert.NotContains(t, metrics, "redis://secret")
}

func TestOutboxMetricsExposeWorkerKindRetryAndDeadLetterWithoutPII(t *testing.T) {
	db := fakeSQLMetricsDB{
		lockWaitCount: 0,
		outboxRows: [][]any{
			{"notification.requested.v2", "notification", "dead_letter", int64(2), float64(42), int64(7), int64(2), float64(0)},
			{"reservation.compensation.release_required.v2", "compensation", "dead_letter", int64(1), float64(17), int64(2), int64(1), float64(0)},
			{"report.export.requested", "export", "processing", int64(1), float64(5), int64(1), int64(0), float64(11)},
		},
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_outbox_pending_total{event_type="notification.requested.v2",worker_kind="notification",status="dead_letter"} 2`)
	assert.Contains(t, metrics, `cets_outbox_oldest_lag_seconds{event_type="notification.requested.v2",worker_kind="notification",status="dead_letter"} 42`)
	assert.Contains(t, metrics, `cets_outbox_retry_count{event_type="notification.requested.v2",worker_kind="notification",status="dead_letter"} 7`)
	assert.Contains(t, metrics, `cets_outbox_dead_letter_total{event_type="notification.requested.v2",worker_kind="notification"} 2`)
	assert.Contains(t, metrics, `cets_outbox_oldest_lag_seconds{event_type="reservation.compensation.release_required.v2",worker_kind="compensation",status="dead_letter"} 17`)
	assert.Contains(t, metrics, `cets_outbox_retry_count{event_type="reservation.compensation.release_required.v2",worker_kind="compensation",status="dead_letter"} 2`)
	assert.Contains(t, metrics, `cets_outbox_dead_letter_total{event_type="reservation.compensation.release_required.v2",worker_kind="compensation"} 1`)
	assert.Contains(t, metrics, `cets_outbox_pending_total{event_type="report.export.requested",worker_kind="export",status="processing"} 1`)
	assert.Contains(t, metrics, `cets_outbox_lease_held_seconds{event_type="report.export.requested",worker_kind="export",status="processing"} 11`)
	assert.NotContains(t, metrics, "out_secret")
	assert.NotContains(t, metrics, "E1001")
	assert.NotContains(t, metrics, "e1001@cets.local")
	assert.NotContains(t, metrics, "idempotency")
}

func TestOutboxMetricsRedactUnsafeEventTypeLabels(t *testing.T) {
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJF1001xyz.XyZ_12345678"
	otherUnsafeEventType := "reservation.compensation.e2002@cets.local.eyJhbGciOiJIUzI1NiJ9.payload.signature"
	db := fakeSQLMetricsDB{
		lockWaitCount: 0,
		outboxRows: [][]any{
			{unsafeEventType, "notification", "dead_letter", int64(1), float64(9), int64(3), int64(1), float64(0)},
			{otherUnsafeEventType, "compensation", "dead_letter", int64(2), float64(12), int64(4), int64(2), float64(5)},
		},
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_outbox_pending_total{event_type="unknown",worker_kind="unknown",status="dead_letter"} 3`)
	assert.Contains(t, metrics, `cets_outbox_oldest_lag_seconds{event_type="unknown",worker_kind="unknown",status="dead_letter"} 12`)
	assert.Contains(t, metrics, `cets_outbox_retry_count{event_type="unknown",worker_kind="unknown",status="dead_letter"} 7`)
	assert.Contains(t, metrics, `cets_outbox_lease_held_seconds{event_type="unknown",worker_kind="unknown",status="dead_letter"} 5`)
	assert.Contains(t, metrics, `cets_outbox_dead_letter_total{event_type="unknown",worker_kind="unknown"} 3`)
	assert.NotContains(t, metrics, `event_type="unknown",worker_kind="notification"`)
	assert.NotContains(t, metrics, `event_type="unknown",worker_kind="compensation"`)
	assert.NotContains(t, metrics, "e1001@cets.local")
	assert.NotContains(t, metrics, "e2002@cets.local")
	assert.NotContains(t, metrics, "eyJhbGci")
	assert.NotContains(t, metrics, unsafeEventType)
	assert.NotContains(t, metrics, otherUnsafeEventType)
}

func TestOutboxLagHistogramExposesSafeWorkerKindBuckets(t *testing.T) {
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJF1001xyz.signature"
	otherUnsafeEventType := "reservation.compensation.e2002@cets.local.eyJhbGciOiJIUzI1NiJ9.payload.signature"
	queries := []string{}
	db := fakeSQLMetricsDB{
		lockWaitCount: 0,
		queries:       &queries,
		outboxLagHistogramRows: [][]any{
			{"notification.requested.v2", "notification", int64(0), int64(1), int64(1), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), float64(27.5)},
			{unsafeEventType, "notification", int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), float64(0.9)},
			{otherUnsafeEventType, "compensation", int64(0), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), int64(2), float64(4.2)},
		},
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `# TYPE cets_outbox_lag_seconds histogram`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_bucket{event_type="notification.requested.v2",worker_kind="notification",le="1"} 0`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_bucket{event_type="notification.requested.v2",worker_kind="notification",le="5"} 1`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_bucket{event_type="notification.requested.v2",worker_kind="notification",le="30"} 2`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_bucket{event_type="notification.requested.v2",worker_kind="notification",le="+Inf"} 2`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_sum{event_type="notification.requested.v2",worker_kind="notification"} 27.5`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_count{event_type="notification.requested.v2",worker_kind="notification"} 2`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_bucket{event_type="unknown",worker_kind="unknown",le="1"} 1`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_bucket{event_type="unknown",worker_kind="unknown",le="5"} 3`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_bucket{event_type="unknown",worker_kind="unknown",le="+Inf"} 3`)
	assert.Contains(t, metrics, `cets_outbox_lag_seconds_sum{event_type="unknown",worker_kind="unknown"} 5.1`)
	assert.NotContains(t, metrics, `event_type="unknown",worker_kind="notification"`)
	assert.NotContains(t, metrics, `event_type="unknown",worker_kind="compensation"`)
	assert.NotContains(t, metrics, "e1001@cets.local")
	assert.NotContains(t, metrics, "e2002@cets.local")
	assert.NotContains(t, metrics, "eyJhbGci")
	assert.NotContains(t, metrics, unsafeEventType)
	assert.NotContains(t, metrics, otherUnsafeEventType)
	histogramQuery := queryContaining(t, queries, "outbox_lag_histogram")
	assert.Contains(t, histogramQuery, "publish_status = 'published'")
	assert.Contains(t, histogramQuery, "published_at - created_at")
	assert.NotContains(t, histogramQuery, "now() - created_at")
}

func TestWorkerOutcomeMetricsExposeAuditBackedRetryAndDeadLetterCounters(t *testing.T) {
	unsafeReason := "smtp rejected e1001@cets.local with token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJtZXRyaWMifQ.signature"
	db := fakeSQLMetricsDB{
		lockWaitCount: 0,
		workerOutcomeRows: [][]any{
			{"outbox.retry_scheduled", "notification", "retryable_failure", int64(4)},
			{"outbox.dead_letter", "export", "", int64(2)},
			{"outbox.retry_scheduled", "E1001", unsafeReason, int64(1)},
		},
	}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `# TYPE cets_worker_retry_total counter`)
	assert.Contains(t, metrics, `# TYPE cets_worker_deadletter_total counter`)
	assert.Contains(t, metrics, `cets_worker_retry_total{worker_kind="notification",reason="retryable_failure"} 4`)
	assert.Contains(t, metrics, `cets_worker_deadletter_total{worker_kind="export"} 2`)
	assert.Contains(t, metrics, `cets_worker_retry_total{worker_kind="unknown",reason="unknown"} 1`)
	assert.NotContains(t, metrics, "E1001")
	assert.NotContains(t, strings.ToLower(metrics), "e1001@cets.local")
	assert.NotContains(t, metrics, "eyJhbGci")
	assert.NotContains(t, metrics, unsafeReason)
}

type fakeSQLMetricsDB struct {
	lockWaitCount               int64
	queries                     *[]string
	outboxRows                  [][]any
	outboxLagHistogramRows      [][]any
	workerOutcomeRows           [][]any
	reservationCompensationRows [][]any
}

func (db fakeSQLMetricsDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return fakeMetricRow{value: db.lockWaitCount}
}

func (db fakeSQLMetricsDB) Query(_ context.Context, query string, _ ...interface{}) (pgx.Rows, error) {
	if db.queries != nil {
		*db.queries = append(*db.queries, query)
	}
	if strings.Contains(query, "outbox_lag_histogram") {
		return &fakeMetricRows{rows: db.outboxLagHistogramRows}, nil
	}
	if strings.Contains(query, "worker_outbox_audit_totals") {
		return &fakeMetricRows{rows: db.workerOutcomeRows}, nil
	}
	if strings.Contains(query, "reservation_compensation_metric_totals") {
		return &fakeMetricRows{rows: db.reservationCompensationRows}, nil
	}
	return &fakeMetricRows{rows: db.outboxRows}, nil
}

func queryContaining(t *testing.T, queries []string, needle string) string {
	t.Helper()
	for _, query := range queries {
		if strings.Contains(query, needle) {
			return query
		}
	}
	t.Fatalf("query containing %q was not executed", needle)
	return ""
}

type fakeMetricRow struct {
	value int64
}

func (r fakeMetricRow) Scan(dest ...interface{}) error {
	if len(dest) != 1 {
		return errors.New("expected one destination")
	}
	return assignMetricDest(dest[0], r.value)
}

type fakeMetricRows struct {
	rows  [][]any
	index int
}

func (r *fakeMetricRows) Close() {
}

func (r *fakeMetricRows) Err() error {
	return nil
}

func (r *fakeMetricRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *fakeMetricRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *fakeMetricRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.index++
	return true
}

func (r *fakeMetricRows) Scan(dest ...any) error {
	if r.index == 0 || r.index > len(r.rows) {
		return errors.New("scan called without current row")
	}
	row := r.rows[r.index-1]
	if len(dest) != len(row) {
		return errors.New("destination count does not match row")
	}
	for i := range dest {
		if err := assignMetricDest(dest[i], row[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeMetricRows) Values() ([]any, error) {
	if r.index == 0 || r.index > len(r.rows) {
		return nil, errors.New("values called without current row")
	}
	return r.rows[r.index-1], nil
}

func (r *fakeMetricRows) RawValues() [][]byte {
	return nil
}

func (r *fakeMetricRows) Conn() *pgx.Conn {
	return nil
}

func assignMetricDest(dest any, value any) error {
	switch target := dest.(type) {
	case *string:
		value, ok := value.(string)
		if !ok {
			return errors.New("expected string source")
		}
		*target = value
	case *int64:
		value, ok := value.(int64)
		if !ok {
			return errors.New("expected int64 source")
		}
		*target = value
	case *float64:
		value, ok := value.(float64)
		if !ok {
			return errors.New("expected float64 source")
		}
		*target = value
	default:
		return errors.New("unsupported scan destination")
	}
	return nil
}
