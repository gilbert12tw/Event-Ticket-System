package httpapi

import (
	"bytes"
	"encoding/json"
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
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	body := bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", body)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.createActor.ID != "admin-1" || service.createActor.Role != ticketing.RoleActivityAdmin {
		t.Fatalf("actor = %+v", service.createActor)
	}
}

func TestEventHandlersDecodeOpenAPIEventFields(t *testing.T) {
	service := &fakeTicketingService{}
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	body := bytes.NewBufferString(`{
		"title":"OpenAPI Event",
		"description":"Demo",
		"starts_at":"2026-06-01T10:00:00Z",
		"registration_opens_at":"2026-05-01T10:00:00Z",
		"registration_closes_at":"2026-05-20T10:00:00Z",
		"event_city":"Taipei",
		"event_site":"HQ",
		"capacity_type":"unlimited",
		"capacity":null,
		"allows_family":true,
		"eligibility_rule":{"department":"Engineering"}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", body)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.createRequest.CapacityType != ticketing.CapacityTypeUnlimited || service.createRequest.Capacity != 0 || !service.createRequest.AllowsFamily {
		t.Fatalf("create request capacity fields = %+v", service.createRequest)
	}
	if service.createRequest.EventCity != "Taipei" || service.createRequest.EventSite != "HQ" || service.createRequest.Rule.Department != "Engineering" {
		t.Fatalf("create request OpenAPI fields = %+v", service.createRequest)
	}

	patch := bytes.NewBufferString(`{"registration_opens_at":"2026-05-02T10:00:00Z","registration_closes_at":"2026-05-21T10:00:00Z","capacity_type":"limited","capacity":25,"allows_family":false}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/events/evt_1", patch)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.updateRequest.CapacityType == nil || *service.updateRequest.CapacityType != ticketing.CapacityTypeLimited {
		t.Fatalf("update capacity type = %+v", service.updateRequest.CapacityType)
	}
	if service.updateRequest.Capacity == nil || *service.updateRequest.Capacity != 25 || service.updateRequest.RegistrationStart == nil || service.updateRequest.RegistrationClose == nil {
		t.Fatalf("update OpenAPI fields = %+v", service.updateRequest)
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
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/evt_1/bookings", bytes.NewBufferString(`{`))
	authorizeRequest(t, req, ticketing.RoleEmployee)
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
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkins", bytes.NewBufferString(`{"signed_token":"token","device_id":"gate-1"}`))
	authorizeRequest(t, req, ticketing.RoleCheckinStaff)
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
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	requests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/admin/events", "", http.StatusOK},
		{http.MethodGet, "/api/v1/events/evt_1", "", http.StatusOK},
		{http.MethodPatch, "/api/v1/admin/events/evt_1", `{"title":"Updated"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/events/evt_1/state", `{"status":"closed","reason":"done"}`, http.StatusOK},
		{http.MethodPost, "/api/v1/admin/events/evt_1/duplicate", `{}`, http.StatusCreated},
		{http.MethodDelete, "/api/v1/admin/events/evt_1", "", http.StatusOK},
	}
	for _, tt := range requests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		authorizeRequest(t, req, ticketing.RoleActivityAdmin)
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
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
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
		authorizeRequest(t, req, ticketing.RoleActivityAdmin)
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
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
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
		{http.MethodGet, "/api/v1/me/tickets", "", ticketing.RoleEmployee, http.StatusOK},
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
		authorizeRequest(t, req, tt.role)
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
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs?action=event.updated&entity_type=event&limit=25", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
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

func authorizeRequest(t *testing.T, req *http.Request, role string) {
	t.Helper()
	req.Header.Set("Authorization", "Bearer "+signProviderClaims(t, providerTestSecret(), validProviderClaims(role)))
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

func TestMockProviderTokenIssuesBearerAndMeReadsClaims(t *testing.T) {
	var logs bytes.Buffer
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewJSONHandler(&logs, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"E1001"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data mockProviderTokenResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ProviderToken == "" {
		t.Fatal("expected provider token")
	}
	assertEnvelope(t, rec.Body.String(), `"employee_id":"E1001"`, `"claims_status":"complete"`, `"provider_token"`)
	if strings.Contains(logs.String(), `"actor_id":"E1001"`) || strings.Contains(logs.String(), `"E1001"`) {
		t.Fatalf("mock auth log leaked raw actor id: %s", logs.String())
	}
	if !strings.Contains(logs.String(), `"actor_ref"`) {
		t.Fatalf("mock auth log missing redacted actor ref: %s", logs.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+envelope.Data.ProviderToken)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", meRec.Code, meRec.Body.String())
	}
	assertEnvelope(t, meRec.Body.String(), `"employee_id":"E1001"`, `"mapped_roles":["employee"]`, `"claims_status":"complete"`)
}

func TestMockProviderTokenRejectsUnknownProfile(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", bytes.NewBufferString(`{"profile_id":"unknown"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "provider_token") {
		t.Fatal("invalid mock profile must not return a provider token")
	}
}

func TestMeRequiresProviderBearerAndLogoutRouteIsRemoved(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusNotFound && logoutRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("logout status = %d", logoutRec.Code)
	}
}

func TestLocalSSORoutesAreRemoved(t *testing.T) {
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
}

func TestProtectedAPIRejectsTamperedBearerWithoutLegacyFallback(t *testing.T) {
	router := NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      &fakeTicketingService{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer tampered.provider")
	req.Header.Set("X-Actor-ID", "E1001")
	req.Header.Set("X-Role", ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "evt_1") {
		t.Fatal("tampered bearer should not fall back to legacy headers")
	}
}
