package observability

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"event-ticket-system/internal/eventcontract"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var httpBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
var allowedOutboxMetricEventTypes = buildAllowedOutboxMetricEventTypes()

const outboxScrapeErrorMetric = "cets_metrics_scrape_errors_total{collector=\"outbox\"} 1"

type SQLMetricsDB interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type PoolStater interface {
	Stat() *pgxpool.Stat
}

type PoolMetricsSource interface {
	PoolStats() map[string]PoolStater
}

type DatabaseMetrics struct {
	Write SQLMetricsDB
	Read  PoolStater
}

func (m DatabaseMetrics) PoolStats() map[string]PoolStater {
	pools := map[string]PoolStater{}
	if write, ok := m.Write.(PoolStater); ok {
		pools["write"] = write
	}
	if m.Read != nil {
		pools["read"] = m.Read
	}
	return pools
}

func (m DatabaseMetrics) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return m.Write.Query(ctx, sql, args...)
}

func (m DatabaseMetrics) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return m.Write.QueryRow(ctx, sql, args...)
}

type Registry struct {
	mu          sync.Mutex
	http        map[httpKey]*histogram
	booking     map[bookingStageKey]*histogram
	reservation map[reservationKey]*histogram
}

type httpKey struct {
	Route       string
	Method      string
	StatusClass string
}

type histogram struct {
	Buckets []uint64
	Count   uint64
	Sum     float64
}

func NewRegistry() *Registry {
	return &Registry{
		http:        map[httpKey]*histogram{},
		booking:     map[bookingStageKey]*histogram{},
		reservation: map[reservationKey]*histogram{},
	}
}

func (r *Registry) ObserveHTTPRequest(route string, method string, status int, duration time.Duration) {
	if r == nil {
		return
	}
	key := httpKey{
		Route:       boundedLabel(route, "unknown"),
		Method:      boundedMethod(method),
		StatusClass: statusClass(status),
	}
	seconds := duration.Seconds()

	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.http[key]
	if h == nil {
		h = &histogram{Buckets: make([]uint64, len(httpBuckets))}
		r.http[key] = h
	}
	for i, bucket := range httpBuckets {
		if seconds <= bucket {
			h.Buckets[i]++
		}
	}
	h.Count++
	h.Sum += seconds
}

type bookingStageKey struct {
	Stage   string
	Outcome string
}

func (r *Registry) ObserveBookingStage(stage string, outcome string, duration time.Duration) {
	if r == nil {
		return
	}
	key := bookingStageKey{
		Stage:   boundedBookingStage(stage),
		Outcome: boundedOutcome(outcome),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.booking[key]
	if h == nil {
		h = &histogram{Buckets: make([]uint64, len(httpBuckets))}
		r.booking[key] = h
	}
	observeDuration(h, duration.Seconds())
}

type reservationKey struct {
	Outcome      string
	CapacityType string
	OutageMode   string
}

func (r *Registry) ObserveReservationAttempt(outcome string, capacityType string, outageMode string, duration time.Duration) {
	if r == nil {
		return
	}
	key := reservationKey{
		Outcome:      boundedOutcome(outcome),
		CapacityType: boundedCapacityType(capacityType),
		OutageMode:   boundedOutageMode(outageMode),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.reservation[key]
	if h == nil {
		h = &histogram{Buckets: make([]uint64, len(httpBuckets))}
		r.reservation[key] = h
	}
	observeDuration(h, duration.Seconds())
}

func observeDuration(h *histogram, seconds float64) {
	for i, bucket := range httpBuckets {
		if seconds <= bucket {
			h.Buckets[i]++
		}
	}
	h.Count++
	h.Sum += seconds
}

func (r *Registry) Handler(db any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.WritePrometheus(req.Context(), w, db)
	})
}

func (r *Registry) WritePrometheus(ctx context.Context, w io.Writer, db any) {
	if r == nil {
		r = NewRegistry()
	}
	r.writeHTTPMetrics(w)
	r.writeBookingMetrics(w)
	r.writeReservationMetrics(w)
	writePoolMetrics(w, db)
	writeSQLMetrics(ctx, w, db)
}

