package ticketing

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingReportStore struct {
	err         error
	keys        []string
	existing    map[string]bool
	existsCalls int
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

func (s *recordingReportStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.existsCalls++
	return s.existing[key], nil
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
	// Default source is the reporting projection (PH2-45); populate it.
	_, err = service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, store.keys, 1)
	assert.Equal(t, export.ObjectKey, store.keys[0])
	assert.Equal(t, "text/csv; charset=utf-8", store.contentType)
	records := parseReportCSV(t, store.body)
	require.NotEmpty(t, records)
	assert.Equal(t, projectionExportCSVHeader, records[0])
	row := findReportCSVRecord(t, records, event.EventID)
	assert.Equal(t, reportProjectionStatusFresh, row[14])
	assert.Contains(t, store.body, event.Title)
	assert.NotContains(t, store.body, "Ariel Chen")
	assert.NotContains(t, store.body, "signed_token")
	assert.NotContains(t, store.body, "qr_payload")

	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
}

func TestProcessOutboxOnceGeneratesReportExportArtifactForLegacyEvent(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	convertReportExportOutboxToLegacyEvent(t, service, ctx, export)
	store := &recordingReportStore{}
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindExport},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	require.Len(t, store.keys, 1)
	assert.Equal(t, export.ObjectKey, store.keys[0])
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatusForEventType(t, service, ctx, export.ExportID, outboxEventReportExportRequested, "published", 1)
}

func TestBuildReportExportCSVIncludesAggregateColumns(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:        "CSV Aggregate Report",
		EventCity:    "Tainan",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "csv-aggregate-book", FamilyCount: 2})
	require.NoError(t, err)

	body, err := service.buildReportExportCSV(ctx)
	require.NoError(t, err)
	records := parseReportCSV(t, string(body))
	require.GreaterOrEqual(t, len(records), 2)
	row := findReportCSVRecord(t, records, event.EventID)
	assert.Equal(t, event.Title, row[1])
	assert.Equal(t, CapacityTypeUnlimited, row[2])
	assert.Equal(t, "", row[3])
	assert.Equal(t, "1", row[4])
	assert.Equal(t, "0", row[5])
	assert.Equal(t, "1", row[6])
	assert.Equal(t, "2", row[7])
	assert.Equal(t, "3", row[8])
	assert.Equal(t, `{"Tainan":3}`, row[12])
	assert.NotContains(t, string(body), "Ariel Chen")
	assert.NotContains(t, string(body), "E1001")
	assert.NotContains(t, string(body), "signed_token")
	assert.NotContains(t, string(body), "qr_payload")
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
	providerToken := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJyZXRyeSJ9.signature"
	store := &recordingReportStore{err: errors.New("object store unavailable for " + export.ObjectKey + " and e1001@cets.local with token " + providerToken)}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusPending, false)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "pending", 1)
	state := loadReportExportOutboxFailureState(t, service, ctx, export.ExportID)
	assert.Equal(t, "object store unavailable", state.lastError)
	assert.NotContains(t, state.lastError, export.ObjectKey)
	assert.NotContains(t, strings.ToLower(state.lastError), "e1001@cets.local")
	assert.NotContains(t, state.lastError, providerToken)
}

