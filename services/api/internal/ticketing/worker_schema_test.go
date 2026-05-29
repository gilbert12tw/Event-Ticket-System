package ticketing

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessOutboxOnceDeadLettersUnknownSchemaVersionWithoutSideEffect(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx := context.Background()
	insertWorkerOutboxPayload(t, service, ctx, "out-unknown-schema", "booking.confirmed", "pending", 0, `{
		"employee_id":"E1001",
		"event_title":"Confidential Schema Drift",
		"recipient":"e1001@cets.local"
	}`)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events SET schema_version = 99 WHERE outbox_id = $1`, "out-unknown-schema")
	require.NoError(t, err)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerDeliveryCount(t, service, ctx, "out-unknown-schema", 0)
	state := loadOutboxFailureState(t, service, ctx, "out-unknown-schema")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, 1, state.attempts)
	assert.Equal(t, 1, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
	assert.Contains(t, state.lastError, "reason=unknown_schema_version")
	assert.Contains(t, state.lastError, "unsupported outbox schema_version=99")
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.dead_letter'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, "out-unknown-schema")
	assert.Equal(t, "booking.confirmed", audit["event_type"])
	assert.Equal(t, "notification", audit["worker_kind"])
	assert.Equal(t, float64(1), audit["retry_count"])
	assert.Equal(t, float64(99), audit["schema_version"])
	assert.Equal(t, unknownOutboxSchemaVersionReason, audit["reason"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "e1001@cets.local", "Confidential Schema Drift")
	output := logs.String()
	assert.Contains(t, output, `"level":"WARN"`)
	assert.Contains(t, output, `"msg":"unknown outbox schema version"`)
	assert.Contains(t, output, `"event_id":"out-unknown-schema"`)
	assert.Contains(t, output, `"event_type":"booking.confirmed"`)
	assert.Contains(t, output, `"worker_kind":"notification"`)
	assert.Contains(t, output, `"schema_version":99`)
	assert.Contains(t, output, `"reason":"unknown_schema_version"`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "Confidential Schema Drift")
	assert.NotContains(t, output, "payload")
}

func TestProcessOutboxOnceRedactsUnsafeEventTypeInFailureAuditAndLogs(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx := context.Background()
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJF1001xyz.XyZ_12345678"
	insertWorkerOutboxPayload(t, service, ctx, "out-unsafe-event-type", unsafeEventType, "pending", 0, `{
		"employee_id":"E1001",
		"event_title":"Confidential Unsafe Event Type",
		"recipient":"e1001@cets.local"
	}`)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events SET schema_version = 99 WHERE outbox_id = $1`, "out-unsafe-event-type")
	require.NoError(t, err)

	processed, err := service.ProcessOutboxOnce(ctx, &recordingNotificationSender{}, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.dead_letter'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, "out-unsafe-event-type")
	assert.Equal(t, "unknown", audit["event_type"])
	assert.Equal(t, outboxWorkerKindUnknown, audit["worker_kind"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "e1001@cets.local", "eyJhbGci", unsafeEventType)
	output := logs.String()
	assert.Contains(t, output, `"event_type":"unknown"`)
	assert.Contains(t, output, `"worker_kind":"unknown"`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "eyJhbGci")
	assert.NotContains(t, output, unsafeEventType)
}

func TestProcessOutboxOnceStillProcessesV1OutboxRows(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-v1-schema", "pending", 0)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, sender.calls)
	assertWorkerOutboxStatus(t, service, ctx, "out-v1-schema", "published", 1)
}

func TestProcessOutboxOnceDeadLettersV2PayloadWithoutEnvelope(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-v2-no-envelope", "pending", 2)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events SET schema_version = 2 WHERE outbox_id = $1`, "out-v2-no-envelope")
	require.NoError(t, err)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerDeliveryCount(t, service, ctx, "out-v2-no-envelope", 0)
	state := loadOutboxFailureState(t, service, ctx, "out-v2-no-envelope")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, 3, state.attempts)
	assert.Equal(t, 3, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
	assert.Equal(t, "invalid outbox payload", state.lastError)
	assert.NotContains(t, state.lastError, "E1001")
}

func TestProcessOutboxOnceProcessesV2EnvelopePayload(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	idempotencyKey := "notification.requested:out-v2-envelope"
	envelope := workerV2NotificationEnvelope("out-v2-envelope", idempotencyKey, "E1001")
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	insertWorkerOutboxPayload(t, service, ctx, "out-v2-envelope", "notification.requested.v2", "pending", 0, string(body))
	setWorkerOutboxV2Metadata(t, service, ctx, "out-v2-envelope", idempotencyKey, "E1001")
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, sender.messages, 1)
	assert.Equal(t, "e1001@cets.local", sender.messages[0].To)
	assert.Contains(t, sender.messages[0].Body, "Envelope Safety Drill")
	statuses := workerDeliveryStatuses(t, service, ctx, "out-v2-envelope")
	assert.Equal(t, deliveryStatusSent, statuses["email"])
	_, hasInApp := statuses["in_app"]
	assert.False(t, hasInApp)
	assertWorkerOutboxStatus(t, service, ctx, "out-v2-envelope", "published", 1)
}