func (r *Registry) writeHTTPMetrics(w io.Writer) {
	writeLine(w, "# HELP cets_http_requests_total HTTP requests by route, method, and status class.")
	writeLine(w, "# TYPE cets_http_requests_total counter")
	writeLine(w, "# HELP cets_http_request_seconds HTTP request duration histogram by route, method, and status class.")
	writeLine(w, "# TYPE cets_http_request_seconds histogram")

	r.mu.Lock()
	keys := make([]httpKey, 0, len(r.http))
	for key := range r.http {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return labelSet(keys[i]) < labelSet(keys[j])
	})
	snapshots := make(map[httpKey]histogram, len(keys))
	for _, key := range keys {
		current := r.http[key]
		snapshots[key] = histogram{
			Buckets: append([]uint64(nil), current.Buckets...),
			Count:   current.Count,
			Sum:     current.Sum,
		}
	}
	r.mu.Unlock()

	for _, key := range keys {
		labels := labelSet(key)
		h := snapshots[key]
		writeFormat(w, "cets_http_requests_total{%s} %d\n", labels, h.Count)
		for i, bucket := range httpBuckets {
			writeFormat(w, "cets_http_request_seconds_bucket{%s,le=%q} %d\n", labels, formatBucket(bucket), h.Buckets[i])
		}
		writeFormat(w, "cets_http_request_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, h.Count)
		writeFormat(w, "cets_http_request_seconds_sum{%s} %s\n", labels, strconv.FormatFloat(h.Sum, 'f', -1, 64))
		writeFormat(w, "cets_http_request_seconds_count{%s} %d\n", labels, h.Count)
	}
}

func (r *Registry) writeBookingMetrics(w io.Writer) {
	writeLine(w, "# HELP cets_booking_stage_seconds Booking hot-path stage duration histogram by bounded stage and outcome.")
	writeLine(w, "# TYPE cets_booking_stage_seconds histogram")

	r.mu.Lock()
	keys := make([]bookingStageKey, 0, len(r.booking))
	for key := range r.booking {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bookingStageLabelSet(keys[i]) < bookingStageLabelSet(keys[j])
	})
	snapshots := make(map[bookingStageKey]histogram, len(keys))
	for _, key := range keys {
		current := r.booking[key]
		snapshots[key] = histogram{
			Buckets: append([]uint64(nil), current.Buckets...),
			Count:   current.Count,
			Sum:     current.Sum,
		}
	}
	r.mu.Unlock()

	for _, key := range keys {
		labels := bookingStageLabelSet(key)
		h := snapshots[key]
		for i, bucket := range httpBuckets {
			writeFormat(w, "cets_booking_stage_seconds_bucket{%s,le=%q} %d\n", labels, formatBucket(bucket), h.Buckets[i])
		}
		writeFormat(w, "cets_booking_stage_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, h.Count)
		writeFormat(w, "cets_booking_stage_seconds_sum{%s} %s\n", labels, strconv.FormatFloat(h.Sum, 'f', -1, 64))
		writeFormat(w, "cets_booking_stage_seconds_count{%s} %d\n", labels, h.Count)
	}
}

func (r *Registry) writeReservationMetrics(w io.Writer) {
	writeLine(w, "# HELP cets_reservation_attempt_total Reservation pre-admission attempts by bounded outcome, capacity type, and outage mode.")
	writeLine(w, "# TYPE cets_reservation_attempt_total counter")
	writeLine(w, "# HELP cets_booking_preadmission_seconds Reservation pre-admission latency histogram by bounded outcome, capacity type, and outage mode.")
	writeLine(w, "# TYPE cets_booking_preadmission_seconds histogram")

	r.mu.Lock()
	keys := make([]reservationKey, 0, len(r.reservation))
	for key := range r.reservation {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return reservationLabelSet(keys[i]) < reservationLabelSet(keys[j])
	})
	snapshots := make(map[reservationKey]histogram, len(keys))
	for _, key := range keys {
		current := r.reservation[key]
		snapshots[key] = histogram{
			Buckets: append([]uint64(nil), current.Buckets...),
			Count:   current.Count,
			Sum:     current.Sum,
		}
	}
	r.mu.Unlock()

	for _, key := range keys {
		labels := reservationLabelSet(key)
		h := snapshots[key]
		writeFormat(w, "cets_reservation_attempt_total{%s} %d\n", labels, h.Count)
		for i, bucket := range httpBuckets {
			writeFormat(w, "cets_booking_preadmission_seconds_bucket{%s,le=%q} %d\n", labels, formatBucket(bucket), h.Buckets[i])
		}
		writeFormat(w, "cets_booking_preadmission_seconds_bucket{%s,le=\"+Inf\"} %d\n", labels, h.Count)
		writeFormat(w, "cets_booking_preadmission_seconds_sum{%s} %s\n", labels, strconv.FormatFloat(h.Sum, 'f', -1, 64))
		writeFormat(w, "cets_booking_preadmission_seconds_count{%s} %d\n", labels, h.Count)
	}
}