func TestProcessOutboxOncePublishesExistingReportExportAfterCrashWithoutRewrite(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'processing',
			attempts = 1,
			available_at = now() - interval '1 minute',
			lease_started_at = now() - interval '2 minutes'
		WHERE aggregate_id = $1 AND event_type = $2`, export.ExportID, outboxEventReportExportRequestedV2)
	require.NoError(t, err)
	store := &recordingReportStore{existing: map[string]bool{export.ObjectKey: true}}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, store.existsCalls)
	assert.Empty(t, store.keys, "object already exists after crash; retry must not rewrite export artifact")
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 2)
	assertReportExportOutboxLeaseCleared(t, service, ctx, export.ExportID)
}

func TestProcessOutboxOnceMarksReportExportFailedAtMaxAttempts(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE outbox_events SET attempts = 2 WHERE aggregate_id = $1 AND event_type = $2`, export.ExportID, outboxEventReportExportRequestedV2)
	require.NoError(t, err)
	providerToken := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJyZXBvcnQifQ.signature"
	store := &recordingReportStore{err: errors.New("object store unavailable for " + export.ObjectKey + " and e1001@cets.local with token " + providerToken)}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{ReportStore: store, MaxAttempts: 3})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusFailed, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "dead_letter", 3)
	state := loadReportExportOutboxFailureState(t, service, ctx, export.ExportID)
	assert.Equal(t, 3, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
	assert.Equal(t, "object store unavailable", state.lastError)
	assert.NotContains(t, state.lastError, export.ObjectKey)
	assert.NotContains(t, strings.ToLower(state.lastError), "e1001@cets.local")
	assert.NotContains(t, state.lastError, providerToken)
	outboxID := reportExportOutboxIDForEventType(t, service, ctx, export.ExportID, outboxEventReportExportRequestedV2)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.dead_letter'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, outboxID)
	assert.Equal(t, outboxEventReportExportRequestedV2, audit["event_type"])
	assert.Equal(t, outboxWorkerKindExport, audit["worker_kind"])
	assert.Equal(t, float64(3), audit["retry_count"])
	assert.Equal(t, float64(2), audit["schema_version"])
	assert.Equal(t, outboxDeadLetterReasonRetryExhausted, audit["reason"])
	assertNoSensitiveJSONValues(t, audit, export.ObjectKey, "object store unavailable")
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
	assertReportExportOutboxStatusForEventType(t, service, ctx, exportID, outboxEventReportExportRequestedV2, wantStatus, wantAttempts)
}

func assertReportExportOutboxLeaseCleared(t *testing.T, service *Service, ctx context.Context, exportID string) {
	t.Helper()
	var leaseCleared bool
	require.NoError(t, service.db.QueryRow(ctx, `SELECT lease_started_at IS NULL
		FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`,
		exportID, outboxEventReportExportRequestedV2).Scan(&leaseCleared))
	assert.True(t, leaseCleared)
}

func assertReportExportOutboxStatusForEventType(t *testing.T, service *Service, ctx context.Context, exportID string, eventType string, wantStatus string, wantAttempts int) {
	t.Helper()
	var gotStatus string
	var gotAttempts int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, attempts FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`, exportID, eventType).
		Scan(&gotStatus, &gotAttempts))
	assert.Equal(t, wantStatus, gotStatus)
	assert.Equal(t, wantAttempts, gotAttempts)
}

func convertReportExportOutboxToLegacyEvent(t *testing.T, service *Service, ctx context.Context, export ReportExport) {
	t.Helper()
	tag, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET event_type = $1,
			payload = jsonb_build_object('requested_by', $2::text, 'report_type', $3::text, 'object_key', $4::text),
			schema_version = 1,
			idempotency_key = NULL,
			partition_key = NULL
		WHERE aggregate_id = $5 AND event_type = $6`,
		outboxEventReportExportRequested, export.RequestedBy, export.ReportType, export.ObjectKey, export.ExportID, outboxEventReportExportRequestedV2)
	require.NoError(t, err)
	require.Equal(t, int64(1), tag.RowsAffected())
}

func loadReportExportOutboxFailureState(t *testing.T, service *Service, ctx context.Context, exportID string) outboxFailureState {
	t.Helper()
	outboxID := reportExportOutboxIDForEventType(t, service, ctx, exportID, outboxEventReportExportRequestedV2)
	return loadOutboxFailureState(t, service, ctx, outboxID)
}

func reportExportOutboxIDForEventType(t *testing.T, service *Service, ctx context.Context, exportID string, eventType string) string {
	t.Helper()
	var outboxID string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT outbox_id FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`,
		exportID, eventType).Scan(&outboxID))
	return outboxID
}

func parseReportCSV(t *testing.T, body string) [][]string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	require.NoError(t, err)
	return records
}

func findReportCSVRecord(t *testing.T, records [][]string, eventID string) []string {
	t.Helper()
	for _, record := range records[1:] {
		require.Len(t, record, len(records[0]))
		if record[0] == eventID {
			return record
		}
	}
	require.Failf(t, "CSV record not found", "event_id=%s", eventID)
	return nil
}
