package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"event-ticket-system/internal/observability"
	"event-ticket-system/internal/ticketing"
	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/baggage"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type schemaPinger interface {
	Pinger
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type Dependencies struct {
	DB                          Pinger
	MetricsDB                   any
	Ticketing                   TicketingService
	ReadTicketing               TicketingService
	Logger                      *slog.Logger
	Metrics                     *observability.Registry
	DemoClock                   *ticketing.DemoClock
	TracingEnabled              bool
	RequestTimeout              time.Duration
	AppEnv                      string
	OpsAPIEnabled               bool
	ProviderAuth                ProviderAuthConfig
	ReportStaleThresholdSeconds int
	ReportStore                 ticketing.ReportObjectReader
	ObjectStore                 objectStore
}

func NewRouter(deps Dependencies) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.RequestTimeout <= 0 {
		deps.RequestTimeout = 5 * time.Second
	}
	if deps.Metrics == nil {
		deps.Metrics = observability.NewRegistry()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex(deps.AppEnv))
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /readyz", handleReady(deps.DB, deps.RequestTimeout))
	metricsDB := deps.MetricsDB
	if metricsDB == nil {
		metricsDB = deps.DB
	}
	mux.Handle("GET /metrics", deps.Metrics.Handler(metricsDB))
	provider := NewProviderVerifier(deps.ProviderAuth)
	registerAuthRoutes(mux, provider, deps.AppEnv, deps.DemoClock != nil, deps.OpsAPIEnabled, deps.Logger)
	registerDemoDebugRoutes(mux, provider, deps.DemoClock, deps.Logger)
	readTicketing := deps.ReadTicketing
	if readTicketing == nil {
		readTicketing = deps.Ticketing
	}
	reportStore := deps.ReportStore
	if reportStore == nil && deps.ObjectStore != nil {
		reportStore = deps.ObjectStore
	}
	registerTicketingRoutes(mux, deps.Ticketing, readTicketing, ticketingRouteConfig{
		appEnv:                      deps.AppEnv,
		provider:                    provider,
		opsAPIEnabled:               deps.OpsAPIEnabled,
		reportStaleThresholdSeconds: deps.ReportStaleThresholdSeconds,
		reportStore:                 reportStore,
		objectStore:                 deps.ObjectStore,
		logger:                      deps.Logger,
	})

	replica := backendReplicaName()
	handler := withHTTPMetrics(deps.Metrics, withRequestLogging(deps.Logger, replica, withTimeout(deps.RequestTimeout, mux)))
	if deps.TracingEnabled {
		handler = observability.TraceHTTP(routePattern, handler)
	}
	return withBackendReplicaHeader(replica, withTraceID(handler))
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReady(db Pinger, timeout time.Duration) http.HandlerFunc {
	var mu sync.Mutex
	var cached readyCheck
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		mu.Lock()
		if cached.expiresAt.After(now) {
			result := cached
			mu.Unlock()
			writeReadyResult(w, result)
			return
		}
		mu.Unlock()

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result := checkReady(ctx, db)
		result.expiresAt = now.Add(readyCacheTTL)
		mu.Lock()
		cached = result
		mu.Unlock()
		writeReadyResult(w, result)
	}
}

const readyCacheTTL = time.Second

type readyCheck struct {
	status    int
	message   string
	expiresAt time.Time
}

func checkReady(ctx context.Context, db Pinger) readyCheck {
	if db == nil {
		return readyCheck{status: http.StatusServiceUnavailable, message: "database is not configured"}
	}
	if err := db.Ping(ctx); err != nil {
		return readyCheck{status: http.StatusServiceUnavailable, message: "database is not ready"}
	}
	if schemaDB, ok := db.(schemaPinger); ok {
		if err := checkRequiredSchema(ctx, schemaDB); err != nil {
			return readyCheck{status: http.StatusServiceUnavailable, message: "database schema is not ready"}
		}
	}
	return readyCheck{status: http.StatusOK}
}

