package ticketing

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/traceid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessOutboxOnceLogsTraceSafeDeadLetterWorkerAttempt(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx, cancel := context.WithTimeout(traceid.WithContext(context.Background(), "trace-worker-dead-letter-1"), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
	insertWorkerOutboxPayload(t, service, ctx, "out-worker-dead-letter-log", "booking.confirmed", "pending", 2, `{
		"employee_id":"E1001",
		"event_id":"evt_worker_dead_letter_log",
		"event_title":"Confidential Dead Letter",
		"recipient":"e1001@cets.local"
	}`)
	sender := &recordingNotificationSender{err: errors.New("smtp rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	output := logs.String()
	assert.Contains(t, output, `"msg":"outbox worker attempt"`)
	assert.Contains(t, output, `"trace_id":"trace-worker-dead-letter-1"`)
	assert.Contains(t, output, `"event_id":"out-worker-dead-letter-log"`)
	assert.Contains(t, output, `"event_type":"booking.confirmed"`)
	assert.Contains(t, output, `"worker_kind":"notification"`)
	assert.Contains(t, output, `"retry_count":3`)
	assert.Contains(t, output, `"outcome":"dead_letter"`)
	assert.Contains(t, output, `"latency_ms":`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "Confidential Dead Letter")
	assert.NotContains(t, output, "payload")
}

func TestProcessOutboxOnceLogsTraceSafeRetryWorkerAttempt(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx, cancel := context.WithTimeout(traceid.WithContext(context.Background(), "trace-worker-retry-1"), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
	insertWorkerOutboxPayload(t, service, ctx, "out-worker-retry-log", "booking.confirmed", "pending", 0, `{
		"employee_id":"E1001",
		"event_id":"evt_worker_retry_log",
		"event_title":"Confidential Retry",
		"recipient":"e1001@cets.local"
	}`)
	sender := &recordingNotificationSender{err: errors.New("smtp rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	output := logs.String()
	assert.Contains(t, output, `"msg":"outbox worker attempt"`)
	assert.Contains(t, output, `"trace_id":"trace-worker-retry-1"`)
	assert.Contains(t, output, `"event_id":"out-worker-retry-log"`)
	assert.Contains(t, output, `"event_type":"booking.confirmed"`)
	assert.Contains(t, output, `"worker_kind":"notification"`)
	assert.Contains(t, output, `"retry_count":1`)
	assert.Contains(t, output, `"outcome":"retry"`)
	assert.Contains(t, output, `"latency_ms":`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "Confidential Retry")
	assert.NotContains(t, output, "payload")
}
