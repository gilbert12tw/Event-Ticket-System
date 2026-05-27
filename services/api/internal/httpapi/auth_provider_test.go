package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderBearerMeReturnsCompleteClaims(t *testing.T) {
	secret := providerTestSecret()
	router := testRouter(Dependencies{AppEnv: "production"})
	token := signProviderClaims(t, secret, validProviderClaims(ticketing.RoleEmployee))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(),
		`"employee_id":"E1001"`,
		`"display_name":"Ariel Chen"`,
		`"department":"Engineering"`,
		`"site":"Taipei HQ"`,
		`"city":"Taipei"`,
		`"grade":6`,
		`"employment_status":"active"`,
		`"claims_status":"complete"`,
	)
}

func TestBearerTokenRejectsNonBearerAuthorizationScheme(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	token, ok := bearerToken(req)

	assert.False(t, ok, "ok = true, token = %q; want non-bearer authorization to be treated as missing bearer", token)
}

func TestProtectedAPIsRequireProviderBearerInProduction(t *testing.T) {
	service := &fakeTicketingService{}
	secret := providerTestSecret()
	router := testRouter(Dependencies{Ticketing: service, AppEnv: "production"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	req.Header.Set("X-Actor-ID", "admin-1")
	req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected header-only production request to be rejected")

	cookieReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	cookieReq.AddCookie(&http.Cookie{Name: "cets_session", Value: "legacy.session"})
	cookieRec := httptest.NewRecorder()
	router.ServeHTTP(cookieRec, cookieReq)
	assert.Equal(t, http.StatusUnauthorized, cookieRec.Code, "expected local session cookie to be rejected in production")

	okReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	okReq.Header.Set("Authorization", "Bearer "+signProviderClaims(t, secret, validProviderClaims(ticketing.RoleActivityAdmin)))
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, okReq)
	require.Equal(t, http.StatusCreated, okRec.Code, okRec.Body.String())
	assert.Equal(t, "admin-1", service.createActor.ID)
	assert.Equal(t, ticketing.RoleActivityAdmin, service.createActor.Role)
}

func TestProviderBearerMapsAllRolesToProtectedActors(t *testing.T) {
	secret := providerTestSecret()
	tests := []struct {
		role   string
		method string
		path   string
		body   string
		assert func(t *testing.T, service *fakeTicketingService)
	}{
		{
			role:   ticketing.RoleEmployee,
			method: http.MethodGet,
			path:   "/api/v1/events",
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				assert.Equal(t, "E1001", service.listEventsActor.ID)
				assert.Equal(t, ticketing.RoleEmployee, service.listEventsActor.Role)
			},
		},
		{
			role:   ticketing.RoleActivityAdmin,
			method: http.MethodPost,
			path:   "/api/v1/admin/events",
			body:   eventRequestBody(),
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				assert.Equal(t, "admin-1", service.createActor.ID)
				assert.Equal(t, ticketing.RoleActivityAdmin, service.createActor.Role)
			},
		},
		{
			role:   ticketing.RoleCheckinStaff,
			method: http.MethodPost,
			path:   "/api/v1/checkins",
			body:   `{"signed_token":"ticket-token","device_id":"gate-1"}`,
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				assert.Equal(t, "staff-1", service.checkinActor.ID)
				assert.Equal(t, ticketing.RoleCheckinStaff, service.checkinActor.Role)
			},
		},
		{
			role:   ticketing.RoleHRAdmin,
			method: http.MethodGet,
			path:   "/api/v1/admin/reports",
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				assert.Equal(t, "hr-1", service.reportsActor.ID)
				assert.Equal(t, ticketing.RoleHRAdmin, service.reportsActor.Role)
			},
		},
		{
			role:   ticketing.RoleSystemAdmin,
			method: http.MethodGet,
			path:   "/api/v1/admin/audit-logs",
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				assert.Equal(t, "system-1", service.auditActor.ID)
				assert.Equal(t, ticketing.RoleSystemAdmin, service.auditActor.Role)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			service := &fakeTicketingService{}
			router := testRouter(Dependencies{Ticketing: service, AppEnv: "production"})
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Authorization", "Bearer "+signProviderClaims(t, secret, validProviderClaims(tt.role)))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.True(t, rec.Code >= http.StatusOK && rec.Code < http.StatusMultipleChoices, "status = %d, body = %s", rec.Code, rec.Body.String())
			tt.assert(t, service)
		})
	}
}

func TestProviderBearerRejectsMalformedClaimsBeforeMutatingEndpoint(t *testing.T) {
	service := &fakeTicketingService{}
	secret := providerTestSecret()
	router := testRouter(Dependencies{Ticketing: service, AppEnv: "production"})
	token := signProviderClaims(t, secret, func() providerClaims {
		claims := validProviderClaims(ticketing.RoleActivityAdmin)
		claims.Department = ""
		return claims
	}())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	assert.Empty(t, service.createActor.ID, "malformed claims reached mutating service")
}

