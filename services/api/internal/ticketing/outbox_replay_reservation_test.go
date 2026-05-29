package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateReplayOutboxRequestAcceptsReservationCompensationAlias(t *testing.T) {
	base := time.Date(2026, 5, 29, 11, 0, 0, 0, time.UTC)

	req, err := ValidateReplayOutboxRequest(ReplayOutboxRequest{
		Kind:       outboxWorkerKindReservationCompensation,
		From:       base,
		To:         base.Add(time.Minute),
		EventTypes: []string{" reservation.compensation.release_required.v2 "},
		DryRun:     true,
	})

	require.NoError(t, err)
	assert.Equal(t, outboxWorkerKindReservationCompensation, req.Kind)
	assert.Equal(t, []string{"reservation.compensation.release_required.v2"}, req.EventTypes)

	_, err = ValidateReplayOutboxRequest(ReplayOutboxRequest{
		Kind:       outboxWorkerKindReservationCompensation,
		From:       base,
		To:         base.Add(time.Minute),
		EventTypes: []string{"notification.requested.v2"},
		DryRun:     true,
	})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
	assert.Contains(t, err.Error(), "does not belong to worker kind")
}

func TestValidateReplayOutboxRequestRejectsUnsafeReservationCompensationEventFilters(t *testing.T) {
	base := time.Date(2026, 5, 29, 11, 0, 0, 0, time.UTC)
	tests := []string{
		"reservation.compensation.unknown.v2",
		"reservation.compensation.e1001@cets.local.v2",
		"reservation.compensation.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJyZXBsYXkifQ.signature",
	}

	for _, eventType := range tests {
		t.Run(eventType, func(t *testing.T) {
			_, err := ValidateReplayOutboxRequest(ReplayOutboxRequest{
				Kind:       outboxWorkerKindReservationCompensation,
				From:       base,
				To:         base.Add(time.Minute),
				EventTypes: []string{eventType},
				DryRun:     true,
			})

			require.Error(t, err)
			assert.Equal(t, 400, ErrorStatus(err))
			assert.Contains(t, err.Error(), "replay event type is invalid")
		})
	}
}

func TestReplayOutboxDryRunReservationCompensationAliasSelectsReservationRows(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 29, 11, 30, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-reservation-compensation",
		EventType:    "reservation.compensation.release_required.v2",
		Status:       "dead_letter",
		CreatedAt:    base,
		RetryCount:   2,
		LastError:    "reservation release failed",
		DeadLetterAt: base.Add(time.Minute),
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:  "out-replay-notification-for-alias",
		EventType: "notification.requested.v2",
		Status:    "dead_letter",
		CreatedAt: base,
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:  "out-replay-reservation-pending",
		EventType: "reservation.compensation.release_required.v2",
		Status:    "pending",
		CreatedAt: base,
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:   outboxWorkerKindReservationCompensation,
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Minute),
		DryRun: true,
	})

	require.NoError(t, err)
	assert.Equal(t, outboxWorkerKindReservationCompensation, result.Kind)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.AffectedCount)
	assert.Nil(t, result.EnqueuedCount)
	assert.Empty(t, result.AuditID)
	assert.Equal(t, "dead_letter", loadReplayOutboxState(t, service, ctx, "out-replay-reservation-compensation").publishStatus)
	assert.Equal(t, "dead_letter", loadReplayOutboxState(t, service, ctx, "out-replay-notification-for-alias").publishStatus)
	assert.Equal(t, "pending", loadReplayOutboxState(t, service, ctx, "out-replay-reservation-pending").publishStatus)
	assertReplayAuditCount(t, service, ctx, 0)
}

func TestReplayOutboxApplyReservationCompensationAliasEnqueuesAndAudits(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:     "out-replay-reservation-apply",
		EventType:    "reservation.compensation.release_required.v2",
		Status:       "dead_letter",
		CreatedAt:    base,
		Attempts:     3,
		RetryCount:   3,
		LastError:    "reservation release failed",
		DeadLetterAt: base.Add(time.Minute),
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:  "out-replay-reservation-apply-notification",
		EventType: "notification.requested.v2",
		Status:    "dead_letter",
		CreatedAt: base,
	})
	insertReplayOutbox(t, service, ctx, replayOutboxFixture{
		OutboxID:  "out-replay-reservation-apply-pending",
		EventType: "reservation.compensation.release_required.v2",
		Status:    "pending",
		CreatedAt: base,
	})

	result, err := service.ReplayOutbox(ctx, Actor{ID: "hr-replay-1", Role: RoleHRAdmin}, ReplayOutboxRequest{
		Kind:       outboxWorkerKindReservationCompensation,
		From:       base.Add(-time.Minute),
		To:         base.Add(time.Minute),
		EventTypes: []string{"reservation.compensation.release_required.v2"},
		DryRun:     false,
	})

	require.NoError(t, err)
	assert.Equal(t, outboxWorkerKindReservationCompensation, result.Kind)
	assert.False(t, result.DryRun)
	assert.Equal(t, 1, result.AffectedCount)
	require.NotNil(t, result.EnqueuedCount)
	assert.Equal(t, 1, *result.EnqueuedCount)
	require.NotEmpty(t, result.AuditID)

	replayed := loadReplayOutboxState(t, service, ctx, "out-replay-reservation-apply")
	assert.Equal(t, "pending", replayed.publishStatus)
	assert.Empty(t, replayed.lastError)
	assert.Equal(t, 0, replayed.attempts)
	assert.Equal(t, 0, replayed.retryCount)
	assert.False(t, replayed.deadLetterAt.Valid)
	assert.True(t, replayed.availableNow)
	assert.Equal(t, "dead_letter", loadReplayOutboxState(t, service, ctx, "out-replay-reservation-apply-notification").publishStatus)
	assert.Equal(t, "pending", loadReplayOutboxState(t, service, ctx, "out-replay-reservation-apply-pending").publishStatus)

	audit := loadReplayAuditMetadata(t, service, ctx, result.AuditID)
	assert.Equal(t, outboxWorkerKindReservationCompensation, audit["kind"])
	assert.Equal(t, []interface{}{"reservation.compensation.release_required.v2"}, audit["event_types"])
	assert.Equal(t, float64(1), audit["affected_count"])
	assert.Equal(t, float64(1), audit["enqueued_count"])
	var actorID, role, action, entityType, entityID string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT actor_id, role, action, entity_type, entity_id
		FROM audit_logs WHERE audit_id = $1`, result.AuditID).
		Scan(&actorID, &role, &action, &entityType, &entityID))
	assert.Equal(t, "hr-replay-1", actorID)
	assert.Equal(t, RoleHRAdmin, role)
	assert.Equal(t, outboxReplayAction, action)
	assert.Equal(t, "outbox_queue", entityType)
	assert.Equal(t, outboxWorkerKindReservationCompensation, entityID)
}
