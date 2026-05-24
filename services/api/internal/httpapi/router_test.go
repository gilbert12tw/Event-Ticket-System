package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestHealthz(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":true`, `"status":"ok"`)
}

func TestRouterGeneratesTraceIDHeader(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
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
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewJSONHandler(&logs, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	body := bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", body)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	req.Header.Set("X-Trace-ID", "trace-test-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, "trace-test-123", rec.Header().Get("X-Trace-ID"))
	assert.Equal(t, "trace-test-123", service.createTrace)
	assertEnvelope(t, logs.String(), `"trace_id":"trace-test-123"`, `"path":"/api/v1/admin/events"`, `"status":201`)
}

func TestIndexServesFallbackUIWithoutGeneratedAssets(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	assertEnvelope(t, rec.Body.String(), "企業活動票務系統", `id="root"`)
	assert.NotContains(t, rec.Body.String(), `type="module"`)
	assert.NotContains(t, rec.Body.String(), `/assets/`)
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

	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})

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
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
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
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUnknownAPIPathDoesNotServeSPA(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
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
		if !strings.HasSuffix(base, ".ts") && !strings.HasSuffix(base, ".tsx") {
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

func TestReadyzOK(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakeSchemaPinger{schemaReady: true},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":true`, `"status":"ready"`)
}

func TestReadyzRejectsUnmigratedDatabase(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakeSchemaPinger{schemaReady: false},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"database schema is not ready"`)
}

func TestReadyzDatabaseUnavailable(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{err: errors.New("down")},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
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
