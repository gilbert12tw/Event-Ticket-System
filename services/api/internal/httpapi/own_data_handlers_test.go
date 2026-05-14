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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnDataHandlersRejectCallerSuppliedEmployeeID(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "list events query", method: http.MethodGet, path: "/api/v1/events?employee_id=E1002", status: http.StatusBadRequest},
		{name: "get event query", method: http.MethodGet, path: "/api/v1/events/evt_1?employee_id=E1002", status: http.StatusBadRequest},
		{name: "eligibility query", method: http.MethodGet, path: "/api/v1/events/evt_1/eligibility?employee_id=E1002", status: http.StatusBadRequest},
		{
			name:   "booking body",
			method: http.MethodPost,
			path:   "/api/v1/events/evt_1/bookings",
			body:   `{"employee_id":"E1002","idempotency_key":"book-1"}`,
			status: http.StatusBadRequest,
		},
		{name: "legacy tickets path removed", method: http.MethodGet, path: "/api/v1/employees/E1002/tickets", status: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeTicketingService{}
			router := testTicketingRouter(service)
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			authorizeRequest(t, req, ticketing.RoleEmployee)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tt.status, rec.Code, "%s body=%s", tt.path, rec.Body.String())
			assert.False(t, service.bookCalled, "rejected booking request must not call service.Book")
			assert.Empty(t, service.listEventsEmployeeID, "rejected request leaked caller employee id")
			assert.Empty(t, service.getEventEmployeeID, "rejected request leaked caller employee id")
			assert.Empty(t, service.eligibilityEmployeeID, "rejected request leaked caller employee id")
			assert.Empty(t, service.listTicketsEmployeeID, "rejected request leaked caller employee id")
		})
	}
}

func TestOwnDataHandlersUseProviderClaimsIdentity(t *testing.T) {
	service := &fakeTicketingService{}
	router := testTicketingRouter(service)

	requests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/api/v1/events", "", http.StatusOK},
		{http.MethodGet, "/api/v1/events/evt_1", "", http.StatusOK},
		{http.MethodGet, "/api/v1/events/evt_1/eligibility", "", http.StatusOK},
		{http.MethodPost, "/api/v1/events/evt_1/bookings", `{"idempotency_key":"book-1"}`, http.StatusCreated},
		{http.MethodGet, "/api/v1/me/tickets", "", http.StatusOK},
	}
	for _, tt := range requests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
		authorizeRequest(t, req, ticketing.RoleEmployee)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		require.Equal(t, tt.status, rec.Code, "%s %s body=%s", tt.method, tt.path, rec.Body.String())
	}

	assert.Equal(t, "E1001", service.listEventsActor.ID)
	assert.Equal(t, ticketing.RoleEmployee, service.listEventsActor.Role)
	assert.Empty(t, service.listEventsEmployeeID, "own-data handler passed employee id instead of relying on provider claims")
	assert.Empty(t, service.getEventEmployeeID, "own-data handler passed employee id instead of relying on provider claims")
	assert.Empty(t, service.eligibilityEmployeeID, "own-data handler passed employee id instead of relying on provider claims")
	assert.Empty(t, service.listTicketsEmployeeID, "own-data handler passed employee id instead of relying on provider claims")
	assert.True(t, service.bookCalled, "expected canonical booking request to call service.Book")
	assert.Equal(t, "E1001", service.bookActor.ID)
	assert.Equal(t, ticketing.RoleEmployee, service.bookActor.Role)
	assert.Empty(t, service.bookRequest.EmployeeID)
	assert.Equal(t, "E1001", service.listTicketsActor.ID)
	assert.Equal(t, ticketing.RoleEmployee, service.listTicketsActor.Role)
}

func testTicketingRouter(service *fakeTicketingService) http.Handler {
	return NewRouter(Dependencies{
		DB:             fakePinger{},
		Ticketing:      service,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RequestTimeout: time.Second,
		AppEnv:         "test",
		ProviderAuth:   ProviderAuthConfig{Secret: providerTestSecret()},
	})
}
