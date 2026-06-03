package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadyzOK(t *testing.T) {
	router := testRouter(Dependencies{DB: fakeSchemaPinger{schemaReady: true}})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":true`, `"status":"ready"`)
}

func TestReadyzCachesDatabaseProbe(t *testing.T) {
	db := &countingSchemaPinger{schemaReady: true}
	router := testRouter(Dependencies{DB: db})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	require.Equal(t, 1, db.pings)
	require.Equal(t, 1, db.queries)
}

func TestReadyzRejectsUnmigratedDatabase(t *testing.T) {
	router := testRouter(Dependencies{DB: fakeSchemaPinger{schemaReady: false}})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"database schema is not ready"`)
}

func TestReadyzRequiresBookingBanSchema(t *testing.T) {
	assert.Contains(t, requiredSchemaReadyQuery, "to_regclass('public.booking_bans')")
}

func TestReadyzDatabaseUnavailable(t *testing.T) {
	router := testRouter(Dependencies{DB: fakePinger{err: errors.New("down")}})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"database is not ready"`)
}
