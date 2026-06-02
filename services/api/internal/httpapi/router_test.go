package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

type fakePinger struct {
	err error
}

func (p fakePinger) Ping(context.Context) error {
	return p.err
}

type fakeSchemaPinger struct {
	err         error
	schemaReady bool
	schemaErr   error
}

func (p fakeSchemaPinger) Ping(context.Context) error {
	return p.err
}

func (p fakeSchemaPinger) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return fakeReadyRow{ready: p.schemaReady, err: p.schemaErr}
}

type fakeReadyRow struct {
	ready bool
	err   error
}

func (r fakeReadyRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("expected one destination")
	}
	ready, ok := dest[0].(*bool)
	if !ok {
		return errors.New("expected bool destination")
	}
	*ready = r.ready
	return nil
}

type countingSchemaPinger struct {
	schemaReady bool
	pings       int
	queries     int
}

func (p *countingSchemaPinger) Ping(context.Context) error {
	p.pings++
	return nil
}

func (p *countingSchemaPinger) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	p.queries++
	return fakeReadyRow{ready: p.schemaReady}
}

func TestHealthz(t *testing.T) {
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":true`, `"status":"ok"`)
}

func TestRouterGeneratesTraceIDHeader(t *testing.T) {
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	traceID := rec.Header().Get("X-Trace-ID")
	require.NotEmpty(t, traceID, "expected generated X-Trace-ID response header")
	assert.True(t, strings.HasPrefix(traceID, "trc_"), "trace id = %q", traceID)
}

func TestRouterPreservesTraceIDInResponseContextAndLogs(t *testing.T) {
	var logs bytes.Buffer
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	body := bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", body)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	req.Header.Set("X-Trace-ID", "trace-test-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, "trace-test-123", rec.Header().Get("X-Trace-ID"))
	assert.Equal(t, "trace-test-123", service.createTrace)
	assertEnvelope(t, logs.String(), `"trace_id":"trace-test-123"`, `"path":"/api/v1/admin/events"`, `"status":201`)
	assert.NotContains(t, logs.String(), "otel_trace_id")
	assert.NotContains(t, logs.String(), "otel_span_id")
}

func TestRouterDoesNotEmitHTTPSpanWhenTracingDisabled(t *testing.T) {
	exporter, shutdown := installTestTracer(t)
	defer shutdown()

	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, exporter.GetSpans())
}

