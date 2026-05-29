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

func TestClaimOutboxEventUsesConfiguredLeaseTTL(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	seedWorkerEmployee(t, service, ctx)
	insertWorkerOutbox(t, service, ctx, "out-configured-lease", "pending", 0)
	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer rollback(ctx, tx)

	claim, err := claimOutboxEvent(ctx, tx, nil, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, "out-configured-lease", claim.outboxID)
	require.False(t, claim.leaseStartedAt.IsZero())
	require.NoError(t, tx.Commit(ctx))

	var status string
	var availableInFuture bool
	var availableWithinConfiguredTTL bool
	var leaseStarted bool
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status,
			available_at > now(),
			available_at <= now() + interval '3 seconds',
			lease_started_at IS NOT NULL
		FROM outbox_events WHERE outbox_id = $1`, "out-configured-lease").
		Scan(&status, &availableInFuture, &availableWithinConfiguredTTL, &leaseStarted))
	assert.Equal(t, "processing", status)
	assert.True(t, availableInFuture)
	assert.True(t, availableWithinConfiguredTTL)
	assert.True(t, leaseStarted)
}

func TestOutboxPublishRequiresActiveLease(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-stale-publish", "pending", 0)
	oldClaim := claimOutboxForTest(t, service, ctx, "out-stale-publish", 2*time.Second)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET available_at = now() - interval '1 minute'
		WHERE outbox_id = $1`, "out-stale-publish")
	require.NoError(t, err)
	newClaim := claimOutboxForTest(t, service, ctx, "out-stale-publish", 2*time.Second)

	err = service.markOutboxPublished(ctx, oldClaim)
	require.ErrorIs(t, err, errOutboxLeaseLost)
	var status string
	var leaseStartedAt time.Time
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, lease_started_at
		FROM outbox_events WHERE outbox_id = $1`, "out-stale-publish").Scan(&status, &leaseStartedAt))
	assert.Equal(t, "processing", status)
	assert.True(t, leaseStartedAt.Equal(newClaim.leaseStartedAt))

	require.NoError(t, service.markOutboxPublished(ctx, newClaim))
	assertWorkerOutboxStatus(t, service, ctx, "out-stale-publish", "published", 2)
}

func TestOutboxFailureRequiresActiveLease(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-stale-failure", "pending", 0)
	oldClaim := claimOutboxForTest(t, service, ctx, "out-stale-failure", 2*time.Second)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET available_at = now() - interval '1 minute'
		WHERE outbox_id = $1`, "out-stale-failure")
	require.NoError(t, err)
	newClaim := claimOutboxForTest(t, service, ctx, "out-stale-failure", 2*time.Second)
	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer rollback(ctx, tx)

	err = updateOutboxFailureInTx(ctx, tx, oldClaim, outboxRetryPolicyFromOptions(OutboxProcessorOptions{MaxAttempts: 3}), "dead_letter", "stale failure")
	require.ErrorIs(t, err, errOutboxLeaseLost)
	var status string
	var leaseStartedAt time.Time
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, lease_started_at
		FROM outbox_events WHERE outbox_id = $1`, "out-stale-failure").Scan(&status, &leaseStartedAt))
	assert.Equal(t, "processing", status)
	assert.True(t, leaseStartedAt.Equal(newClaim.leaseStartedAt))
}

func TestOutboxReleaseRequiresActiveLease(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-stale-release", "pending", 0)
	oldClaim := claimOutboxForTest(t, service, ctx, "out-stale-release", 2*time.Second)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET available_at = now() - interval '1 minute'
		WHERE outbox_id = $1`, "out-stale-release")
	require.NoError(t, err)
	newClaim := claimOutboxForTest(t, service, ctx, "out-stale-release", 2*time.Second)

	require.NoError(t, service.releaseOutboxLease(ctx, oldClaim))
	var status string
	var availableInFuture bool
	var leaseStartedAt time.Time
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status,
			available_at > now(),
			lease_started_at
		FROM outbox_events WHERE outbox_id = $1`, "out-stale-release").
		Scan(&status, &availableInFuture, &leaseStartedAt))
	assert.Equal(t, "processing", status)
	assert.True(t, availableInFuture)
	assert.True(t, leaseStartedAt.Equal(newClaim.leaseStartedAt))

	require.NoError(t, service.releaseOutboxLease(ctx, newClaim))
	var pending bool
	var availableNow bool
	var leaseCleared bool
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status = 'pending',
			available_at <= now(),
			lease_started_at IS NULL
		FROM outbox_events WHERE outbox_id = $1`, "out-stale-release").
		Scan(&pending, &availableNow, &leaseCleared))
	assert.True(t, pending)
	assert.True(t, availableNow)
	assert.True(t, leaseCleared)
}

