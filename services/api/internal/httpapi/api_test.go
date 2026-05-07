package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"
)

func TestCreateEventHandlerPassesActorAndReturnsCreated(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	body := bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", body)
	req.Header.Set("X-Actor-ID", "admin-1")
	req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.createActor.ID != "admin-1" || service.createActor.Role != ticketing.RoleActivityAdmin {
		t.Fatalf("actor = %+v", service.createActor)
	}
}

func TestBookHandlerRejectsMalformedJSON(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/evt_1/bookings", bytes.NewBufferString(`{`))
	req.Header.Set("X-Actor-ID", "E1001")
	req.Header.Set("X-Role", ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWriteJSONEncodesNilSlicesAsEmptyArrays(t *testing.T) {
	var rows []string
	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusOK, rows)

	assertEnvelope(t, rec.Body.String(), `"success":true`, `"data":[]`)
}

func TestCheckinHandlerReturnsDuplicateDetailsOnConflict(t *testing.T) {
	service := &fakeTicketingService{checkinErr: ticketing.AppError{Status: http.StatusConflict, Message: "ticket has already been redeemed"}}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkins", bytes.NewBufferString(`{"signed_token":"token","device_id":"gate-1"}`))
	req.Header.Set("X-Actor-ID", "staff-1")
	req.Header.Set("X-Role", ticketing.RoleCheckinStaff)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"checkin_id":"chk_1"`, `"duplicate":true`)
}

func TestEventGovernanceHandlersExposeProductionRoutes(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	requests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/admin/events", "", http.StatusOK},
		{http.MethodGet, "/api/v1/events/evt_1?employee_id=E1001", "", http.StatusOK},
		{http.MethodPatch, "/api/v1/admin/events/evt_1", `{"title":"Updated"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/events/evt_1/state", `{"status":"closed","reason":"done"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/events/evt_1/duplicate", `{}`, http.StatusCreated},
		{http.MethodDelete, "/api/v1/admin/events/evt_1", "", http.StatusOK},
	}
	for _, tt := range requests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		req.Header.Set("X-Actor-ID", "admin-1")
		req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != tt.status {
			t.Fatalf("%s %s status = %d, body = %s", tt.method, tt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestRegistrationAndTicketGovernanceHandlersExposeProductionRoutes(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	requests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/admin/events/evt_1/registrations", "", http.StatusOK},
		{http.MethodPost, "/api/v1/events/evt_1/bookings/reg_1/cancel", `{"idempotency_key":"cancel-1","reason":"cannot attend"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/events/evt_1/waitlist/promote", `{}`, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/tickets/tkt_1/revoke", `{"reason":"admin review"}`, http.StatusOK},
	}
	for _, tt := range requests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		req.Header.Set("X-Actor-ID", "admin-1")
		req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != tt.status {
			t.Fatalf("%s %s status = %d, body = %s", tt.method, tt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestProductionBoundaryHandlersExposeSpecRoutes(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	requests := []struct {
		method string
		path   string
		body   string
		role   string
		status int
	}{
		{http.MethodPost, "/api/v1/admin/events/evt_1/eligibility/preview", `{"rule":{"department":"Engineering"}}`, ticketing.RoleActivityAdmin, http.StatusOK},
		{http.MethodPut, "/api/v1/admin/events/evt_1/eligibility", `{"rule":{"department":"Engineering"},"allow_zero_match":true}`, ticketing.RoleActivityAdmin, http.StatusOK},
		{http.MethodGet, "/api/v1/admin/eligibility-impact-reviews", "", ticketing.RoleHRAdmin, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/eligibility-impact-reviews/rev_1/resolve", `{"reason":"reviewed"}`, ticketing.RoleHRAdmin, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/events/evt_1/lottery-runs", `{"seed":"seed-1"}`, ticketing.RoleActivityAdmin, http.StatusCreated},
		{http.MethodGet, "/api/v1/tickets/tkt_1", "", ticketing.RoleEmployee, http.StatusOK},
		{http.MethodGet, "/api/v1/checkins/events/evt_1/offline-package?device_id=gate-1", "", ticketing.RoleCheckinStaff, http.StatusOK},
		{http.MethodPost, "/api/v1/checkins/offline-sync", `{"batch_id":"off_1","event_id":"evt_1","device_id":"gate-1","package_signature":"sig_1","scans":[]}`, ticketing.RoleCheckinStaff, http.StatusOK},
		{http.MethodGet, "/api/v1/notifications/preferences", "", ticketing.RoleEmployee, http.StatusOK},
		{http.MethodPut, "/api/v1/notifications/preferences", `{"email_enabled":true,"in_app_enabled":true}`, ticketing.RoleEmployee, http.StatusOK},
		{http.MethodGet, "/api/v1/admin/notifications/deliveries", "", ticketing.RoleHRAdmin, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/notifications/deliveries/del_1/retry", `{}`, ticketing.RoleHRAdmin, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/reports/exports", `{"report_type":"participation"}`, ticketing.RoleHRAdmin, http.StatusCreated},
		{http.MethodGet, "/api/v1/admin/reports/exports/exp_1", "", ticketing.RoleHRAdmin, http.StatusOK},
	}
	for _, tt := range requests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		req.Header.Set("X-Actor-ID", actorForRole(tt.role))
		req.Header.Set("X-Role", tt.role)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != tt.status {
			t.Fatalf("%s %s status = %d, body = %s", tt.method, tt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestAuditHandlerParsesServerSideFilterQuery(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs?action=event.updated&entity_type=event&limit=25", nil)
	req.Header.Set("X-Actor-ID", "hr-1")
	req.Header.Set("X-Role", ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(service.auditQuery) != 1 {
		t.Fatalf("expected audit query to be passed, got %#v", service.auditQuery)
	}
	if service.auditQuery[0].Action != "event.updated" || service.auditQuery[0].EntityType != "event" || service.auditQuery[0].Limit != 25 {
		t.Fatalf("audit query = %#v", service.auditQuery[0])
	}
}

func actorForRole(role string) string {
	switch role {
	case ticketing.RoleEmployee:
		return "E1001"
	case ticketing.RoleCheckinStaff:
		return "staff-1"
	case ticketing.RoleHRAdmin:
		return "hr-1"
	default:
		return "admin-1"
	}
}

func TestSeedDemoHandlerIsHiddenOutsideLocalEnvironments(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/seed-demo", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Actor-ID", "admin-1")
	req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestLoginSetsHttpOnlySameSiteCookieAndMeReadsSession(t *testing.T) {
	var logs bytes.Buffer
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewJSONHandler(&logs, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		AuthSession: AuthConfig{
			Secret: "auth-test-secret",
			TTL:    time.Hour,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"principal_id":"E1001"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cookie := findCookie(rec.Result().Cookies(), sessionCookieName)
	if cookie == nil {
		t.Fatal("expected session cookie")
	}
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("same site = %v", cookie.SameSite)
	}
	assertEnvelope(t, rec.Body.String(), `"actor":{"id":"E1001","role":"employee"}`, `"expires_at"`)
	if strings.Contains(logs.String(), `"actor_id":"E1001"`) || strings.Contains(logs.String(), `"E1001"`) {
		t.Fatalf("auth log leaked raw actor id: %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"actor_ref"`) {
		t.Fatalf("auth log missing redacted actor ref: %s", logs.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.AddCookie(cookie)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", meRec.Code, meRec.Body.String())
	}
	assertEnvelope(t, meRec.Body.String(), `"actor":{"id":"E1001","role":"employee"}`)
}

func TestLoginRejectsInvalidPrincipal(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"principal_id":"unknown"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if findCookie(rec.Result().Cookies(), sessionCookieName) != nil {
		t.Fatal("invalid login must not set session cookie")
	}
}

func TestMeRequiresValidSessionAndLogoutClearsCookie(t *testing.T) {
	auth := AuthConfig{Secret: "auth-test-secret", TTL: time.Hour}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		AuthSession:    auth,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"principal_id":"admin-1"}`))
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	cookie := findCookie(loginRec.Result().Cookies(), sessionCookieName)
	if cookie == nil {
		t.Fatal("expected login cookie")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", logoutRec.Code)
	}
	cleared := findCookie(logoutRec.Result().Cookies(), sessionCookieName)
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("expected clearing cookie, got %+v", cleared)
	}
}

func TestProtectedAPIsRequireSessionAndResolveCookieActor(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
		AuthSession: AuthConfig{
			Secret:       "auth-test-secret",
			TTL:          time.Hour,
			CookieSecure: true,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`))
	req.Header.Set("X-Actor-ID", "admin-1")
	req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected header-only production request to be rejected, status = %d", rec.Code)
	}

	auth := NewSessionManager(AuthConfig{
		Secret:       "auth-test-secret",
		TTL:          time.Hour,
		CookieSecure: true,
		AppEnv:       "production",
	})
	token, _, err := auth.Sign(ticketing.Actor{ID: "admin-1", Role: ticketing.RoleActivityAdmin})
	if err != nil {
		t.Fatal(err)
	}

	okReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`))
	okReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", okRec.Code, okRec.Body.String())
	}
	if service.createActor.ID != "admin-1" || service.createActor.Role != ticketing.RoleActivityAdmin {
		t.Fatalf("actor = %+v", service.createActor)
	}
}

func TestLocalSSOLoginIsRejectedInProduction(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      &fakeTicketingService{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "production",
		AuthSession: AuthConfig{
			Secret:       "auth-test-secret",
			TTL:          time.Hour,
			CookieSecure: true,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"principal_id":"admin-1"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if findCookie(rec.Result().Cookies(), sessionCookieName) != nil {
		t.Fatal("production local SSO login must not set a session cookie")
	}
}

func TestProtectedAPIRejectsTamperedSession(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      &fakeTicketingService{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		AuthSession: AuthConfig{
			Secret: "auth-test-secret",
			TTL:    time.Hour,
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "tampered.session"})
	req.Header.Set("X-Actor-ID", "E1001")
	req.Header.Set("X-Role", ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "evt_1") {
		t.Fatal("tampered session should not fall back to legacy headers")
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