func TestProcessOutboxOnceProcessesV2InAppOnlyEnvelope(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	idempotencyKey := "notification.requested:out-v2-in-app-envelope"
	envelope := workerV2NotificationEnvelope("out-v2-in-app-envelope", idempotencyKey, "E1001")
	workerV2EnvelopePayload(t, envelope)["channel"] = "in_app"
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	insertWorkerOutboxPayload(t, service, ctx, "out-v2-in-app-envelope", "notification.requested.v2", "pending", 0, string(body))
	setWorkerOutboxV2Metadata(t, service, ctx, "out-v2-in-app-envelope", idempotencyKey, "E1001")
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-v2-in-app-envelope")
	assert.Equal(t, deliveryStatusSent, statuses["in_app"])
	_, hasEmail := statuses["email"]
	assert.False(t, hasEmail)
	assertWorkerOutboxStatus(t, service, ctx, "out-v2-in-app-envelope", "published", 1)
}

func TestProcessOutboxOnceDeadLettersUnknownV2NotificationChannel(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	idempotencyKey := "notification.requested:out-v2-unknown-channel"
	envelope := workerV2NotificationEnvelope("out-v2-unknown-channel", idempotencyKey, "E1001")
	workerV2EnvelopePayload(t, envelope)["channel"] = "sms"
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	insertWorkerOutboxPayload(t, service, ctx, "out-v2-unknown-channel", "notification.requested.v2", "pending", 2, string(body))
	setWorkerOutboxV2Metadata(t, service, ctx, "out-v2-unknown-channel", idempotencyKey, "E1001")
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerDeliveryCount(t, service, ctx, "out-v2-unknown-channel", 0)
	state := loadOutboxFailureState(t, service, ctx, "out-v2-unknown-channel")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, "invalid outbox payload", state.lastError)
}

func TestProcessOutboxOnceDeadLettersMismatchedV2EnvelopeEventType(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	envelope := map[string]interface{}{
		"event_id":        "out-v2-envelope-mismatch",
		"event_type":      "report.export.requested.v2",
		"schema_version":  2,
		"occurred_at":     "2026-05-28T10:00:00Z",
		"idempotency_key": "notification.requested:out-v2-envelope-mismatch",
		"partition_key":   "E1001",
		"payload": map[string]interface{}{
			"employee_id": "E1001",
			"event_title": "Mismatched Envelope Drill",
			"event_id":    "evt-v2-envelope-mismatch",
		},
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	insertWorkerOutboxPayload(t, service, ctx, "out-v2-envelope-mismatch", "notification.requested.v2", "pending", 2, string(body))
	setWorkerOutboxV2Metadata(t, service, ctx, "out-v2-envelope-mismatch", "notification.requested:out-v2-envelope-mismatch", "E1001")
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerDeliveryCount(t, service, ctx, "out-v2-envelope-mismatch", 0)
	state := loadOutboxFailureState(t, service, ctx, "out-v2-envelope-mismatch")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, 3, state.attempts)
	assert.Equal(t, 3, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
	assert.Equal(t, "invalid outbox payload", state.lastError)
}

func TestProcessOutboxOnceDeadLettersMismatchedV2EnvelopeDurableMetadata(t *testing.T) {
	cases := []struct {
		name     string
		outboxID string
		mutate   func(map[string]interface{})
	}{
		{
			name:     "event id",
			outboxID: "out-v2-envelope-event-id",
			mutate: func(envelope map[string]interface{}) {
				envelope["event_id"] = "out-v2-envelope-other"
			},
		},
		{
			name:     "idempotency key",
			outboxID: "out-v2-envelope-idempotency",
			mutate: func(envelope map[string]interface{}) {
				envelope["idempotency_key"] = "notification.requested:other"
			},
		},
		{
			name:     "partition key",
			outboxID: "out-v2-envelope-partition",
			mutate: func(envelope map[string]interface{}) {
				envelope["partition_key"] = "other-partition"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, ctx := newSeededWorkerTest(t)
			idempotencyKey := "notification.requested:" + tc.outboxID
			envelope := workerV2NotificationEnvelope(tc.outboxID, idempotencyKey, "E1001")
			tc.mutate(envelope)
			body, err := json.Marshal(envelope)
			require.NoError(t, err)
			insertWorkerOutboxPayload(t, service, ctx, tc.outboxID, "notification.requested.v2", "pending", 2, string(body))
			setWorkerOutboxV2Metadata(t, service, ctx, tc.outboxID, idempotencyKey, "E1001")
			sender := &recordingNotificationSender{}

			processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

			require.NoError(t, err)
			assert.Equal(t, 1, processed)
			assert.Equal(t, 0, sender.calls)
			assertWorkerDeliveryCount(t, service, ctx, tc.outboxID, 0)
			state := loadOutboxFailureState(t, service, ctx, tc.outboxID)
			assert.Equal(t, "dead_letter", state.publishStatus)
			assert.Equal(t, 3, state.attempts)
			assert.Equal(t, 3, state.retryCount)
			assert.True(t, state.deadLetterAt.Valid)
			assert.Equal(t, "invalid outbox payload", state.lastError)
		})
	}
}

func TestProcessOutboxOnceDeadLettersInvalidV2EnvelopeOccurredAt(t *testing.T) {
	cases := []struct {
		name       string
		occurredAt string
	}{
		{name: "malformed", occurredAt: "not-a-timestamp"},
		{name: "non utc offset", occurredAt: "2026-05-28T18:00:00+08:00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, ctx := newSeededWorkerTest(t)
			outboxID := "out-v2-envelope-occurred-at-" + strings.ReplaceAll(tc.name, " ", "-")
			idempotencyKey := "notification.requested:" + outboxID
			envelope := workerV2NotificationEnvelope(outboxID, idempotencyKey, "E1001")
			envelope["occurred_at"] = tc.occurredAt
			body, err := json.Marshal(envelope)
			require.NoError(t, err)
			insertWorkerOutboxPayload(t, service, ctx, outboxID, "notification.requested.v2", "pending", 2, string(body))
			setWorkerOutboxV2Metadata(t, service, ctx, outboxID, idempotencyKey, "E1001")
			sender := &recordingNotificationSender{}

			processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

			require.NoError(t, err)
			assert.Equal(t, 1, processed)
			assert.Equal(t, 0, sender.calls)
			assertWorkerDeliveryCount(t, service, ctx, outboxID, 0)
			state := loadOutboxFailureState(t, service, ctx, outboxID)
			assert.Equal(t, "dead_letter", state.publishStatus)
			assert.Equal(t, 3, state.attempts)
			assert.Equal(t, 3, state.retryCount)
			assert.True(t, state.deadLetterAt.Valid)
			assert.Equal(t, "invalid outbox payload", state.lastError)
		})
	}
}

