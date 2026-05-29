package ticketing

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/traceid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayOutboxDryRunDoesNotMutateRows(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-dry-run",
		EventType:    "notification.requested.v2",
		Status:       "dead_letter",
		CreatedAt:    base,
		RetryCount:   3,
		LastError:    "smtp failed",
		DeadLetterAt: base.Add(time.Minute),
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:  "out-replay-outside-window",
		EventType: "notification.requested.v2",
		Status:    "dead_letter",
		CreatedAt: base.Add(-2 * time.Hour),
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:  "out-replay-export",
		EventType: outboxEventReportExportRequested,
		Status:    "dead_letter",
		CreatedAt: base,
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:   "out-replay-live-pending",
		EventType:  "notification.requested.v2",
		Status:     "pending",
		CreatedAt:  base,
		LastError:  "live row must not be replayed",
		RetryCount: 1,
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:   "notification",
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Minute),
		DryRun: true,
	})

	require.NoError(t, err)
	assert.Equal(t, "notification", result.Kind)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.AffectedCount)
	assert.Nil(t, result.EnqueuedCount)
	assert.Empty(t, result.AuditID)
	state := loadReplayOutboxState(t, service, ctx, "out-replay-dry-run")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, "smtp failed", state.lastError)
	assert.True(t, state.deadLetterAt.Valid)
	liveState := loadReplayOutboxState(t, service, ctx, "out-replay-live-pending")
	assert.Equal(t, "pending", liveState.publishStatus)
	assert.Equal(t, "live row must not be replayed", liveState.lastError)
	assert.False(t, liveState.availableNow)
	assertReplayAuditCount(t, service, ctx, 0)
}

func TestReplayOutboxDryRunLogsTraceSafeOutcome(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx := traceid.WithContext(context.Background(), "trace-replay-dry-run-1")
	base := time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-dry-run-log",
		EventType:    "notification.requested.v2",
		Status:       "dead_letter",
		CreatedAt:    base,
		RetryCount:   3,
		LastError:    "smtp failed for e1001@cets.local and E1001",
		DeadLetterAt: base.Add(time.Minute),
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:   "notification",
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Minute),
		DryRun: true,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedCount)
	output := logs.String()
	assert.Contains(t, output, `"msg":"outbox replay dry-run"`)
	assert.Contains(t, output, `"trace_id":"trace-replay-dry-run-1"`)
	assert.Contains(t, output, `"action":"outbox.replay"`)
	assert.Contains(t, output, `"actor_role":"hr_admin"`)
	assert.Contains(t, output, `"kind":"notification"`)
	assert.Contains(t, output, `"dry_run":true`)
	assert.Contains(t, output, `"affected_count":1`)
	assert.NotContains(t, output, "out-replay-dry-run-log")
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "smtp failed")
	assert.NotContains(t, output, "payload")
}