func writeReadyResult(w http.ResponseWriter, result readyCheck) {
	if result.status != http.StatusOK {
		writeError(w, result.status, result.message)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

var errRequiredSchemaMissing = errors.New("required database schema is not ready")

const requiredSchemaReadyQuery = `SELECT
	to_regclass('public.employees') IS NOT NULL AND
	to_regclass('public.events') IS NOT NULL AND
	to_regclass('public.registrations') IS NOT NULL AND
	to_regclass('public.tickets') IS NOT NULL AND
	to_regclass('public.checkin_records') IS NOT NULL AND
	to_regclass('public.audit_logs') IS NOT NULL AND
	to_regclass('public.outbox_events') IS NOT NULL AND
	to_regclass('public.notification_deliveries') IS NOT NULL AND
	to_regclass('public.booking_bans') IS NOT NULL AND
	to_regclass('public.offline_checkin_batches') IS NOT NULL AND
	to_regclass('public.report_exports') IS NOT NULL AND
	to_regclass('public.lottery_results') IS NOT NULL`

func checkRequiredSchema(ctx context.Context, db schemaPinger) error {
	var ready bool
	if err := db.QueryRow(ctx, requiredSchemaReadyQuery).Scan(&ready); err != nil {
		return err
	}
	if !ready {
		return errRequiredSchemaMissing
	}
	return nil
}

func withTimeout(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		timedRequest := r.WithContext(ctx)
		next.ServeHTTP(w, timedRequest)
		r.Pattern = timedRequest.Pattern
	})
}

const requestHandledMsg = "request handled"

func withRequestLogging(logger *slog.Logger, replica string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		attrs := []any{
			"trace_id", traceid.FromContext(r.Context()),
			"method", r.Method,
			"route", routePattern(r),
			"path", r.URL.Path,
			"status", recorder.status,
			"status_class", statusClass(recorder.status),
			"duration_ms", time.Since(started).Milliseconds(),
		}
		if replica != "" {
			attrs = append(attrs, "replica", replica)
		}
		if otelTraceID := otelTraceIDFromContext(r.Context()); otelTraceID != "" {
			attrs = append(attrs, "otel_trace_id", otelTraceID)
			if spanCtx := oteltrace.SpanContextFromContext(r.Context()); spanCtx.IsValid() {
				attrs = append(attrs, "otel_span_id", spanCtx.SpanID().String())
			}
		}
		switch {
		case recorder.status >= 500:
			logger.Error(requestHandledMsg, attrs...)
		case recorder.status >= 400:
			logger.Warn(requestHandledMsg, attrs...)
		default:
			logger.Info(requestHandledMsg, attrs...)
		}
	})
}

func withHTTPMetrics(metrics *observability.Registry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		metrics.ObserveHTTPRequest(routePattern(r), r.Method, recorder.status, time.Since(started))
	})
}

func withTraceID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := traceid.Ensure(r.Header.Get(traceid.Header))
		w.Header().Set(traceid.Header, id)
		ctx := traceid.WithContext(r.Context(), id)
		if member, err := baggage.NewMember("cets.trace_id", id); err == nil {
			if bag, err := baggage.New(member); err == nil {
				ctx = baggage.ContextWithBaggage(ctx, bag)
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withBackendReplicaHeader(replica string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if replica != "" {
			w.Header().Set("X-CETS-Backend-Replica", replica)
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func routePattern(r *http.Request) string {
	pattern := strings.TrimSpace(r.Pattern)
	if pattern == "" {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			return "/api/unknown"
		}
		return "/unknown"
	}
	if _, route, ok := strings.Cut(pattern, " "); ok && strings.HasPrefix(route, "/") {
		return route
	}
	return pattern
}

func statusClass(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return string(rune('0'+status/100)) + "xx"
}

func otelTraceIDFromContext(ctx context.Context) string {
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() || !spanContext.HasTraceID() {
		return ""
	}
	return spanContext.TraceID().String()
}

func backendReplicaName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(hostname)
}
