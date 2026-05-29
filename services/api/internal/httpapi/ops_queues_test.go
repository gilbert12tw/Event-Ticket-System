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
}
