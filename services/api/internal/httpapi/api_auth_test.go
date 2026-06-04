package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProviderTokenIssuesBearerAndMeReadsClaims(t *testing.T) {
	var logs bytes.Buffer
	router := testRouter(Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"E1001"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var envelope struct {
		Data mockProviderTokenResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.NotEmpty(t, envelope.Data.ProviderToken)
	assertEnvelope(t, rec.Body.String(), `"employee_id":"E1001"`, `"claims_status":"complete"`, `"provider_token"`)
	assert.NotContains(t, logs.String(), `"actor_id":"E1001"`, "mock auth log leaked raw actor id")
	assert.NotContains(t, logs.String(), `"E1001"`, "mock auth log leaked raw actor id")
	assert.Contains(t, logs.String(), `"actor_ref"`, "mock auth log missing redacted actor ref")

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+envelope.Data.ProviderToken)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	require.Equal(t, http.StatusOK, meRec.Code, meRec.Body.String())
	assertEnvelope(t, meRec.Body.String(), `"employee_id":"E1001"`, `"mapped_roles":["employee"]`, `"claims_status":"complete"`)
}

func TestMockProviderTokenRejectsUnknownProfile(t *testing.T) {
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"unknown"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.NotContains(t, rec.Body.String(), "provider_token", "invalid mock profile must not return a provider token")
}

func TestMeRequiresProviderBearerAndLogoutRouteIsRemoved(t *testing.T) {
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)

	assert.True(t, logoutRec.Code == http.StatusNotFound || logoutRec.Code == http.StatusMethodNotAllowed, "logout status = %d", logoutRec.Code)
}

func TestLocalSSORoutesAreRemoved(t *testing.T) {
	router := testTicketingRouter(&fakeTicketingService{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"principal_id":"admin-1"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.True(t, rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed, "status = %d", rec.Code)
}

func TestProtectedAPIRejectsTamperedBearerWithoutLegacyFallback(t *testing.T) {
	router := testTicketingRouter(&fakeTicketingService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer tampered.provider")
	req.Header.Set("X-Actor-ID", "E1001")
	req.Header.Set("X-Role", ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.NotContains(t, rec.Body.String(), "evt_1", "tampered bearer should not fall back to legacy headers")
}
