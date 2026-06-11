package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var projectionExportCSVHeader = []string{
	"event_id", "title", "capacity_type", "capacity", "confirmed_count", "waitlist_count",
	"employee_count", "family_count", "total_attendee_count", "ticket_count", "checkin_count",
	"remaining_capacity", "city_distribution", "starts_at", "projection_status",
}

func TestReportExportSettingsNormalized(t *testing.T) {
	defaults := ReportExportSettings{}.normalized()
	assert.Equal(t, ReportExportSourceProjection, defaults.Source)
	assert.Equal(t, 60, defaults.StaleThresholdSeconds)
	assert.Equal(t, ReportExportStalePolicyFail, defaults.StalePolicy)

	explicit := ReportExportSettings{
		Source:                ReportExportSourceOperational,
		StaleThresholdSeconds: 120,
		StalePolicy:           ReportExportStalePolicyFail,
	}.normalized()
	assert.Equal(t, ReportExportSourceOperational, explicit.Source)
	assert.Equal(t, 120, explicit.StaleThresholdSeconds)
}

// WS5-AC-4: the export reads projection rows, not the operational tables. The
// summary row is mutated to deliberately diverge from OLTP; the CSV must show
// the projection values.
func TestReportExportCSVReadsProjectionRows(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtProj")
	seedRebuildEmployee(t, service, ctx, "ENG1", "Engineering")
	seedRegistrationWithFamily(t, service, ctx, registrationSeed{regID: "r1", eventID: "evtProj", employeeID: "ENG1", status: "confirmed", familyCount: 1})
	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	// Diverge the projection from OLTP truth to prove the read source.
	_, err = service.db.Exec(ctx,
		`UPDATE reporting_event_summary SET confirmed_count = 7, employee_count = 7 WHERE event_id = 'evtProj'`)
	require.NoError(t, err)

	body, err := service.buildReportExportCSVFromProjection(ctx, 60)
	require.NoError(t, err)
	records := parseReportCSV(t, string(body))
	assert.Equal(t, projectionExportCSVHeader, records[0])

	row := findReportCSVRecord(t, records, "evtProj")
	assert.Equal(t, "7", row[4], "confirmed_count must come from the projection, not OLTP")
	assert.Equal(t, "7", row[6], "employee_count must come from the projection, not OLTP")
	assert.Equal(t, reportProjectionStatusFresh, row[14])
}

// WS5-AC-4: a missing projection row is marked pending_projection with empty
// count cells — zeros are never fabricated; event metadata stays populated.
func TestReportExportCSVMarksMissingProjectionRowPending(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtFresh")
	seedRebuildEvent(t, service, ctx, "evtMissing")
	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `DELETE FROM reporting_event_summary WHERE event_id = 'evtMissing'`)
	require.NoError(t, err)

	body, err := service.buildReportExportCSVFromProjection(ctx, 60)
	require.NoError(t, err)
	records := parseReportCSV(t, string(body))

	pending := findReportCSVRecord(t, records, "evtMissing")
	assert.Equal(t, reportProjectionStatusPending, pending[14])
	assert.Equal(t, "evtMissing", pending[1], "title metadata stays populated (seed uses event_id as title)")
	assert.Equal(t, "limited", pending[2])
	assert.Equal(t, "1000", pending[3])
	for i := 4; i <= 12; i++ {
		assert.Empty(t, pending[i], "count cell %d must be empty, never a fabricated zero", i)
	}
	assert.NotEmpty(t, pending[13], "starts_at metadata stays populated")

	fresh := findReportCSVRecord(t, records, "evtFresh")
	assert.Equal(t, reportProjectionStatusFresh, fresh[14])
	assert.Equal(t, "0", fresh[4], "rebuilt zero-activity row carries genuine zeros")
}

