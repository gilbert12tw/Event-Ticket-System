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

	"github.com/jackc/pgx/v5"
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

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
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
	if traceID == "" {
		t.Fatal("expected generated X-Trace-ID response header")
	}
	if !strings.HasPrefix(traceID, "trc_") {
		t.Fatalf("trace id = %q", traceID)
	}
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
	})
	body := bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", body)
	req.Header.Set("X-Actor-ID", "admin-1")
	req.Header.Set("X-Role", "activity_admin")
	req.Header.Set("X-Trace-ID", "trace-test-123")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Header().Get("X-Trace-ID") != "trace-test-123" {
		t.Fatalf("response trace id = %q", rec.Header().Get("X-Trace-ID"))
	}
	if service.createTrace != "trace-test-123" {
		t.Fatalf("service context trace id = %q", service.createTrace)
	}
	assertEnvelope(t, logs.String(), `"trace_id":"trace-test-123"`, `"path":"/api/v1/admin/events"`, `"status":201`)
}

func TestIndexServesDemoUI(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	assertEnvelope(t, rec.Body.String(), "企業活動票務系統", `id="root"`, `type="module"`, `/assets/`)
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

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			assertEnvelope(t, rec.Body.String(), "企業活動票務系統", `id="root"`)
		})
	}
}

func TestStaticAssetIsServedAndMissingAssets404(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
	})
	entries, err := os.ReadDir("static/assets")
	if err != nil {
		t.Fatal(err)
	}
	var script string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".js") {
			script = "/assets/" + entry.Name()
			break
		}
	}
	if script == "" {
		t.Fatal("expected generated React script asset")
	}

	req := httptest.NewRequest(http.MethodGet, script, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d", rec.Code)
	}
	assertEnvelope(t, rec.Body.String(), "React root node is missing")

	req = httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", rec.Code)
	}
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

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"not found"`)
	if strings.Contains(rec.Body.String(), `id="root"`) {
		t.Fatal("unknown API path served the React SPA")
	}
}

func TestReactSourceKeepsPhase1UIContracts(t *testing.T) {
	html, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	source, err := readReactSourceTree(filepath.Join("..", "..", "..", "..", "apps", "web", "src"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(html) + "\n" + source

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
		"/api/v1/auth/login",
		"/api/v1/auth/me",
		"/api/v1/auth/logout",
		"Local SSO 模擬登入",
		"選擇一個企業身分",
		"credentials: \"same-origin\"",
		"User Workspace",
		"Admin Console",
		"Metadata detail drawer",
		"Demo helper",
		"Demo Runbook",
		"發布檢查",
		"容量使用",
		"Signed token 已保留",
		"QR 可用",
		"Audit records",
		"API Activity",
		"tokens redacted",
		"key === \"signed_token\"",
		"key === \"qr_payload\"",
		"contentType.includes(\"application/json\")",
		"await response.text()",
	}
	for _, fragment := range required {
		if !strings.Contains(content, fragment) {
			t.Fatalf("React UI contract is missing %q", fragment)
		}
	}
	forbidden := []string{
		"\"X-Actor-ID\"",
		"\"X-Role\"",
	}
	for _, fragment := range forbidden {
		if strings.Contains(content, fragment) {
			t.Fatalf("React UI contract still exposes legacy header %q", fragment)
		}
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

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
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

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
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

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"database is not ready"`)
}

func assertEnvelope(t *testing.T, body string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(body, part) {
			t.Fatalf("response body %q does not contain %q", body, part)
		}
	}
}
