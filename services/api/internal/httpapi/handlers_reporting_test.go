package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T, ctx context.Context, databaseURL string) (*pgxpool.Pool, func()) {
	t.Helper()
	adminPool, err := postgres.Connect(ctx, databaseURL)
	require.NoError(t, err)

	schema := fmt.Sprintf("httpapi_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchema))
	require.NoError(t, err)

	cfg, err := pgxpool.ParseConfig(databaseURL)
	require.NoError(t, err)
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)

	return pool, func() {
		pool.Close()
		dropCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchema))
		adminPool.Close()
	}
}

func setupReportingTest(t *testing.T) (*pgxpool.Pool, http.Handler, func()) {
	pool, router, _, cleanup := setupReportingTestWithService(t)
	return pool, router, cleanup
}

func setupReportingTestWithService(t *testing.T) (*pgxpool.Pool, http.Handler, *ticketing.Service, func()) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, cleanupDB := newTestDB(t, ctx, databaseURL)
	require.NoError(t, postgres.Migrate(ctx, pool))

	// The migration seeds event_summary with -infinity. Delete it so we can start clean.
	_, err := pool.Exec(ctx, "DELETE FROM reporting_projection_offsets WHERE projection_name = 'event_summary'")
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := ticketing.NewService(pool, ticketing.NewSigner("secret"), logger)

	router := testRouter(Dependencies{
		DB:                          pool,
		Ticketing:                   service,
		ReportStaleThresholdSeconds: 60,
	})

	return pool, router, service, cleanupDB
}

func TestReportsHandler_Fresh(t *testing.T) {
	pool, router, cleanup := setupReportingTest(t)
	defer cleanup()
	ctx := context.Background()

	// Seed fresh projection offset (10s ago, threshold is 60)
	_, err := pool.Exec(ctx, `
		INSERT INTO reporting_projection_offsets (projection_name, last_processed_at, updated_at)
		VALUES ('event_summary', now(), now() - interval '10 seconds')`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(), `"degraded":false`, `"source":"reporting_projection"`)
}

func TestReportsHandler_Stale(t *testing.T) {
	pool, router, cleanup := setupReportingTest(t)
	defer cleanup()
	ctx := context.Background()

	// Seed stale projection offset (120s ago, threshold is 60)
	_, err := pool.Exec(ctx, `
		INSERT INTO reporting_projection_offsets (projection_name, last_processed_at, updated_at)
		VALUES ('event_summary', now(), now() - interval '120 seconds')`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(), `"degraded":true`, `"source":"reporting_projection"`, `"data":[]`)
}

