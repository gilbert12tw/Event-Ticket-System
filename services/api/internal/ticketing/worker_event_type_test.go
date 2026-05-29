package ticketing

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"event-ticket-system/internal/traceid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessOutboxOnceDeadLettersUnknownEventTypeWithoutSideEffect(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx := traceid.WithContext(context.Background(), "trace-unknown-event-type-1")
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJldmVudCJ9.signature"
	insertWorkerOutboxPayload(t, service, ctx, "out-unknown-event-type", unsafeEventType, "pending", 0, `{
		"employee_id":"E1001",
		"event_title":"Confidential Unknown Event",
		"recipient":"e1001@cets.local"
	}`)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerDeliveryCount(t, service, ctx, "out-unknown-event-type", 0)
	state := loadOutboxFailureState(t, service, ctx, "out-unknown-event-type")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, 1, state.attempts)
	assert.Equal(t, 1, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
	assert.Contains(t, state.lastError, "reason=unknown_event_type")
	assert.Contains(t, state.lastError, "event_type=unknown")
	assert.NotContains(t, state.lastError, unsafeEventType)
	assert.NotContains(t, state.lastError, "E1001")
	assert.NotContains(t, strings.ToLower(state.lastError), "e1001@cets.local")

	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.dead_letter'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, "out-unknown-event-type")
	assert.Equal(t, "unknown", audit["event_type"])
	assert.Equal(t, outboxWorkerKindUnknown, audit["worker_kind"])
	assert.Equal(t, unknownOutboxEventTypeReason, audit["reason"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "e1001@cets.local", "Confidential Unknown Event", "eyJhbGci", unsafeEventType)

	output := logs.String()
	assert.Contains(t, output, `"msg":"unknown outbox event type"`)
	assert.Contains(t, output, `"trace_id":"trace-unknown-event-type-1"`)
	assert.Contains(t, output, `"event_id":"out-unknown-event-type"`)
	assert.Contains(t, output, `"event_type":"unknown"`)
	assert.Contains(t, output, `"worker_kind":"unknown"`)
	assert.Contains(t, output, `"reason":"unknown_event_type"`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "Confidential Unknown Event")
	assert.NotContains(t, output, "eyJhbGci")
	assert.NotContains(t, output, unsafeEventType)
	assert.NotContains(t, output, "payload")
}
