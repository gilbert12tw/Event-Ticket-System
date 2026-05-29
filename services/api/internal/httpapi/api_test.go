package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEventHandlerPassesActorAndReturnsCreated(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	body := bytes.NewBufferString(`{"title":"Demo","capacity":10,"status":"published","rule":{"department":"Engineering"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/events", body)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "admin-1", service.createActor.ID)
	assert.Equal(t, ticketing.RoleActivityAdmin, service.createActor.Role)
}

func TestEventHandlersDecodeOpenAPIEventFields(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
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

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, ticketing.CapacityTypeUnlimited, service.createRequest.CapacityType)
	assert.Equal(t, 0, service.createRequest.Capacity)
	assert.True(t, service.createRequest.AllowsFamily)
	assert.Equal(t, "Taipei", service.createRequest.EventCity)
	assert.Equal(t, "HQ", service.createRequest.EventSite)
	assert.Equal(t, "Engineering", service.createRequest.Rule.Department)

	patch := bytes.NewBufferString(`{"registration_opens_at":"2026-05-02T10:00:00Z","registration_closes_at":"2026-05-21T10:00:00Z","capacity_type":"limited","capacity":25,"allows_family":false}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/events/evt_1", patch)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, service.updateRequest.CapacityType)
	assert.Equal(t, ticketing.CapacityTypeLimited, *service.updateRequest.CapacityType)
	require.NotNil(t, service.updateRequest.Capacity)
	assert.Equal(t, 25, *service.updateRequest.Capacity)
	assert.NotNil(t, service.updateRequest.RegistrationStart)
	assert.NotNil(t, service.updateRequest.RegistrationClose)
}

func TestBookHandlerRejectsMalformedJSON(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/evt_1/bookings", bytes.NewBufferString(`{`))
	authorizeRequest(t, req, ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWriteJSONEncodesNilSlicesAsEmptyArrays(t *testing.T) {
	var rows []string
	rec := httptest.NewRecorder()

	writeJSON(rec, http.StatusOK, rows)

	assertEnvelope(t, rec.Body.String(), `"success":true`, `"data":[]`)
}

func TestCheckinHandlerReturnsDuplicateDetailsOnConflict(t *testing.T) {
	service := &fakeTicketingService{checkinErr: ticketing.AppError{Status: http.StatusConflict, Message: "ticket has already been redeemed"}}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkins", bytes.NewBufferString(`{"signed_token":"token","device_id":"gate-1"}`))
	authorizeRequest(t, req, ticketing.RoleCheckinStaff)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"checkin_id":"chk_1"`, `"duplicate":true`)
}

func TestCheckinHandlerRejectsScannedAtField(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	body := `{"signed_token":"token","event_id":"evt_1","device_id":"gate-1","scanned_at":"2026-05-06T10:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkins", bytes.NewBufferString(body))
	authorizeRequest(t, req, ticketing.RoleCheckinStaff)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, service.checkinActor.ID)
}

func TestOfflineCheckinSyncRejectsLocalScanIDField(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	body := `{"batch_id":"off_1","event_id":"evt_1","device_id":"gate-1","package_signature":"sig_1","scans":[{"signed_token":"token","scanned_at":"2026-05-06T10:00:00Z","local_scan_id":"scan-1"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkins/offline-sync", bytes.NewBufferString(body))
	authorizeRequest(t, req, ticketing.RoleCheckinStaff)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, service.offlineSyncActor.ID)
}

func TestNotificationPreferencesResponseIncludesEmployeeID(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/preferences", nil)
	authorizeRequest(t, req, ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(), `"employee_id":"E1001"`, `"email_enabled":true`, `"in_app_enabled":true`)
}

func TestEventGovernanceHandlersExposeProductionRoutes(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
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

		assert.Equal(t, tt.status, rec.Code, "%s %s body=%s", tt.method, tt.path, rec.Body.String())
	}
}

func TestRegistrationAndTicketGovernanceHandlersExposeProductionRoutes(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
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

		assert.Equal(t, tt.status, rec.Code, "%s %s body=%s", tt.method, tt.path, rec.Body.String())
	}
}

func TestProductionBoundaryHandlersExposeSpecRoutes(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
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

		assert.Equal(t, tt.status, rec.Code, "%s %s body=%s", tt.method, tt.path, rec.Body.String())
	}
}

func TestAuditHandlerParsesServerSideFilterQuery(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs?action=event.updated&entity_type=event&limit=25", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, service.auditQuery, 1)
	assert.Equal(t, "event.updated", service.auditQuery[0].Action)
	assert.Equal(t, "event", service.auditQuery[0].EntityType)
	assert.Equal(t, 25, service.auditQuery[0].Limit)
}

func TestNotificationDeliveriesResponseUsesRedactedEmployeeRef(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/notifications/deliveries", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(), `"employee_ref":"E100****"`)
	assert.NotContains(t, rec.Body.String(), `"employee_id"`)
	assert.NotContains(t, rec.Body.String(), `"E1001"`)
}

func TestOpsNotificationDeliveriesRouteRequiresFeatureFlag(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/notification-deliveries", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, service.notificationOpsQuery)
}

func TestOpsNotificationDeliveriesParsesQueryAndReturnsPage(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true})
	cursorTime := time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC)
	cursor := ticketing.EncodeNotificationDeliveryCursor(cursorTime, "del_cursor")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/notification-deliveries?status=dead_letter&limit=25&cursor="+cursor, nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, service.notificationOpsQuery, 1)
	query := service.notificationOpsQuery[0]
	assert.Equal(t, "dead_letter", query.Status)
	assert.Equal(t, 25, query.Limit)
	assert.Equal(t, cursorTime, query.Cursor)
	assert.Equal(t, "del_cursor", query.CursorID)
	assertEnvelope(t, rec.Body.String(), `"deliveries"`, `"recipient_redacted":"E100****"`, `"retry_eligible":true`, `"dead_letter_eligible":true`, `"next_cursor":"next-delivery-cursor"`)
	assert.NotContains(t, rec.Body.String(), `"employee_id"`)
	assert.NotContains(t, rec.Body.String(), "E1001")
}

func TestOpsNotificationDeliveriesRejectsMalformedCursor(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/notification-deliveries?cursor=2026-05-28T10:30:00Z", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, service.notificationOpsQuery)
}

func TestOpsNotificationDeliveriesRejectsInvalidLimit(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/notification-deliveries?limit=0", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, service.notificationOpsQuery)
}

func authorizeRequest(t *testing.T, req *http.Request, role string) {
	t.Helper()
	req.Header.Set("Authorization", "Bearer "+signProviderClaims(t, providerTestSecret(), validProviderClaims(role)))
}

func TestSeedDemoHandlerIsHiddenOutsideLocalEnvironments(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, AppEnv: "production"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/seed-demo", bytes.NewBufferString(`{}`))
	req.Header.Set("X-Actor-ID", "admin-1")
	req.Header.Set("X-Role", ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

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
