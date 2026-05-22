package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEligibilityImpactReviewsResponseUsesRedactedEmployeeRef(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/eligibility-impact-reviews", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(), `"employee_ref":"E100****"`)
	assert.NotContains(t, rec.Body.String(), `"employee_id"`)
	assert.NotContains(t, rec.Body.String(), `"E1001"`)
}