// A stale projection (unprocessed projection backlog older than the
// threshold) fails the export retryably: status back to pending, sanitized
// last_error, retry audit row, and no artifact written.
func TestProcessOutboxOnceReportExportStaleProjectionRetryable(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	seedStaleProjectionBacklog(t, service, ctx, "ob-stale-1")
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindExport},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Empty(t, store.keys, "stale projection must not produce an artifact")
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusPending, false)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "pending", 1)
	state := loadReportExportOutboxFailureState(t, service, ctx, export.ExportID)
	assert.Equal(t, "reporting projection stale", state.lastError)

	outboxID := reportExportOutboxIDForEventType(t, service, ctx, export.ExportID, outboxEventReportExportRequestedV2)
	var retryAudits int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs
		WHERE action = $1 AND entity_type = 'outbox_event' AND entity_id = $2`,
		outboxRetryScheduledAction, outboxID).Scan(&retryAudits))
	assert.Equal(t, 1, retryAudits)
}

// A projection that never becomes fresh dead-letters the export at max
// attempts: report_exports.status='failed', dead-letter audit, no artifact.
func TestProcessOutboxOnceReportExportStaleProjectionDeadLetters(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE outbox_events SET attempts = 2 WHERE aggregate_id = $1 AND event_type = $2`,
		export.ExportID, outboxEventReportExportRequestedV2)
	require.NoError(t, err)
	seedStaleProjectionBacklog(t, service, ctx, "ob-stale-2")
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindExport},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Empty(t, store.keys)
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusFailed, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "dead_letter", 3)
	state := loadReportExportOutboxFailureState(t, service, ctx, export.ExportID)
	assert.Equal(t, "reporting projection stale", state.lastError)

	outboxID := reportExportOutboxIDForEventType(t, service, ctx, export.ExportID, outboxEventReportExportRequestedV2)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.dead_letter'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, outboxID)
	assert.Equal(t, outboxDeadLetterReasonRetryExhausted, audit["reason"])
}

// A missing event_summary offsets row means the projection is unavailable —
// same controlled failure as staleness.
func TestProcessOutboxOnceReportExportProjectionUnavailable(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `DELETE FROM reporting_projection_offsets WHERE projection_name = $1`, projectionProjectionName)
	require.NoError(t, err)
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindExport},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Empty(t, store.keys)
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusPending, false)
	state := loadReportExportOutboxFailureState(t, service, ctx, export.ExportID)
	assert.Equal(t, "reporting projection stale", state.lastError)
}

// EXPORTS_SOURCE=operational restores the byte-exact Phase 1 path: 14-column
// CSV from the OLTP aggregation, no projection dependency.
func TestProcessOutboxOnceReportExportOperationalSourceRollback(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	// Even with the projection unavailable, the operational source succeeds.
	_, err = service.db.Exec(ctx, `DELETE FROM reporting_projection_offsets WHERE projection_name = $1`, projectionProjectionName)
	require.NoError(t, err)
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		ReportStore:    store,
		ExportSettings: ReportExportSettings{Source: ReportExportSourceOperational},
		MaxAttempts:    3,
		WorkerKinds:    []string{outboxWorkerKindExport},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, store.keys, 1)
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	records := parseReportCSV(t, store.body)
	require.NotEmpty(t, records)
	assert.Len(t, records[0], 14, "operational source keeps the byte-exact Phase 1 header")
	assert.NotContains(t, records[0], "projection_status")
}

// Phase 1 privacy whitelist regression on the projection path: no employee
// rows, full names, employee IDs, tokens, QR payloads, or provider tokens.
func TestReportExportProjectionCSVExcludesPII(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:        "Projection Privacy Report",
		EventCity:    "Tainan",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID: "E1001", IdempotencyKey: "projection-pii-book", FamilyCount: 2,
	})
	require.NoError(t, err)
	_, err = service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	body, err := service.buildReportExportCSVFromProjection(ctx, 60)
	require.NoError(t, err)
	csv := string(body)

	records := parseReportCSV(t, csv)
	row := findReportCSVRecord(t, records, event.EventID)
	assert.Equal(t, "1", row[4], "confirmed_count from rebuilt projection")
	assert.Equal(t, "1", row[6], "employee_count from rebuilt projection")
	assert.Equal(t, "2", row[7], "family_count from rebuilt projection")
	assert.Equal(t, "3", row[8], "total_attendee_count derived from projection counts")
	assert.Equal(t, "1", row[9], "ticket_count from rebuilt projection")
	assert.Equal(t, `{"Tainan":3}`, row[12])

	assert.NotContains(t, csv, "Ariel Chen")
	assert.NotContains(t, csv, "E1001")
	assert.NotContains(t, csv, "signed_token")
	assert.NotContains(t, csv, "qr_payload")
	assert.NotContains(t, csv, "provider_token")
	assert.NotContains(t, csv, "@cets.local")
}

// seedStaleProjectionBacklog inserts a pending projection outbox event whose
// created_at is older than the staleness threshold (60s default), making the
// projection demonstrably behind OLTP.
func seedStaleProjectionBacklog(t *testing.T, service *Service, ctx context.Context, outboxID string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `
		INSERT INTO outbox_events
			(outbox_id, aggregate_id, event_type, payload, publish_status, schema_version, attempts, available_at, created_at)
		VALUES ($1, 'evt-stale', $2, '{"aggregate_id":"evt-stale","inner_event_type":"booking.confirmed"}'::jsonb,
		        'pending', 1, 0, now() - interval '5 minutes', now() - interval '5 minutes')`,
		outboxID, outboxEventReportingProjectionUpdateRequiredV2)
	require.NoError(t, err)
}
