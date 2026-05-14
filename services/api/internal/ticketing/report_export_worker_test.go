package ticketing

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingReportStore struct {
	err         error
	keys        []string
	contentType string
	body        string
}

func TestNullableIntCSVValue(t *testing.T) {
	assert.Equal(t, "", nullableIntCSVValue(nil))
	value := 12
	assert.Equal(t, "12", nullableIntCSVValue(&value))
}

func (s *recordingReportStore) Put(ctx context.Context, key string, contentType string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.keys = append(s.keys, key)
	s.contentType = contentType
	s.body = string(body)
	return s.err
}

func TestProcessOutboxOnceGeneratesReportExportArtifact(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Exportable Report",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, store.keys, 1)
	assert.Equal(t, export.ObjectKey, store.keys[0])
	assert.Equal(t, "text/csv; charset=utf-8", store.contentType)
	assert.Contains(t, store.body, "event_id,title,capacity")
	assert.Contains(t, store.body, event.Title)

	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
}

func TestProcessOutboxOncePrioritizesReportExportOverNotificationBacklog(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	insertWorkerOutbox(t, service, ctx, "out-older-notification", "pending", 0)
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	store := &recordingReportStore{}
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		ReportStore: store,
		MaxAttempts: 3,
		BatchSize:   1,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls, "want report export before notification")
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-older-notification", "pending", 0)
}

func TestProcessOutboxOnceMarksReportExportFailureRetryable(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	store := &recordingReportStore{err: errors.New("object store unavailable")}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusFailed, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "pending", 1)
}

func assertReportExportState(t *testing.T, service *Service, ctx context.Context, exportID string, wantStatus string, wantCompleted bool) {
	t.Helper()
	var status string
	var completedAt sql.NullTime
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status, completed_at FROM report_exports WHERE export_id = $1`, exportID).Scan(&status, &completedAt))
	assert.Equal(t, wantStatus, status)
	assert.Equal(t, wantCompleted, completedAt.Valid)
}

func assertReportExportOutboxStatus(t *testing.T, service *Service, ctx context.Context, exportID string, wantStatus string, wantAttempts int) {
	t.Helper()
	var gotStatus string
	var gotAttempts int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, attempts FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`, exportID, outboxEventReportExportRequested).
		Scan(&gotStatus, &gotAttempts))
	assert.Equal(t, wantStatus, gotStatus)
	assert.Equal(t, wantAttempts, gotAttempts)
}
