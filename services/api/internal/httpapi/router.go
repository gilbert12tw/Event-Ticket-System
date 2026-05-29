package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"event-ticket-system/internal/observability"
	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
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
	Ticketing                   TicketingService
	Logger                      *slog.Logger
	Metrics                     *observability.Registry
	RequestTimeout              time.Duration
	AppEnv                      string
	ProviderAuth                ProviderAuthConfig
	ReportStaleThresholdSeconds int
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
	mux.Handle("GET /metrics", deps.Metrics.Handler(deps.DB))
	provider := NewProviderVerifier(deps.ProviderAuth)
	registerAuthRoutes(mux, provider, deps.AppEnv, deps.Logger)
	registerTicketingRoutes(mux, deps.Ticketing, deps.AppEnv, provider, deps.ReportStaleThresholdSeconds, deps.Logger)

	return withTraceID(withHTTPMetrics(deps.Metrics, withRequestLogging(deps.Logger, withTimeout(deps.RequestTimeout, mux))))
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReady(db Pinger, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeError(w, http.StatusServiceUnavailable, "database is not configured")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database is not ready")
			return
		}
		if schemaDB, ok := db.(schemaPinger); ok {
			if err := checkRequiredSchema(ctx, schemaDB); err != nil {
				writeError(w, http.StatusServiceUnavailable, "database schema is not ready")
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
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

func withRequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Info("request handled",
			"trace_id", traceid.FromContext(r.Context()),
			"method", r.Method,
			"route", routePattern(r),
			"path", r.URL.Path,
			"status", recorder.status,
			"status_class", statusClass(recorder.status),
			"duration_ms", time.Since(started).Milliseconds(),
		)
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
		next.ServeHTTP(w, r.WithContext(traceid.WithContext(r.Context(), id)))
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
