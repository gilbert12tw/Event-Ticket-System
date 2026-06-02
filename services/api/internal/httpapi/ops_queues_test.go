package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsQueuesRouteRequiresFeatureFlag(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/queues", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, service.queueStatusActor)
}

func TestOpsQueuesPassesActorAndReturnsStatus(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/queues", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, ticketing.RoleHRAdmin, service.queueStatusActor.Role)
	assert.NotEmpty(t, service.queueStatusActor.ID)
	assertEnvelope(t, rec.Body.String(), `"queues"`, `"name":"notification"`, `"pending":3`, `"in_flight":1`, `"dead_letter":2`, `"p95_age_seconds":42`)
	assertEnvelope(t, rec.Body.String(), `"meta"`, `"source":"operational"`)
}

func TestOpsCapacityPressureRouteRequiresFeatureFlag(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/capacity-pressure", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, service.capacityPressureActor)
}

func TestOpsCapacityPressurePassesActorAndReturnsSnapshot(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/capacity-pressure", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, ticketing.RoleActivityAdmin, service.capacityPressureActor.Role)
	assertEnvelope(t, rec.Body.String(), `"events"`, `"event_id":"evt_1"`, `"remaining_capacity":9`, `"reservation_count":4`)
	assertEnvelope(t, rec.Body.String(), `"source":"operational"`)
}

func TestOpsReportFreshnessPassesActorAndThreshold(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true, ReportStaleThresholdSeconds: 45})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/report-freshness", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, ticketing.RoleHRAdmin, service.reportFreshnessActor.Role)
	assert.Equal(t, 45, service.reportFreshnessLimit)
	assertEnvelope(t, rec.Body.String(), `"projections"`, `"name":"event_summary"`, `"lag_seconds":12`, `"degraded":false`)
	assertEnvelope(t, rec.Body.String(), `"source":"unavailable"`)
}

func TestOpsDashboardPassesActorAndThreshold(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true, ReportStaleThresholdSeconds: 30})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/dashboard", nil)
	authorizeRequest(t, req, ticketing.RoleSystemAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, ticketing.RoleSystemAdmin, service.opsDashboardActor.Role)
	assert.Equal(t, 30, service.opsDashboardLimit)
	assertEnvelope(t, rec.Body.String(), `"capacity_pressure"`, `"queues"`, `"reports_freshness"`)
}
