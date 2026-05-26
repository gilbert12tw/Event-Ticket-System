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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var httpBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type SQLMetricsDB interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type PoolStater interface {
	Stat() *pgxpool.Stat
}

type Registry struct {
	mu   sync.Mutex
	http map[httpKey]*histogram
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
	return &Registry{http: map[httpKey]*histogram{}}
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

func writePoolMetrics(w io.Writer, db any) {
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

func writeSQLMetrics(ctx context.Context, w io.Writer, db any) {
	sqlDB, ok := db.(SQLMetricsDB)
	if !ok {
		return
	}
	writeLockWaitMetric(ctx, w, sqlDB)
	writeOutboxMetrics(ctx, w, sqlDB)
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
	writeLine(w, "# HELP cets_outbox_oldest_lag_seconds Age of the oldest outbox row still requiring worker attention.")
	writeLine(w, "# TYPE cets_outbox_oldest_lag_seconds gauge")
	rows, err := db.Query(ctx, `SELECT event_type, publish_status, count(*),
			COALESCE(EXTRACT(EPOCH FROM now() - min(created_at)), 0)
		FROM outbox_events
		WHERE publish_status IN ('pending', 'processing', 'dead_letter')
		GROUP BY event_type, publish_status
		ORDER BY event_type, publish_status`)
	if err != nil {
		writeLine(w, "cets_metrics_scrape_errors_total{collector=\"outbox\"} 1")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var eventType, status string
		var count int64
		var oldestLag float64
		if err := rows.Scan(&eventType, &status, &count, &oldestLag); err != nil {
			writeLine(w, "cets_metrics_scrape_errors_total{collector=\"outbox\"} 1")
			return
		}
		labels := fmt.Sprintf(`event_type="%s",status="%s"`, escapeLabel(eventType), escapeLabel(status))
		writeFormat(w, "cets_outbox_pending_total{%s} %d\n", labels, count)
		writeFormat(w, "cets_outbox_oldest_lag_seconds{%s} %s\n", labels, strconv.FormatFloat(oldestLag, 'f', -1, 64))
	}
	if err := rows.Err(); err != nil {
		writeLine(w, "cets_metrics_scrape_errors_total{collector=\"outbox\"} 1")
	}
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

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func formatBucket(bucket float64) string {
	return strconv.FormatFloat(bucket, 'f', -1, 64)
}
