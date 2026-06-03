package httpapi

import (
	"bytes"
	"context"
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

func TestRouterUsesReadServiceForReadRoutes(t *testing.T) {
	writeService := &fakeTicketingService{}
	readService := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: writeService, ReadTicketing: readService})

	readReq := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	authorizeRequest(t, readReq, ticketing.RoleEmployee)
	readRec := httptest.NewRecorder()
	router.ServeHTTP(readRec, readReq)

	require.Equal(t, http.StatusOK, readRec.Code, readRec.Body.String())
	assert.Equal(t, "E1001", readService.listEventsActor.ID)
	assert.Empty(t, writeService.listEventsActor.ID)

	writeReq := httptest.NewRequest(http.MethodPost, "/api/v1/events/evt_1/bookings", bytes.NewBufferString(`{"idempotency_key":"idem-1"}`))
	authorizeRequest(t, writeReq, ticketing.RoleEmployee)
	writeRec := httptest.NewRecorder()
	router.ServeHTTP(writeRec, writeReq)

	require.Equal(t, http.StatusCreated, writeRec.Code, writeRec.Body.String())
	assert.True(t, writeService.bookCalled)
	assert.False(t, readService.bookCalled)

	packageReq := httptest.NewRequest(http.MethodGet, "/api/v1/checkins/events/evt_1/offline-package?device_id=gate-1", nil)
	authorizeRequest(t, packageReq, ticketing.RoleCheckinStaff)
	packageRec := httptest.NewRecorder()
	router.ServeHTTP(packageRec, packageReq)

	require.Equal(t, http.StatusOK, packageRec.Code, packageRec.Body.String())
	assert.Equal(t, "staff-1", writeService.offlinePackageActor.ID)
	assert.Empty(t, readService.offlinePackageActor.ID)
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
		"allocation_mode":"fcfs",
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
	assert.Equal(t, ticketing.AllocationModeFCFS, service.createRequest.AllocationMode)
	assert.Equal(t, "Taipei", service.createRequest.EventCity)
	assert.Equal(t, "HQ", service.createRequest.EventSite)
	assert.Equal(t, "Engineering", service.createRequest.Rule.Department)

	patch := bytes.NewBufferString(`{"registration_opens_at":"2026-05-02T10:00:00Z","registration_closes_at":"2026-05-21T10:00:00Z","capacity_type":"limited","capacity":25,"allows_family":false,"allocation_mode":"lottery"}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/events/evt_1", patch)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotNil(t, service.updateRequest.CapacityType)
	assert.Equal(t, ticketing.CapacityTypeLimited, *service.updateRequest.CapacityType)
	require.NotNil(t, service.updateRequest.Capacity)
	assert.Equal(t, 25, *service.updateRequest.Capacity)
	require.NotNil(t, service.updateRequest.AllocationMode)
	assert.Equal(t, ticketing.AllocationModeLottery, *service.updateRequest.AllocationMode)
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

func TestBookHandlerReturnsRateLimitRetryAfter(t *testing.T) {
	service := &fakeTicketingService{
		bookErr: ticketing.AppError{
			Status:            http.StatusTooManyRequests,
			Code:              "BOOKING_RATE_LIMITED",
			Message:           "booking rate limit exceeded; retry shortly",
			RetryAfterSeconds: 1,
		},
	}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/evt_1/bookings", bytes.NewBufferString(`{"idempotency_key":"rate-1"}`))
	authorizeRequest(t, req, ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"error_code":"BOOKING_RATE_LIMITED"`)
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

func TestSeedDemoHandlerRequiresActorAndAllowedRole(t *testing.T) {
	service := &fakeTicketingService{}
	cases := []struct {
		name    string
		actorID string
		role    string
		want    int
	}{
		{name: "missing actor", want: http.StatusUnauthorized},
		{name: "employee forbidden", actorID: "E1001", role: ticketing.RoleEmployee, want: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/seed-demo", bytes.NewBufferString(`{}`))
			if tc.actorID != "" || tc.role != "" {
				identity := authIdentity{Actor: ticketing.Actor{ID: tc.actorID, Role: tc.role}}
				req = req.WithContext(context.WithValue(req.Context(), authContextKey{}, identity))
			}
			rec := httptest.NewRecorder()

			handleSeedDemo(service, "local").ServeHTTP(rec, req)

			assert.Equal(t, tc.want, rec.Code)
		})
	}
}