func TestRouterEmitsRouteBoundedHTTPSpanWhenTracingEnabled(t *testing.T) {
	exporter, shutdown := installTestTracer(t)
	defer shutdown()
	var logs bytes.Buffer

	router := testRouter(Dependencies{
		TracingEnabled: true,
		Logger:         slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "GET /healthz", spans[0].Name)
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.route", "/healthz"))
	assertEnvelope(t, logs.String(),
		`"route":"/healthz"`,
		`"otel_trace_id":"`+spans[0].SpanContext.TraceID().String()+`"`,
		`"otel_span_id":"`+spans[0].SpanContext.SpanID().String()+`"`,
	)
}

func TestRouterSetsBackendReplicaHeader(t *testing.T) {
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.NotEmpty(t, rec.Header().Get("X-CETS-Backend-Replica"))
}

func TestMetricsEndpointUsesRoutePatternsNotRawIdentifiers(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/evt_secret_token", nil)
	authorizeRequest(t, req, ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)

	require.Equal(t, http.StatusOK, metricsRec.Code)
	assert.Equal(t, "text/plain; version=0.0.4; charset=utf-8", metricsRec.Header().Get("Content-Type"))
	assertEnvelope(t, metricsRec.Body.String(),
		`cets_http_requests_total{route="/api/v1/events/{event_id}",method="GET",status_class="2xx"} 1`,
		`cets_http_request_seconds_bucket{route="/api/v1/events/{event_id}",method="GET",status_class="2xx"`,
	)
	assert.NotContains(t, metricsRec.Body.String(), "evt_secret_token")
}

func TestMetricsEndpointCollapsesUnmatchedRoutesToBoundedLabel(t *testing.T) {
	var logs bytes.Buffer
	router := testRouter(Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	req := httptest.NewRequest(http.MethodDelete, "/wp-login-secret", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)

	require.Equal(t, http.StatusOK, metricsRec.Code)
	assertEnvelope(t, metricsRec.Body.String(),
		`cets_http_requests_total{route="/unknown",method="DELETE",status_class="4xx"} 1`,
		`cets_http_request_seconds_bucket{route="/unknown",method="DELETE",status_class="4xx"`,
	)
	assertEnvelope(t, logs.String(), `"route":"/unknown"`, `"path":"/wp-login-secret"`)
	assert.NotContains(t, metricsRec.Body.String(), "wp-login-secret")
}

func TestMetricsEndpointNormalizesHeadRoutesFromGetPatterns(t *testing.T) {
	var logs bytes.Buffer
	router := testRouter(Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	router.ServeHTTP(metricsRec, metricsReq)

	require.Equal(t, http.StatusOK, metricsRec.Code)
	assertEnvelope(t, metricsRec.Body.String(),
		`cets_http_requests_total{route="/healthz",method="HEAD",status_class="2xx"} 1`,
		`cets_http_request_seconds_bucket{route="/healthz",method="HEAD",status_class="2xx"`,
	)
	assertEnvelope(t, logs.String(), `"method":"HEAD"`, `"route":"/healthz"`)
	assert.NotContains(t, metricsRec.Body.String(), `route="GET /healthz"`)
	assert.NotContains(t, logs.String(), `"route":"GET /healthz"`)
}

func TestIndexServesFallbackUIWithoutGeneratedAssets(t *testing.T) {
	useEmptyStaticRoot(t)
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assertEnvelope(t, rec.Body.String(),
		"企業活動票務系統",
		`id="root"`,
		"Frontend assets are not built",
	)
	assert.NotContains(t, rec.Body.String(), `type="module"`)
	assert.NotContains(t, rec.Body.String(), `/assets/`)
}

func TestIndexWithoutGeneratedAssetsFailsOutsideLocalEnvironments(t *testing.T) {
	useEmptyStaticRoot(t)
	router := testRouter(Dependencies{AppEnv: "production"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"frontend assets are not built"`)
	assert.NotContains(t, rec.Body.String(), `id="root"`)
}

func TestGeneratedIndexAndAssetsAreServedWhenBuilt(t *testing.T) {
	originalStaticRoot := staticRoot
	tempDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(tempDir, "assets"), 0o755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(tempDir, "index.html"),
			[]byte(`<!doctype html><div id="root"></div><script type="module" src="/assets/app.js"></script>`),
			0o644,
		),
	)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tempDir, "assets", "app.js"), []byte(`console.log("built")`), 0o644),
	)
	staticRoot = os.DirFS(tempDir)
	t.Cleanup(func() {
		staticRoot = originalStaticRoot
	})

	router := testRouter(Dependencies{})

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRec := httptest.NewRecorder()
	router.ServeHTTP(indexRec, indexReq)
	require.Equal(t, http.StatusOK, indexRec.Code)
	assertEnvelope(t, indexRec.Body.String(), `id="root"`, `type="module"`, `/assets/app.js`)

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetRec := httptest.NewRecorder()
	router.ServeHTTP(assetRec, assetReq)
	require.Equal(t, http.StatusOK, assetRec.Code)
	assert.Contains(t, assetRec.Body.String(), `console.log("built")`)
}

func TestReactSPARoutesServeIndex(t *testing.T) {
	router := testRouter(Dependencies{})
	paths := []string{
		"/user/events",
		"/user/tickets",
		"/admin/events",
		"/admin/checkin",
		"/admin/reports",
		"/admin/audit",
		"/admin/demo",
		"/employee/events",
		"/employee/tickets",
		"/checkin",
		"/hr/reports",
		"/demo",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assertEnvelope(t, rec.Body.String(), "企業活動票務系統", `id="root"`)
		})
	}
}

func TestMissingGeneratedAssets404(t *testing.T) {
	router := testRouter(Dependencies{})

	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUnknownAPIPathDoesNotServeSPA(t *testing.T) {
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"not found"`)
	assert.NotContains(t, rec.Body.String(), `id="root"`, "unknown API path served the React SPA")
}

func TestReactSourceKeepsPhase1UIContracts(t *testing.T) {
	source, err := readReactSourceTree(filepath.Join("..", "..", "..", "..", "apps", "web", "src"))
	require.NoError(t, err)
	content := fallbackIndexHTML + "\n" + source

	required := []string{
		"/employee/events",
		"/employee/tickets",
		"/user/events",
		"/user/tickets",
		"/admin/events",
		"/admin/checkin",
		"/admin/reports",
		"/admin/audit",
		"/admin/demo",
		"/checkin",
		"/hr/reports",
		"/demo",
		"/api/v1/auth/me",
		"/api/v1/auth/bootstrap",
		"/api/v1/auth/mock-provider-token",
		"ProviderClaimsCard",
		"mock_profiles",
		"選擇一個本機身分",
		"身分宣告",
		"Authorization",
		"credentials: \"same-origin\"",
		"Frontend assets are not built",
		"員工工作區",
		"管理工作台",
		"稽核中繼資料",
		"DebugChromeGate",
		"DemoRunbookPage",
		"發布檢查",
		"容量使用",
		"簽章碼保護",
		"二維碼入場",
		"稽核查詢",
		"ApiActivity",
		"已遮蔽",
		"ticketSignatureKeys",
		"\"signed_token\"",
		"\"qr_payload\"",
		"contentType.includes(\"application/json\")",
		"await response.text()",
	}
	for _, fragment := range required {
		assert.Contains(t, content, fragment, "React UI contract is missing %q", fragment)
	}
	forbidden := []string{
		"\"X-Actor-ID\"",
		"\"X-Role\"",
	}
	for _, fragment := range forbidden {
		assert.NotContains(t, content, fragment, "React UI contract still exposes legacy header %q", fragment)
	}
}

func readReactSourceTree(root string) (string, error) {
	var content strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.Contains(base, ".test.") {
			return nil
		}
		if !strings.HasSuffix(base, ".ts") &&
			!strings.HasSuffix(base, ".tsx") &&
			!strings.HasSuffix(base, ".json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content.WriteByte('\n')
		content.Write(data)
		return nil
	})
	return content.String(), err
}

func installTestTracer(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	return exporter, func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
}

func useEmptyStaticRoot(t *testing.T) {
	t.Helper()
	originalStaticRoot := staticRoot
	staticRoot = os.DirFS(t.TempDir())
	t.Cleanup(func() {
		staticRoot = originalStaticRoot
	})
}

func TestReadyzOK(t *testing.T) {
	router := testRouter(Dependencies{DB: fakeSchemaPinger{schemaReady: true}})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":true`, `"status":"ready"`)
}

func TestReadyzCachesDatabaseProbe(t *testing.T) {
	db := &countingSchemaPinger{schemaReady: true}
	router := testRouter(Dependencies{DB: db})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	require.Equal(t, 1, db.pings)
	require.Equal(t, 1, db.queries)
}

func TestReadyzRejectsUnmigratedDatabase(t *testing.T) {
	router := testRouter(Dependencies{DB: fakeSchemaPinger{schemaReady: false}})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"database schema is not ready"`)
}

func TestReadyzRequiresBookingBanSchema(t *testing.T) {
	assert.Contains(t, requiredSchemaReadyQuery, "to_regclass('public.booking_bans')")
}

func TestReadyzDatabaseUnavailable(t *testing.T) {
	router := testRouter(Dependencies{DB: fakePinger{err: errors.New("down")}})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"database is not ready"`)
}

func assertEnvelope(t *testing.T, body string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		assert.Contains(t, body, part, "response body %q does not contain %q", body, part)
	}
}
