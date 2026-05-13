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

			if rec.Code != tt.status {
				t.Fatalf("%s status = %d, want %d, body = %s", tt.path, rec.Code, tt.status, rec.Body.String())
			}
			if service.bookCalled {
				t.Fatal("rejected booking request must not call service.Book")
			}
			if service.listEventsEmployeeID != "" || service.getEventEmployeeID != "" || service.eligibilityEmployeeID != "" || service.listTicketsEmployeeID != "" {
				t.Fatalf("rejected request leaked caller employee id into service: %#v", service)
			}
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

		if rec.Code != tt.status {
			t.Fatalf("%s %s status = %d, body = %s", tt.method, tt.path, rec.Code, rec.Body.String())
		}
	}

	if service.listEventsActor.ID != "E1001" || service.listEventsActor.Role != ticketing.RoleEmployee {
		t.Fatalf("list events actor = %+v", service.listEventsActor)
	}
	if service.listEventsEmployeeID != "" || service.getEventEmployeeID != "" || service.eligibilityEmployeeID != "" || service.listTicketsEmployeeID != "" {
		t.Fatalf("own-data handler passed employee id instead of relying on provider claims: %#v", service)
	}
	if !service.bookCalled {
		t.Fatal("expected canonical booking request to call service.Book")
	}
	if service.bookActor.ID != "E1001" || service.bookActor.Role != ticketing.RoleEmployee {
		t.Fatalf("book actor = %+v", service.bookActor)
	}
	if service.bookRequest.EmployeeID != "" {
		t.Fatalf("booking request employee id = %q", service.bookRequest.EmployeeID)
	}
	if service.listTicketsActor.ID != "E1001" || service.listTicketsActor.Role != ticketing.RoleEmployee {
		t.Fatalf("tickets actor = %+v", service.listTicketsActor)
	}
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