func TestReportsHandler_MissingProjection(t *testing.T) {
	_, router, service, cleanup := setupReportingTestWithService(t) // DB is already cleared by setupReportingTest
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, service.SeedDemoData(ctx))
	admin := ticketing.Actor{ID: "admin-1", Role: ticketing.RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, ticketing.CreateEventRequest{
		Title:     "Missing Projection",
		EventCity: "Taipei",
		Capacity:  1,
		Status:    ticketing.EventStatusPublished,
		Rule:      ticketing.RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, ticketing.Actor{ID: "E1001", Role: ticketing.RoleEmployee}, event.EventID, ticketing.BookingRequest{EmployeeID: "E1001", IdempotencyKey: "missing-projection-book"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	response := decodeReportsResponse(t, rec.Body.Bytes())
	assert.True(t, response.Success)
	assert.Equal(t, ticketing.ReportSourceUnavailable, response.Meta.Source)
	assert.Equal(t, -1, response.Meta.LagSeconds)
	assert.Nil(t, response.Meta.AsOf)
	row := findReportRow(t, response.Data, event.EventID)
	assert.Equal(t, 0, row.ConfirmedCount)
	assert.Equal(t, 0, row.WaitlistCount)
	assert.Equal(t, 0, row.EmployeeCount)
	assert.Equal(t, 0, row.FamilyCount)
	assert.Equal(t, 0, row.TotalAttendeeCount)
	assert.Equal(t, 0, row.TicketCount)
	assert.Equal(t, 0, row.CheckinCount)
}

func TestReportsHandler_DeniedRole(t *testing.T) {
	_, router, cleanup := setupReportingTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports", nil)
	authorizeRequest(t, req, ticketing.RoleEmployee)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.NotContains(t, rec.Body.String(), `"meta"`)
	assert.NotContains(t, rec.Body.String(), `"report"`)
}

func TestReportsHandler_Unauthenticated(t *testing.T) {
	_, router, cleanup := setupReportingTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestReportsHandler_ThresholdFromEnv(t *testing.T) {
	pool, _, cleanup := setupReportingTest(t)
	defer cleanup()
	ctx := context.Background()

	// Override the testRouter threshold to 10s
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := ticketing.NewService(pool, ticketing.NewSigner("secret"), logger)
	router := testRouter(Dependencies{
		DB:                          pool,
		Ticketing:                   service,
		ReportStaleThresholdSeconds: 10,
	})

	// Seed projection offset 30s ago (stale against threshold 10)
	_, err := pool.Exec(ctx, `
		INSERT INTO reporting_projection_offsets (projection_name, last_processed_at, updated_at)
		VALUES ('event_summary', now(), now() - interval '30 seconds')`)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports", nil)
	authorizeRequest(t, req, ticketing.RoleActivityAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertEnvelope(t, rec.Body.String(), `"degraded":true`)
}

type fakeReportStore struct {
	body        []byte
	contentType string
	key         string
}

func (s *fakeReportStore) Get(_ context.Context, key string) ([]byte, string, error) {
	s.key = key
	return s.body, s.contentType, nil
}

func TestReportExportDownloadHandlerServesReadyCSV(t *testing.T) {
	service := &fakeTicketingService{}
	store := &fakeReportStore{
		body:        []byte("event_id,title\n"),
		contentType: "text/csv; charset=utf-8",
	}
	router := testRouter(Dependencies{Ticketing: service, ReportStore: store})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/exports/exp_1/download", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "exports/exp_1.csv", store.key)
	assert.Equal(t, "text/csv; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="exp_1.csv"`, rec.Header().Get("Content-Disposition"))
	assert.Equal(t, "event_id,title\n", rec.Body.String())
}

func TestReportExportDownloadHandlerRejectsPendingExport(t *testing.T) {
	service := &fakeTicketingService{reportExport: ticketing.ReportExport{
		ExportID:   "exp_pending",
		ReportType: ticketing.ReportExportTypeParticipation,
		Format:     ticketing.ReportExportFormatCSV,
		Status:     ticketing.ReportExportStatusPending,
		ObjectKey:  "exports/exp_pending.csv",
	}}
	store := &fakeReportStore{body: []byte("should not read")}
	router := testRouter(Dependencies{Ticketing: service, ReportStore: store})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports/exports/exp_pending/download", nil)
	authorizeRequest(t, req, ticketing.RoleHRAdmin)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Empty(t, store.key)
	assertEnvelope(t, rec.Body.String(), `"success":false`, `"report export is not ready"`)
}

type reportsResponse struct {
	Success bool                  `json:"success"`
	Data    []ticketing.ReportRow `json:"data"`
	Meta    ticketing.ReportMeta  `json:"meta"`
}

func decodeReportsResponse(t *testing.T, body []byte) reportsResponse {
	t.Helper()
	var response reportsResponse
	require.NoError(t, json.Unmarshal(body, &response))
	return response
}

func findReportRow(t *testing.T, rows []ticketing.ReportRow, eventID string) ticketing.ReportRow {
	t.Helper()
	for _, row := range rows {
		if row.EventID == eventID {
			return row
		}
	}
	require.Failf(t, "report row not found", "event_id=%s", eventID)
	return ticketing.ReportRow{}
}
