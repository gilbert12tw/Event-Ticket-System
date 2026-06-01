package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoClockRoutesRequireFeatureFlagAndAdminRole(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/demo-clock", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	testRouter(Dependencies{}).ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	clock := ticketing.NewDemoClock()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/debug/demo-clock", nil)
	authorizeRequest(t, req, ticketing.RoleEmployee)
	rec = httptest.NewRecorder()

	testRouter(Dependencies{DemoClock: clock}).ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"role is not allowed"`)
}

func TestDemoClockRoutesExposeAndUpdateBusinessTime(t *testing.T) {
	clock := ticketing.NewDemoClockWithRealNow(func() time.Time {
		return time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	})
	router := testRouter(Dependencies{DemoClock: clock})

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/debug/demo-clock", nil)
	authorizeRequest(t, getReq, ticketing.RoleActivityAdmin)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
	assertEnvelope(t, getRec.Body.String(), `"enabled":true`, `"mode":"real"`, `"now":"2026-05-31T08:00:00Z"`)

	body := bytes.NewBufferString(`{"mode":"fixed","now":"2026-06-01T09:30:00+08:00","reason":"cutoff demo"}`)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/debug/demo-clock", body)
	authorizeRequest(t, putReq, ticketing.RoleActivityAdmin)
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)

	require.Equal(t, http.StatusOK, putRec.Code, putRec.Body.String())
	assertEnvelope(t, putRec.Body.String(), `"mode":"fixed"`, `"now":"2026-06-01T01:30:00Z"`, `"reason":"cutoff demo"`)
	assert.Equal(t, time.Date(2026, 6, 1, 1, 30, 0, 0, time.UTC), clock.Now())

	resetReq := httptest.NewRequest(http.MethodPut, "/api/v1/debug/demo-clock", bytes.NewBufferString(`{"mode":"real","reason":"reset"}`))
	authorizeRequest(t, resetReq, ticketing.RoleSystemAdmin)
	resetRec := httptest.NewRecorder()
	router.ServeHTTP(resetRec, resetReq)

	require.Equal(t, http.StatusOK, resetRec.Code, resetRec.Body.String())
	assertEnvelope(t, resetRec.Body.String(), `"mode":"real"`, `"reason":"reset"`)
	assert.Equal(t, time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC), clock.Now())
}

func TestDemoClockUpdateRejectsInvalidFixedPayload(t *testing.T) {
	router := testRouter(Dependencies{DemoClock: ticketing.NewDemoClock()})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/debug/demo-clock", bytes.NewBufferString(`{"mode":"fixed","reason":"missing now"}`))
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"now is required when demo clock mode is fixed"`)
}

func TestDemoClockUpdateValidatesReasonContract(t *testing.T) {
	router := testRouter(Dependencies{DemoClock: ticketing.NewDemoClock()})
	longReason := strings.Repeat("x", 301)
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing reason",
			body: `{"mode":"real"}`,
			want: `"reason is required"`,
		},
		{
			name: "blank reason",
			body: `{"mode":"real","reason":"   "}`,
			want: `"reason is required"`,
		},
		{
			name: "oversized reason",
			body: `{"mode":"real","reason":"` + longReason + `"}`,
			want: `"reason must be 300 characters or fewer"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/debug/demo-clock", bytes.NewBufferString(tc.body))
			authorizeRequest(t, req, ticketing.RoleActivityAdmin)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assertEnvelope(t, rec.Body.String(), tc.want)
		})
	}
}