func TestReplayOutboxApplyEnqueuesAndAudits(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-apply",
		EventType:    "notification.requested.v2",
		Status:       "dead_letter",
		CreatedAt:    base,
		Attempts:     4,
		RetryCount:   4,
		LastError:    "smtp failed for e1001@cets.local",
		DeadLetterAt: base.Add(time.Minute),
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:   "out-replay-apply-live-pending",
		EventType:  "notification.requested.v2",
		Status:     "pending",
		CreatedAt:  base,
		LastError:  "live row must not be mutated",
		Attempts:   2,
		RetryCount: 2,
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:       "notification",
		From:       base.Add(-time.Minute),
		To:         base.Add(time.Minute),
		EventTypes: []string{"notification.requested.v2"},
		DryRun:     false,
	})

	require.NoError(t, err)
	assert.False(t, result.DryRun)
	assert.Equal(t, 1, result.AffectedCount)
	require.NotNil(t, result.EnqueuedCount)
	assert.Equal(t, 1, *result.EnqueuedCount)
	assert.NotEmpty(t, result.AuditID)
	state := loadReplayOutboxState(t, service, ctx, "out-replay-apply")
	assert.Equal(t, "pending", state.publishStatus)
	assert.Empty(t, state.lastError)
	assert.Equal(t, 0, state.attempts)
	assert.Equal(t, 0, state.retryCount)
	assert.False(t, state.deadLetterAt.Valid)
	assert.True(t, state.availableNow)
	liveState := loadReplayOutboxState(t, service, ctx, "out-replay-apply-live-pending")
	assert.Equal(t, "pending", liveState.publishStatus)
	assert.Equal(t, "live row must not be mutated", liveState.lastError)
	assert.Equal(t, 2, liveState.attempts)
	assert.Equal(t, 2, liveState.retryCount)
	assert.False(t, liveState.availableNow)
	audit := loadReplayAuditMetadata(t, service, ctx, result.AuditID)
	assert.Equal(t, "notification", audit["kind"])
	assert.Equal(t, float64(1), audit["affected_count"])
	assert.Equal(t, float64(1), audit["enqueued_count"])
	assert.Equal(t, float64(1), audit["replayed_row_count"])
	assert.Equal(t, false, audit["replayed_rows_truncated"])
	replayedRows, ok := audit["replayed_rows"].([]interface{})
	require.True(t, ok)
	require.Len(t, replayedRows, 1)
	replayedRow, ok := replayedRows[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "out-replay-apply", replayedRow["outbox_id"])
	assert.Equal(t, "notification.requested.v2", replayedRow["event_type"])
	assert.Equal(t, outboxWorkerKindNotification, replayedRow["worker_kind"])
	assert.Equal(t, "dead_letter", replayedRow["previous_status"])
	assert.Equal(t, float64(4), replayedRow["previous_attempts"])
	assert.Equal(t, float64(4), replayedRow["previous_retry_count"])
	assert.Equal(t, "smtp failed for [redacted email]", replayedRow["previous_last_error"])
	assert.NotEmpty(t, replayedRow["previous_dead_letter_at"])
	var actorID, role, action, entityType, entityID string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT actor_id, role, action, entity_type, entity_id
		FROM audit_logs WHERE audit_id = $1`, result.AuditID).
		Scan(&actorID, &role, &action, &entityType, &entityID))
	assert.Equal(t, "hr-1", actorID)
	assert.Equal(t, RoleHRAdmin, role)
	assert.Equal(t, outboxReplayAction, action)
	assert.Equal(t, "outbox_queue", entityType)
	assert.Equal(t, "notification", entityID)
	encoded, err := json.Marshal(audit)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "e1001@cets.local")
	assert.NotContains(t, string(encoded), "E1001")
}

func TestReplayOutboxApplyLogsTraceSafeOutcome(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx := traceid.WithContext(context.Background(), "trace-replay-apply-1")
	base := time.Date(2026, 5, 28, 11, 30, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-apply-log",
		EventType:    "notification.requested.v2",
		Status:       "dead_letter",
		CreatedAt:    base,
		RetryCount:   4,
		LastError:    "smtp failed for e1001@cets.local and E1001",
		DeadLetterAt: base.Add(time.Minute),
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:   "notification",
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Minute),
		DryRun: false,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedCount)
	require.NotNil(t, result.EnqueuedCount)
	output := logs.String()
	assert.Contains(t, output, `"msg":"outbox replay applied"`)
	assert.Contains(t, output, `"trace_id":"trace-replay-apply-1"`)
	assert.Contains(t, output, `"action":"outbox.replay"`)
	assert.Contains(t, output, `"actor_role":"hr_admin"`)
	assert.Contains(t, output, `"kind":"notification"`)
	assert.Contains(t, output, `"dry_run":false`)
	assert.Contains(t, output, `"affected_count":1`)
	assert.Contains(t, output, `"enqueued_count":1`)
	assert.Contains(t, output, `"audit_id":"`)
	assert.NotContains(t, output, "out-replay-apply-log")
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "smtp failed")
	assert.NotContains(t, output, "payload")
}

func TestReplayOutboxApplyDoesNotDuplicateAlreadySentNotification(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 28, 11, 45, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-idempotent-sent",
		EventType:    "notification.requested.v2",
		Payload:      `{"channel":"email","recipient_employee_id":"E1001","event_id":"evt_replay_idempotent"}`,
		Status:       "dead_letter",
		CreatedAt:    base,
		Attempts:     4,
		RetryCount:   4,
		LastError:    "smtp failed for e1001@cets.local",
		DeadLetterAt: base.Add(time.Minute),
	})
	insertWorkerDelivery(t, service, ctx, "del-replay-already-sent", "out-replay-idempotent-sent", "email", deliveryStatusSent)

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:   "notification",
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Minute),
		DryRun: false,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedCount)
	assert.Equal(t, "pending", loadReplayOutboxState(t, service, ctx, "out-replay-idempotent-sent").publishStatus)

	sender := &recordingNotificationSender{}
	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-replay-idempotent-sent")
	assert.Equal(t, deliveryStatusSent, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-replay-idempotent-sent", "published", 1)
}

func TestReplayOutboxAllowsSystemAdminActor(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:  "out-replay-system-admin",
		EventType: "notification.requested.v2",
		Status:    "dead_letter",
		CreatedAt: base,
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "ops-replay", Role: RoleSystemAdmin}, ReplayOutboxRequest{
		Kind:   "notification",
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Minute),
		DryRun: true,
	})

	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.AffectedCount)
	assert.Nil(t, result.EnqueuedCount)
	assert.Empty(t, result.AuditID)
}

