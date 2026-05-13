package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"
)

func TestProviderBearerMeReturnsCompleteClaims(t *testing.T) {
	secret := providerTestSecret()
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
		ProviderAuth:   ProviderAuthConfig{Secret: secret},
	})
	token := signProviderClaims(t, secret, validProviderClaims(ticketing.RoleEmployee))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertEnvelope(t, rec.Body.String(),
		`"employee_id":"E1001"`,
		`"display_name":"Ariel Chen"`,
		`"department":"Engineering"`,
		`"site":"Taipei HQ"`,
		`"city":"Taipei"`,
		`"claims_status":"complete"`,
	)
}

func TestBearerTokenRejectsNonBearerAuthorizationScheme(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	token, ok := bearerToken(req)

	if ok {
		t.Fatalf("ok = true, token = %q; want non-bearer authorization to be treated as missing bearer", token)
	}
}

func TestProtectedAPIsRequireProviderBearerInProduction(t *testing.T) {
	service := &fakeTicketingService{}
	secret := providerTestSecret()
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
		ProviderAuth:   ProviderAuthConfig{Secret: secret},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	req.Header.Set("X-Actor-ID", "admin-1")
	req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected header-only production request to be rejected, status = %d", rec.Code)
	}

	cookieReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	cookieReq.AddCookie(&http.Cookie{Name: "cets_session", Value: "legacy.session"})
	cookieRec := httptest.NewRecorder()
	router.ServeHTTP(cookieRec, cookieReq)
	if cookieRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected local session cookie to be rejected in production, status = %d", cookieRec.Code)
	}

	okReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	okReq.Header.Set("Authorization", "Bearer "+signProviderClaims(t, secret, validProviderClaims(ticketing.RoleActivityAdmin)))
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", okRec.Code, okRec.Body.String())
	}
	if service.createActor.ID != "admin-1" || service.createActor.Role != ticketing.RoleActivityAdmin {
		t.Fatalf("actor = %+v", service.createActor)
	}
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
				if service.listEventsActor.ID != "E1001" || service.listEventsActor.Role != ticketing.RoleEmployee {
					t.Fatalf("employee actor = %+v", service.listEventsActor)
				}
			},
		},
		{
			role:   ticketing.RoleActivityAdmin,
			method: http.MethodPost,
			path:   "/api/v1/admin/events",
			body:   eventRequestBody(),
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				if service.createActor.ID != "admin-1" || service.createActor.Role != ticketing.RoleActivityAdmin {
					t.Fatalf("activity admin actor = %+v", service.createActor)
				}
			},
		},
		{
			role:   ticketing.RoleCheckinStaff,
			method: http.MethodPost,
			path:   "/api/v1/checkins",
			body:   `{"signed_token":"ticket-token","device_id":"gate-1"}`,
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				if service.checkinActor.ID != "staff-1" || service.checkinActor.Role != ticketing.RoleCheckinStaff {
					t.Fatalf("check-in actor = %+v", service.checkinActor)
				}
			},
		},
		{
			role:   ticketing.RoleHRAdmin,
			method: http.MethodGet,
			path:   "/api/v1/admin/reports",
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				if service.reportsActor.ID != "hr-1" || service.reportsActor.Role != ticketing.RoleHRAdmin {
					t.Fatalf("hr actor = %+v", service.reportsActor)
				}
			},
		},
		{
			role:   ticketing.RoleSystemAdmin,
			method: http.MethodGet,
			path:   "/api/v1/admin/audit-logs",
			assert: func(t *testing.T, service *fakeTicketingService) {
				t.Helper()
				if service.auditActor.ID != "system-1" || service.auditActor.Role != ticketing.RoleSystemAdmin {
					t.Fatalf("system actor = %+v", service.auditActor)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			service := &fakeTicketingService{}
			router := NewRouter(Dependencies{
				DB:             fakePinger{},
				Ticketing:      service,
				Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
				RequestTimeout: time.Second,
				AppEnv:         "production",
				ProviderAuth:   ProviderAuthConfig{Secret: secret},
			})
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Authorization", "Bearer "+signProviderClaims(t, secret, validProviderClaims(tt.role)))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code < http.StatusOK || rec.Code >= http.StatusMultipleChoices {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			tt.assert(t, service)
		})
	}
}

func TestProviderBearerRejectsMalformedClaimsBeforeMutatingEndpoint(t *testing.T) {
	service := &fakeTicketingService{}
	secret := providerTestSecret()
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
		ProviderAuth:   ProviderAuthConfig{Secret: secret},
	})
	token := signProviderClaims(t, secret, func() providerClaims {
		claims := validProviderClaims(ticketing.RoleActivityAdmin)
		claims.Department = ""
		return claims
	}())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(eventRequestBody()))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.createActor.ID != "" {
		t.Fatalf("malformed claims reached mutating service with actor %+v", service.createActor)
	}
}

func TestProviderBearerRejectsInvalidClaims(t *testing.T) {
	secret := providerTestSecret()
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
		ProviderAuth:   ProviderAuthConfig{Secret: secret},
	})

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
				claims.Department = ""
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

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

func TestLocalSSOLoginAndLogoutRoutesAreRemoved(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      &fakeTicketingService{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"principal_id":"admin-1"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusNotFound && logoutRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("logout status = %d", logoutRec.Code)
	}
}

func TestAuthBootstrapReportsMockProfiles(t *testing.T) {
	tests := []struct {
		appEnv string
		want   []string
	}{
		{appEnv: "test", want: []string{`"mock_profiles_enabled":true`, `"profile_id":"E1001"`, `"profile_id":"admin-1"`}},
		{appEnv: "production", want: []string{`"mock_profiles_enabled":false`, `"mock_profiles":[]`}},
	}
	for _, tt := range tests {
		t.Run(tt.appEnv, func(t *testing.T) {
			router := NewRouter(Dependencies{
				DB:             fakePinger{},
				Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
				RequestTimeout: time.Second,
				AppEnv:         tt.appEnv,
			})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/bootstrap", nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			assertEnvelope(t, rec.Body.String(), tt.want...)
		})
	}
}

func TestMockProviderTokenUsesProviderBearerPath(t *testing.T) {
	secret := providerTestSecret()
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: secret},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"staff-1"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertEnvelope(t, rec.Body.String(), `"provider_token"`, `"employee_id":"staff-1"`, `"claims_status":"complete"`)

	productionReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"staff-1"}`))
	productionRec := httptest.NewRecorder()
	NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
		ProviderAuth:   ProviderAuthConfig{Secret: secret},
	}).ServeHTTP(productionRec, productionReq)
	if productionRec.Code != http.StatusNotFound {
		t.Fatalf("production mock token status = %d", productionRec.Code)
	}
}

func eventRequestBody() string {
	return `{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`
}

func providerTestSecret() string {
	return "provider-test-secret-0123456789abcdef"
}

func validProviderClaims(role string) providerClaims {
	return providerClaims{
		EmployeeID:  providerEmployeeIDForRole(role),
		DisplayName: providerDisplayNameForRole(role),
		RoleClaims:  []string{role},
		Department:  "Engineering",
		Site:        "Taipei HQ",
		City:        "Taipei",
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
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
	if err != nil {
		t.Fatal(err)
	}
	return token
}