func TestProviderBearerRejectsInvalidClaims(t *testing.T) {
	secret := providerTestSecret()
	router := testRouter(Dependencies{AppEnv: "production"})

	tests := []struct {
		name   string
		token  string
		status int
	}{
		{
			name:   "tampered",
			token:  signProviderClaims(t, secret, validProviderClaims(ticketing.RoleEmployee)) + "x",
			status: http.StatusUnauthorized,
		},
		{
			name: "missing required claim",
			token: signProviderClaims(t, secret, func() providerClaims {
				claims := validProviderClaims(ticketing.RoleEmployee)
				claims.EmploymentStatus = ""
				return claims
			}()),
			status: http.StatusUnauthorized,
		},
		{
			name: "expired",
			token: signProviderClaims(t, secret, func() providerClaims {
				claims := validProviderClaims(ticketing.RoleEmployee)
				claims.ExpiresAt = time.Now().Add(-time.Minute).Unix()
				return claims
			}()),
			status: http.StatusUnauthorized,
		},
		{
			name: "unmapped role",
			token: signProviderClaims(t, secret, func() providerClaims {
				claims := validProviderClaims("payroll_admin")
				return claims
			}()),
			status: http.StatusForbidden,
		},
		{
			name: "ambiguous roles",
			token: signProviderClaims(t, secret, func() providerClaims {
				claims := validProviderClaims(ticketing.RoleEmployee)
				claims.RoleClaims = []string{ticketing.RoleEmployee, ticketing.RoleHRAdmin}
				return claims
			}()),
			status: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tt.status, rec.Code, "body = %s", rec.Body.String())
		})
	}
}

func TestLocalSSOLoginAndLogoutRoutesAreRemoved(t *testing.T) {
	router := testTicketingRouter(&fakeTicketingService{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"principal_id":"admin-1"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.True(t, rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed, "status = %d", rec.Code)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)
	assert.True(t, logoutRec.Code == http.StatusNotFound || logoutRec.Code == http.StatusMethodNotAllowed, "logout status = %d", logoutRec.Code)
}

func TestAuthBootstrapReportsMockProfiles(t *testing.T) {
	tests := []struct {
		appEnv string
		want   []string
	}{
		{appEnv: "test", want: []string{`"mock_profiles_enabled":true`, `"debug_chrome_enabled":true`, `"profile_id":"E1001"`, `"profile_id":"admin-1"`, `"grade":6`, `"employment_status":"active"`}},
		{appEnv: "local", want: []string{`"mock_profiles_enabled":true`, `"debug_chrome_enabled":true`, `"profile_id":"E1001"`, `"grade":6`, `"employment_status":"active"`}},
		{appEnv: "demo", want: []string{`"mock_profiles_enabled":true`, `"debug_chrome_enabled":true`, `"profile_id":"E1001"`, `"grade":6`, `"employment_status":"active"`}},
		{appEnv: "production", want: []string{`"mock_profiles_enabled":false`, `"debug_chrome_enabled":false`, `"mock_profiles":[]`}},
	}
	for _, tt := range tests {
		t.Run(tt.appEnv, func(t *testing.T) {
			router := testRouter(Dependencies{AppEnv: tt.appEnv})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/bootstrap", nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assertEnvelope(t, rec.Body.String(), tt.want...)
		})
	}
}

func TestMockProviderTokenUsesProviderBearerPath(t *testing.T) {
	router := testRouter(Dependencies{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"staff-1"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(), `"provider_token"`, `"employee_id":"staff-1"`, `"claims_status":"complete"`)

	productionReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"staff-1"}`))
	productionRec := httptest.NewRecorder()
	testRouter(Dependencies{AppEnv: "production"}).ServeHTTP(productionRec, productionReq)
	assert.Equal(t, http.StatusNotFound, productionRec.Code)
}

func eventRequestBody() string {
	return `{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`
}

func providerTestSecret() string {
	return "provider-test-secret-0123456789abcdef"
}

func validProviderClaims(role string) providerClaims {
	return providerClaims{
		EmployeeID:       providerEmployeeIDForRole(role),
		DisplayName:      providerDisplayNameForRole(role),
		RoleClaims:       []string{role},
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            6,
		EmploymentStatus: "active",
		ExpiresAt:        time.Now().Add(time.Hour).Unix(),
	}
}

func providerEmployeeIDForRole(role string) string {
	switch role {
	case ticketing.RoleActivityAdmin:
		return "admin-1"
	case ticketing.RoleCheckinStaff:
		return "staff-1"
	case ticketing.RoleHRAdmin:
		return "hr-1"
	case ticketing.RoleSystemAdmin:
		return "system-1"
	default:
		return "E1001"
	}
}

func providerDisplayNameForRole(role string) string {
	switch role {
	case ticketing.RoleActivityAdmin:
		return "Admin One"
	case ticketing.RoleCheckinStaff:
		return "Staff One"
	case ticketing.RoleHRAdmin:
		return "HR One"
	case ticketing.RoleSystemAdmin:
		return "System One"
	default:
		return "Ariel Chen"
	}
}

func signProviderClaims(t *testing.T, secret string, claims providerClaims) string {
	t.Helper()
	token, err := NewProviderVerifier(ProviderAuthConfig{Secret: secret}).Sign(claims)
	require.NoError(t, err)
	return token
}