func TestProcessOutboxOnceDoesNotClaimUnexpiredProcessingLease(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-active-lease", "processing", 1)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET available_at = now() + interval '1 minute',
			lease_started_at = now()
		WHERE outbox_id = $1`, "out-active-lease")
	require.NoError(t, err)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerDeliveryCount(t, service, ctx, "out-active-lease", 0)
	var status string
	var attempts int
	var availableInFuture bool
	var leaseStarted bool
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status,
			attempts,
			available_at > now(),
			lease_started_at IS NOT NULL
		FROM outbox_events WHERE outbox_id = $1`, "out-active-lease").
		Scan(&status, &attempts, &availableInFuture, &leaseStarted))
	assert.Equal(t, "processing", status)
	assert.Equal(t, 1, attempts)
	assert.True(t, availableInFuture)
	assert.True(t, leaseStarted)
}

func TestProcessOutboxOnceDoesNotResendEmailAfterExpiredLeaseCrash(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-crash-recovered-email", "processing", 1)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET available_at = now() - interval '1 minute',
			lease_started_at = now() - interval '2 minutes'
		WHERE outbox_id = $1`, "out-crash-recovered-email")
	require.NoError(t, err)
	insertWorkerDelivery(t, service, ctx, "del-crash-email", "out-crash-recovered-email", "email", deliveryStatusSent)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		MaxAttempts: 3,
		LeaseTTL:    2 * time.Second,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-crash-recovered-email")
	assert.Equal(t, deliveryStatusSent, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-crash-recovered-email", "published", 2)
	var leaseCleared bool
	require.NoError(t, service.db.QueryRow(ctx, `SELECT lease_started_at IS NULL
		FROM outbox_events WHERE outbox_id = $1`, "out-crash-recovered-email").Scan(&leaseCleared))
	assert.True(t, leaseCleared)
}

func TestProcessOutboxOnceReleasesLeaseAfterContextCancel(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
	insertWorkerOutbox(t, service, ctx, "out-cancel-release", "pending", 0)
	sender := &idempotentCancelingNotificationSender{cancel: cancel}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		MaxAttempts: 3,
		LeaseTTL:    2 * time.Second,
	})

	require.Error(t, err)
	assert.Equal(t, 1, processed)
	var status string
	var availableInFuture bool
	var leaseStarted bool
	require.NoError(t, service.db.QueryRow(context.Background(), `SELECT publish_status,
			available_at > now(),
			lease_started_at IS NOT NULL
		FROM outbox_events WHERE outbox_id = $1`, "out-cancel-release").
		Scan(&status, &availableInFuture, &leaseStarted))
	assert.Equal(t, "pending", status)
	assert.False(t, availableInFuture)
	assert.False(t, leaseStarted)

	runCtx := context.Background()
	_, err = service.ProcessOutboxOnce(runCtx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, sender.calls)
	assert.Equal(t, 1, sender.providerSent)
	statuses := workerDeliveryStatuses(t, service, runCtx, "out-cancel-release")
	assert.Equal(t, deliveryStatusDeadLetter, statuses["email"])
	assertWorkerOutboxStatus(t, service, runCtx, "out-cancel-release", "dead_letter", 2)
	var lastError string
	require.NoError(t, service.db.QueryRow(runCtx, `SELECT last_error FROM outbox_events WHERE outbox_id = $1`, "out-cancel-release").Scan(&lastError))
	assert.Contains(t, lastError, "ambiguous")
}

func TestLogOutboxLeaseReleaseFailureRedactsUnsafeTelemetry(t *testing.T) {
	var logs bytes.Buffer
	service := &Service{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	ctx := traceid.WithContext(context.Background(), "trace-lease-release-1")
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJsZWFzZSJ9.signature"

	service.logOutboxLeaseReleaseFailure(ctx, outboxClaim{
		outboxID:  "out-lease-release-log",
		eventType: unsafeEventType,
	}, errors.New("release failed for e1001@cets.local and E1001 token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJlcnJvciJ9.signature message_body=Private lease body for E1001"))

	output := logs.String()
	assert.Contains(t, output, `"msg":"outbox lease release failed"`)
	assert.Contains(t, output, `"trace_id":"trace-lease-release-1"`)
	assert.Contains(t, output, `"event_id":"out-lease-release-log"`)
	assert.Contains(t, output, `"event_type":"unknown"`)
	assert.Contains(t, output, `"worker_kind":"unknown"`)
	assert.Contains(t, output, `"error":"release failed for [redacted email] and [redacted employee] token [redacted token] message_body=[redacted email body]"`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "eyJhbGci")
	assert.NotContains(t, output, "Private lease body")
	assert.NotContains(t, output, unsafeEventType)
}

func claimOutboxForTest(t *testing.T, service *Service, ctx context.Context, outboxID string, leaseTTL time.Duration) outboxClaim {
	t.Helper()
	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer rollback(ctx, tx)
	claim, err := claimOutboxEvent(ctx, tx, nil, leaseTTL)
	require.NoError(t, err)
	assert.Equal(t, outboxID, claim.outboxID)
	require.NoError(t, tx.Commit(ctx))
	return claim
}
