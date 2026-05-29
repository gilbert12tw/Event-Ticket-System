package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayOutboxApplyDoesNotRewriteExistingReportExport(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC)
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	outboxID := reportExportReplayOutboxID(t, service, ctx, export.ExportID)
	_, err = service.db.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'dead_letter',
			attempts = 4,
			retry_count = 4,
			last_error = 'object store unavailable',
			dead_letter_at = $2,
			available_at = now() + interval '1 hour',
			created_at = $2
		WHERE aggregate_id = $1 AND event_type = $3`, export.ExportID, base, outboxEventReportExportRequestedV2)
	require.NoError(t, err)

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:       outboxWorkerKindExport,
		From:       base.Add(-time.Minute),
		To:         base.Add(time.Minute),
		EventTypes: []string{outboxEventReportExportRequestedV2},
		DryRun:     false,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedCount)
	require.NotNil(t, result.EnqueuedCount)
	assert.Equal(t, 1, *result.EnqueuedCount)
	assert.NotEmpty(t, result.AuditID)
	state := loadReplayOutboxState(t, service, ctx, outboxID)
	assert.Equal(t, "pending", state.publishStatus)
	assert.Equal(t, 0, state.attempts)
	assert.Equal(t, 0, state.retryCount)
	assert.False(t, state.deadLetterAt.Valid)
	assert.True(t, state.availableNow)
	audit := loadReplayAuditMetadata(t, service, ctx, result.AuditID)
	assert.Equal(t, outboxWorkerKindExport, audit["kind"])

	store := &recordingReportStore{existing: map[string]bool{export.ObjectKey: true}}
	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindExport},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, store.existsCalls)
	assert.Empty(t, store.keys, "replayed export with existing artifact must not rewrite object storage")
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
}

func reportExportReplayOutboxID(t *testing.T, service *Service, ctx context.Context, exportID string) string {
	t.Helper()
	var outboxID string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT outbox_id
		FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`,
		exportID, outboxEventReportExportRequestedV2).Scan(&outboxID))
	return outboxID
}
