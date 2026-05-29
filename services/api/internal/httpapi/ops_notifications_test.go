package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsNotificationDeliveriesPassesHRAdminActor(t *testing.T) {
	service := &fakeTicketingService{}
	router := testRouter(Dependencies{Ticketing: service, OpsAPIEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/notification-deliveries", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, ticketing.RoleHRAdmin, service.notificationOpsActor.Role)
	assert.NotEmpty(t, service.notificationOpsActor.ID)
}
