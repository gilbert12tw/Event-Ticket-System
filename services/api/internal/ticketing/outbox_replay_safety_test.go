package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayOutboxApplySkipsUnknownEventTypesWithoutFilter(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-known-notification",
		EventType:    "notification.requested.v2",
		Status:       "dead_letter",
		CreatedAt:    base,
		Attempts:     3,
		RetryCount:   3,
		LastError:    "smtp failed",
		DeadLetterAt: base.Add(time.Minute),
	})
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJyZXBsYXkifQ.signature"
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-unknown-notification",
		EventType:    unsafeEventType,
		Status:       "dead_letter",
		CreatedAt:    base,
		Attempts:     3,
		RetryCount:   3,
		LastError:    "smtp failed with unsafe event type",
		DeadLetterAt: base.Add(time.Minute),
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:   outboxWorkerKindNotification,
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Minute),
		DryRun: false,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.AffectedCount)
	require.NotNil(t, result.EnqueuedCount)
	assert.Equal(t, 1, *result.EnqueuedCount)
	known := loadReplayOutboxState(t, service, ctx, "out-replay-known-notification")
	assert.Equal(t, "pending", known.publishStatus)
	assert.Empty(t, known.lastError)
	assert.Equal(t, 0, known.attempts)
	assert.Equal(t, 0, known.retryCount)
	assert.False(t, known.deadLetterAt.Valid)
	unknown := loadReplayOutboxState(t, service, ctx, "out-replay-unknown-notification")
	assert.Equal(t, "dead_letter", unknown.publishStatus)
	assert.Equal(t, "smtp failed with unsafe event type", unknown.lastError)
	assert.Equal(t, 3, unknown.attempts)
	assert.Equal(t, 3, unknown.retryCount)
	assert.True(t, unknown.deadLetterAt.Valid)
}