func buildAllowedOutboxMetricEventTypes() map[string]struct{} {
	allowed := map[string]struct{}{
		"booking.confirmed":                 {},
		"booking.received":                  {},
		"booking.waitlisted":                {},
		"eligibility.impact_review.created": {},
		"hr_sync.completed":                 {},
		"lottery.completed":                 {},
		"registration.cancelled":            {},
		"registration.no_show_recorded":     {},
		"report.export.requested":           {},
		"ticket.expired":                    {},
		"ticket.issued":                     {},
		"ticket.redeemed":                   {},
		"ticket.revoked":                    {},
		"waitlist.promoted":                 {},
	}
	for _, eventType := range eventcontract.Registry {
		allowed[eventType] = struct{}{}
	}
	return allowed
}

func writeLine(w io.Writer, line string) {
	_, _ = fmt.Fprintln(w, line)
}

func writeFormat(w io.Writer, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func labelSet(key httpKey) string {
	return fmt.Sprintf(`route="%s",method="%s",status_class="%s"`,
		escapeLabel(key.Route), escapeLabel(key.Method), escapeLabel(key.StatusClass))
}

func bookingStageLabelSet(key bookingStageKey) string {
	return fmt.Sprintf(`stage="%s",outcome="%s"`,
		escapeLabel(key.Stage), escapeLabel(key.Outcome))
}

func reservationLabelSet(key reservationKey) string {
	return fmt.Sprintf(`outcome="%s",capacity_type="%s",outage_mode="%s"`,
		escapeLabel(key.Outcome), escapeLabel(key.CapacityType), escapeLabel(key.OutageMode))
}

func statusClass(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return fmt.Sprintf("%dxx", status/100)
}

func boundedLabel(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 160 {
		return value[:160]
	}
	return value
}

// allowedMethods is the fixed set of HTTP methods the method label may take.
// Go's net/http accepts any RFC 7230 token as a method and reaches the handler
// even for unmatched requests, so collapsing anything outside this set keeps the
// method label bounded-cardinality and prevents an unauthenticated scanner from
// minting unbounded metric series via arbitrary method tokens.
var allowedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodConnect: {},
	http.MethodOptions: {},
	http.MethodTrace:   {},
}

func boundedMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if _, ok := allowedMethods[method]; ok {
		return method
	}
	return "UNKNOWN"
}

func boundedBookingStage(stage string) string {
	stage = strings.TrimSpace(stage)
	switch stage {
	case "total", "tx", "idempotency_replay", "preadmission", "begin_tx", "idempotency_lock", "event_lock", "validate", "duplicate_lookup", "capacity", "create_response", "commit":
		return stage
	default:
		return "unknown"
	}
}

func boundedOutcome(outcome string) string {
	outcome = strings.TrimSpace(outcome)
	switch outcome {
	case "success", "error", "confirmed", "waitlisted", "received", "duplicate", "granted", "exhausted", "misconfigured", "unavailable", "skipped":
		return outcome
	default:
		return "unknown"
	}
}

func boundedCapacityType(capacityType string) string {
	capacityType = strings.TrimSpace(capacityType)
	switch capacityType {
	case "limited", "unlimited", "unknown":
		return capacityType
	default:
		return "unknown"
	}
}

func boundedOutageMode(outageMode string) string {
	outageMode = strings.TrimSpace(outageMode)
	switch outageMode {
	case "degrade", "fail", "none", "unknown":
		return outageMode
	default:
		return "unknown"
	}
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func formatBucket(bucket float64) string {
	return strconv.FormatFloat(bucket, 'f', -1, 64)
}