func TestReplayOutboxValidatesFilters(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		req  ReplayOutboxRequest
	}{
		{name: "unsupported kind", req: ReplayOutboxRequest{Kind: "unknown", From: base, To: base.Add(time.Minute), DryRun: true}},
		{name: "missing from", req: ReplayOutboxRequest{Kind: "notification", To: base.Add(time.Minute), DryRun: true}},
		{name: "inverted window", req: ReplayOutboxRequest{Kind: "notification", From: base.Add(time.Minute), To: base, DryRun: true}},
		{name: "event type wrong kind", req: ReplayOutboxRequest{Kind: "notification", From: base, To: base.Add(time.Minute), EventTypes: []string{outboxEventReportExportRequested}, DryRun: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, tc.req)

			require.Error(t, err)
			assert.Equal(t, 400, ErrorStatus(err))
		})
	}
}

func TestReplayOutboxValidatesReportExportV2AsExportKind(t *testing.T) {
	base := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	req, err := ValidateReplayOutboxRequest(ReplayOutboxRequest{
		Kind:       outboxWorkerKindExport,
		From:       base,
		To:         base.Add(time.Minute),
		EventTypes: []string{"report.export.requested.v2"},
		DryRun:     true,
	})

	require.NoError(t, err)
	assert.Equal(t, outboxWorkerKindExport, req.Kind)
	assert.Equal(t, []string{"report.export.requested.v2"}, req.EventTypes)

	_, err = ValidateReplayOutboxRequest(ReplayOutboxRequest{
		Kind:       outboxWorkerKindNotification,
		From:       base,
		To:         base.Add(time.Minute),
		EventTypes: []string{"report.export.requested.v2"},
		DryRun:     true,
	})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
}

func TestValidateReplayOutboxRejectsZeroWidthWindow(t *testing.T) {
	base := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	_, err := ValidateReplayOutboxRequest(ReplayOutboxRequest{
		Kind:   outboxWorkerKindNotification,
		From:   base,
		To:     base,
		DryRun: true,
	})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
	assert.Contains(t, err.Error(), "replay to time must be after from time")
}

func TestReplayOutboxRequiresOpsAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 28, 13, 0, 0, 0, time.UTC)

	_, err := service.ReplayOutbox(ctx, Actor{ID: "E1001", Role: RoleEmployee}, ReplayOutboxRequest{
		Kind:   "notification",
		From:   base,
		To:     base.Add(time.Minute),
		DryRun: true,
	})

	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
}

type replayOutboxFixture struct {
	OutboxID     string
	EventType    string
	Payload      string
	Status       string
	CreatedAt    time.Time
	Attempts     int
	RetryCount   int
	LastError    string
	DeadLetterAt time.Time
}

type replayOutboxState struct {
	publishStatus string
	lastError     string
	attempts      int
	retryCount    int
	deadLetterAt  sql.NullTime
	availableNow  bool
}

func insertReplayOutbox(t *testing.T, service *Service, ctx context.Context, fixture replayOutboxFixture) {
	t.Helper()
	if fixture.CreatedAt.IsZero() {
		fixture.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(fixture.Payload) == "" {
		fixture.Payload = "{}"
	}
	_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, attempts, retry_count, last_error, dead_letter_at, available_at, created_at)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,now() + interval '1 hour',$10)`,
		fixture.OutboxID, fixture.OutboxID+"-aggregate", fixture.EventType, fixture.Payload, fixture.Status, fixture.Attempts,
		fixture.RetryCount, fixture.LastError, nullableTime(fixture.DeadLetterAt), fixture.CreatedAt)
	require.NoError(t, err)
}

func loadReplayOutboxState(t *testing.T, service *Service, ctx context.Context, outboxID string) replayOutboxState {
	t.Helper()
	var state replayOutboxState
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, last_error, attempts, retry_count, dead_letter_at, available_at <= now()
		FROM outbox_events WHERE outbox_id = $1`, outboxID).
		Scan(&state.publishStatus, &state.lastError, &state.attempts, &state.retryCount, &state.deadLetterAt, &state.availableNow))
	return state
}

func assertReplayAuditCount(t *testing.T, service *Service, ctx context.Context, want int) {
	t.Helper()
	var got int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'outbox.replay'`).Scan(&got))
	assert.Equal(t, want, got)
}

func loadReplayAuditMetadata(t *testing.T, service *Service, ctx context.Context, auditID string) map[string]interface{} {
	t.Helper()
	var raw string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT metadata::text FROM audit_logs WHERE audit_id = $1`, auditID).Scan(&raw))
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &metadata))
	return metadata
}
