package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"
)

func TestAuditHandlerRejectsMalformedCursorAndTimeFilters(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	for _, path := range []string{
		"/api/v1/admin/audit-logs?cursor=not-a-cursor",
		"/api/v1/admin/audit-logs?cursor=2026-05-07T08:09:10Z",
		"/api/v1/admin/audit-logs?from=not-a-time",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		authorizeRequest(t, req, ticketing.RoleHRAdmin)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, body = %s", path, rec.Code, rec.Body.String())
		}
	}
}
