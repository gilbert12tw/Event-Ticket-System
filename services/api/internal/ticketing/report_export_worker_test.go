package ticketing

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type recordingReportStore struct {
	err         error
	keys        []string
	contentType string
	body        string
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
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Exportable Report",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(store.keys) != 1 || store.keys[0] != export.ObjectKey {
		t.Fatalf("store keys = %+v, want %s", store.keys, export.ObjectKey)
	}
	if store.contentType != "text/csv; charset=utf-8" {
		t.Fatalf("content type = %q", store.contentType)
	}
	if !strings.Contains(store.body, "event_id,title,capacity") || !strings.Contains(store.body, event.Title) {
		t.Fatalf("csv body = %q", store.body)
	}

	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
}

func TestProcessOutboxOncePrioritizesReportExportOverNotificationBacklog(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	insertWorkerOutbox(t, service, ctx, "out-older-notification", "pending", 0)
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingReportStore{}
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		ReportStore: store,
		MaxAttempts: 3,
		BatchSize:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want report export before notification", sender.calls)
	}
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-older-notification", "pending", 0)
}

func TestProcessOutboxOnceMarksReportExportFailureRetryable(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingReportStore{err: errors.New("object store unavailable")}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusFailed, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "pending", 1)
}

func assertReportExportState(t *testing.T, service *Service, ctx context.Context, exportID string, wantStatus string, wantCompleted bool) {
	t.Helper()
	var status string
	var completedAt sql.NullTime
	if err := service.db.QueryRow(ctx, `SELECT status, completed_at FROM report_exports WHERE export_id = $1`, exportID).Scan(&status, &completedAt); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || completedAt.Valid != wantCompleted {
		t.Fatalf("report export = (%s, completed=%t), want (%s, completed=%t)", status, completedAt.Valid, wantStatus, wantCompleted)
	}
}

func assertReportExportOutboxStatus(t *testing.T, service *Service, ctx context.Context, exportID string, wantStatus string, wantAttempts int) {
	t.Helper()
	var gotStatus string
	var gotAttempts int
	if err := service.db.QueryRow(ctx, `SELECT publish_status, attempts FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`, exportID, outboxEventReportExportRequested).
		Scan(&gotStatus, &gotAttempts); err != nil {
		t.Fatal(err)
	}
	if gotStatus != wantStatus || gotAttempts != wantAttempts {
		t.Fatalf("outbox = (%s, %d), want (%s, %d)", gotStatus, gotAttempts, wantStatus, wantAttempts)
	}
}
