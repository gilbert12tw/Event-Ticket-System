package ticketing

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessOutboxOnceRecordsRetryStateOnFailedNotification(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-email-retry-state", "pending", 0)
	sender := &recordingNotificationSender{err: errors.New("550 rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	state := loadOutboxFailureState(t, service, ctx, "out-email-retry-state")
	assert.Equal(t, "pending", state.publishStatus)
	assert.Equal(t, 1, state.attempts)
	assert.Equal(t, 1, state.retryCount)
	assert.False(t, state.deadLetterAt.Valid)
	assert.True(t, state.availableInFuture)
	assertSanitizedNotificationError(t, state.lastError)
}

func TestProcessOutboxOnceAuditsRetryStateOnFailedNotification(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-email-retry-audit", "pending", 0)
	sender := &recordingNotificationSender{err: errors.New("550 rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.retry_scheduled'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, "out-email-retry-audit")
	assert.Equal(t, "booking.confirmed", audit["event_type"])
	assert.Equal(t, "notification", audit["worker_kind"])
	assert.Equal(t, float64(1), audit["retry_count"])
	assert.Equal(t, float64(1), audit["schema_version"])
	assert.Equal(t, "retryable_failure", audit["reason"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "e1001@cets.local", "550 rejected")
}

func TestProcessOutboxOnceRecordsDeadLetterStateOnFailedNotification(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-email-dead-letter-state", "pending", 2)
	sender := &recordingNotificationSender{err: errors.New("550 rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	state := loadOutboxFailureState(t, service, ctx, "out-email-dead-letter-state")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, 3, state.attempts)
	assert.Equal(t, 3, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
	assertSanitizedNotificationError(t, state.lastError)
}

func TestProcessOutboxOnceAuditsDeadLetterStateOnFailedNotification(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-email-dead-letter-audit", "pending", 2)
	sender := &recordingNotificationSender{err: errors.New("550 rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.dead_letter'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, "out-email-dead-letter-audit")
	assert.Equal(t, "booking.confirmed", audit["event_type"])
	assert.Equal(t, "notification", audit["worker_kind"])
	assert.Equal(t, float64(3), audit["retry_count"])
	assert.Equal(t, float64(1), audit["schema_version"])
	assert.Equal(t, "retry_exhausted", audit["reason"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "e1001@cets.local", "550 rejected")
}

func TestProcessOutboxOnceDeadLettersNonObjectPayload(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, attempts, available_at)
		VALUES ('out-bad-payload-shape','out-bad-payload-shape-aggregate','booking.confirmed','["E1001"]'::jsonb,'pending',2,now() - interval '1 minute')`)
	require.NoError(t, err)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerDeliveryCount(t, service, ctx, "out-bad-payload-shape", 0)
	state := loadOutboxFailureState(t, service, ctx, "out-bad-payload-shape")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, 3, state.attempts)
	assert.Equal(t, 3, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
	assert.Equal(t, "invalid outbox payload", state.lastError)
	assert.NotContains(t, state.lastError, "E1001")
}

func TestProcessOutboxOnceRecoversPanickingNotificationSender(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-panic-sender", "pending", 0)
	sender := panickingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-panic-sender")
	assert.Equal(t, deliveryStatusFailed, statuses["email"])
	state := loadOutboxFailureState(t, service, ctx, "out-panic-sender")
	assert.Equal(t, "pending", state.publishStatus)
	assert.Equal(t, 1, state.attempts)
	assert.Equal(t, 1, state.retryCount)
	assert.Equal(t, "notification sender panic", state.lastError)
	assert.NotContains(t, state.lastError, "E1001")
	assert.NotContains(t, state.lastError, "@")
}

func TestProcessOutboxOnceHonorsZeroRetryMax(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-zero-retry-max", "pending", 0)
	sender := &recordingNotificationSender{err: errors.New("smtp unavailable")}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender: sender,
		RetryPolicy: &OutboxRetryPolicy{
			MaxAttempts: 0,
			BackoffBase: 500 * time.Millisecond,
			BackoffMax:  2 * time.Second,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	state := loadOutboxFailureState(t, service, ctx, "out-zero-retry-max")
	assert.Equal(t, "dead_letter", state.publishStatus)
	assert.Equal(t, 1, state.attempts)
	assert.Equal(t, 1, state.retryCount)
	assert.True(t, state.deadLetterAt.Valid)
}

func TestProcessOutboxOnceAuditsZeroRetryMaxDeadLetter(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-zero-retry-audit", "pending", 0)
	sender := &recordingNotificationSender{err: errors.New("smtp unavailable for e1001@cets.local and E1001")}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender: sender,
		RetryPolicy: &OutboxRetryPolicy{
			MaxAttempts: 0,
			BackoffBase: 500 * time.Millisecond,
			BackoffMax:  2 * time.Second,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'outbox.dead_letter'
			AND entity_type = 'outbox_event'
			AND entity_id = $1`, "out-zero-retry-audit")
	assert.Equal(t, "booking.confirmed", audit["event_type"])
	assert.Equal(t, "notification", audit["worker_kind"])
	assert.Equal(t, float64(1), audit["retry_count"])
	assert.Equal(t, float64(1), audit["schema_version"])
	assert.Equal(t, "retry_exhausted", audit["reason"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "e1001@cets.local", "smtp unavailable")
}

func TestProcessOutboxOnceCapsConfiguredBackoff(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-capped-backoff", "pending", 2)
	sender := &recordingNotificationSender{err: errors.New("smtp unavailable")}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender: sender,
		RetryPolicy: &OutboxRetryPolicy{
			MaxAttempts: 5,
			BackoffBase: time.Second,
			BackoffMax:  2 * time.Second,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	state := loadOutboxFailureState(t, service, ctx, "out-capped-backoff")
	assert.Equal(t, "pending", state.publishStatus)
	assert.Equal(t, 3, state.retryCount)
	assert.True(t, state.availableInFuture)
	assert.True(t, state.availableWithinCap, "available_at exceeded configured backoff cap")
}

type outboxFailureState struct {
	publishStatus      string
	attempts           int
	retryCount         int
	lastError          string
	deadLetterAt       sql.NullTime
	availableInFuture  bool
	availableWithinCap bool
}

func loadOutboxFailureState(t *testing.T, service *Service, ctx context.Context, outboxID string) outboxFailureState {
	t.Helper()
	var state outboxFailureState
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, attempts, retry_count,
			last_error, dead_letter_at, available_at > now(), available_at <= now() + interval '3 seconds'
		FROM outbox_events WHERE outbox_id = $1`, outboxID).
		Scan(&state.publishStatus, &state.attempts, &state.retryCount, &state.lastError,
			&state.deadLetterAt, &state.availableInFuture, &state.availableWithinCap))
	return state
}

type panickingNotificationSender struct{}

func (panickingNotificationSender) Send(context.Context, DeliveryMessage) error {
	panic("panic for E1001 at e1001@cets.local")
}
