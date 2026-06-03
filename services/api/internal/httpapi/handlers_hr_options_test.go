package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminHROptionsHandlerReturnsSitesFromService(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/hr/options", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, ticketing.RoleActivityAdmin, service.hrOptionsActor.Role)
	assertEnvelope(t, rec.Body.String(), `"sites":[`, `"value":"Tainan HQ"`)
}