func TestProcessOutboxOnceDeadLettersInvalidV2EnvelopePayload(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	envelope := map[string]interface{}{
		"event_id":        "out-v2-envelope-bad",
		"event_type":      "notification.requested.v2",
		"schema_version":  2,
		"occurred_at":     "2026-05-28T10:00:00Z",
		"idempotency_key": "notification.requested:out-v2-envelope-bad",
		"partition_key":   "E1001",
		"payload":         []string{"E1001"},
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	insertWorkerOutboxPayload(t, service, ctx, "out-v2-envelope-bad", "notification.requested.v2", "pending", 2, string(body))
	_, err = service.db.Exec(ctx, `UPDATE outbox_events SET schema_version = 2 WHERE outbox_id = $1`, "out-v2-envelope-bad")
	require.NoError(t, err)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	state := loadOutboxFailureState(t, service, ctx, "out-v2-envelope-bad")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, "invalid outbox payload", state.lastError)
	assert.NotContains(t, state.lastError, "E1001")
}

func workerV2NotificationEnvelope(outboxID string, idempotencyKey string, partitionKey string) map[string]interface{} {
	return map[string]interface{}{
		"event_id":        outboxID,
		"event_type":      "notification.requested.v2",
		"schema_version":  2,
		"occurred_at":     "2026-05-28T10:00:00Z",
		"idempotency_key": idempotencyKey,
		"partition_key":   partitionKey,
		"payload": map[string]interface{}{
			"notification_id":       "ntf-" + outboxID,
			"category":              "registration_confirmed",
			"channel":               "email",
			"recipient_employee_id": "E1001",
			"event_title":           "Envelope Safety Drill",
			"event_id":              "evt-" + outboxID,
			"template_key":          "registration.confirmed.v1",
			"data_refs":             map[string]interface{}{"event_id": "evt-" + outboxID},
			"requested_at":          "2026-05-28T10:00:00Z",
		},
	}
}

func workerV2EnvelopePayload(t *testing.T, envelope map[string]interface{}) map[string]interface{} {
	t.Helper()
	payload, ok := envelope["payload"].(map[string]interface{})
	require.True(t, ok)
	return payload
}

func setWorkerOutboxV2Metadata(t *testing.T, service *Service, ctx context.Context, outboxID string, idempotencyKey string, partitionKey string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET schema_version = 2,
			idempotency_key = $2,
			partition_key = $3
		WHERE outbox_id = $1`, outboxID, idempotencyKey, partitionKey)
	require.NoError(t, err)
}
