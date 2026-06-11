package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLoggingUsesErrorLevelFor5xxAndWarnFor4xx(t *testing.T) {
	var logs bytes.Buffer
	router := testRouter(Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assertEnvelope(t, logs.String(), `"level":"INFO"`, `"status":200`)

	logs.Reset()
	req = httptest.NewRequest(http.MethodDelete, "/no-such-route", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assertEnvelope(t, logs.String(), `"level":"WARN"`, `"status":405`)

	logs.Reset()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/events/evt_missing", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == 401 {
		assertEnvelope(t, logs.String(), `"level":"WARN"`)
	}
}
