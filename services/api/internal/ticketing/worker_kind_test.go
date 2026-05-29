package ticketing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxWorkerKindForEventType(t *testing.T) {
	assert.Equal(t, outboxWorkerKindExport, outboxWorkerKindForEventType(outboxEventReportExportRequested))
	assert.Equal(t, outboxWorkerKindExport, outboxWorkerKindForEventType("report.export.requested.v2"))
	assert.Equal(t, outboxWorkerKindProjection, outboxWorkerKindForEventType(outboxEventReportingProjectionUpdateRequiredV2))
	assert.Equal(t, outboxWorkerKindCompensation, outboxWorkerKindForEventType("reservation.compensation.release_required.v2"))
	assert.Equal(t, outboxWorkerKindNotification, outboxWorkerKindForEventType("booking.confirmed"))
}

func TestNotificationWorkerKindDoesNotClaimReportExport(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	convertReportExportOutboxToLegacyEvent(t, service, ctx, export)
	insertWorkerOutbox(t, service, ctx, "out-notification-kind", "pending", 0)
	sender := &recordingNotificationSender{}
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindNotification},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, sender.calls)
	assert.Empty(t, store.keys)
	assertWorkerOutboxStatus(t, service, ctx, "out-notification-kind", "published", 1)
	assertReportExportOutboxStatusForEventType(t, service, ctx, export.ExportID, outboxEventReportExportRequested, "pending", 0)
}

func TestNotificationWorkerKindDoesNotClaimReportExportV2(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	sender := &recordingNotificationSender{}
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindNotification},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	assert.Equal(t, 0, sender.calls)
	assert.Empty(t, store.keys)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "pending", 0)
}

func TestExportWorkerKindDoesNotClaimNotificationBacklog(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	insertWorkerOutbox(t, service, ctx, "out-notification-backlog", "pending", 0)
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	sender := &recordingNotificationSender{}
	store := &recordingReportStore{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindExport},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	require.Len(t, store.keys, 1)
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-notification-backlog", "pending", 0)
}

func TestProjectionWorkerKindDoesNotClaimNotificationBacklog(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	insertWorkerOutbox(t, service, ctx, "out-notification-backlog-projection", "pending", 0)
	insertWorkerOutboxPayload(t, service, ctx, "out-projection-kind", outboxEventReportingProjectionUpdateRequiredV2, "pending", 0, `{"model":"event_participation_summary"}`)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindProjection},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerOutboxStatus(t, service, ctx, "out-projection-kind", "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-notification-backlog-projection", "pending", 0)
}

func TestProjectionWorkerDrainsWhileNotificationSendBlocks(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-blocking-notification", "pending", 0)
	insertWorkerOutboxPayload(t, service, ctx, "out-live-projection", outboxEventReportingProjectionUpdateRequiredV2, "pending", 0, `{"model":"event_participation_summary"}`)
	sender := newBlockingNotificationSender()
	notificationDone := make(chan workerProcessResult, 1)
	t.Cleanup(sender.release)

	go func() {
		processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
			Sender:      sender,
			MaxAttempts: 3,
			WorkerKinds: []string{outboxWorkerKindNotification},
		})
		notificationDone <- workerProcessResult{processed: processed, err: err}
	}()

	require.Eventually(t, sender.started, time.Second, time.Millisecond)
	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindProjection},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assertWorkerOutboxStatus(t, service, ctx, "out-live-projection", "published", 1)
	sender.release()
	result := waitWorkerProcessResult(t, notificationDone)
	require.NoError(t, result.err)
	assert.Equal(t, 1, result.processed)
	assertWorkerOutboxStatus(t, service, ctx, "out-blocking-notification", "published", 1)
}

func TestCompensationWorkerDrainsWhileNotificationSendBlocks(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-blocking-notification-compensation", "pending", 0)
	insertWorkerOutboxPayload(t, service, ctx, "out-live-compensation", "reservation.compensation.release_required.v2", "pending", 0, `{"reservation_id":"res_1"}`)
	sender := newBlockingNotificationSender()
	notificationDone := make(chan workerProcessResult, 1)
	t.Cleanup(sender.release)

	go func() {
		processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
			Sender:      sender,
			MaxAttempts: 3,
			WorkerKinds: []string{outboxWorkerKindNotification},
		})
		notificationDone <- workerProcessResult{processed: processed, err: err}
	}()

	require.Eventually(t, sender.started, time.Second, time.Millisecond)
	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindCompensation},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assertWorkerOutboxStatus(t, service, ctx, "out-live-compensation", "published", 1)
	sender.release()
	result := waitWorkerProcessResult(t, notificationDone)
	require.NoError(t, result.err)
	assert.Equal(t, 1, result.processed)
	assertWorkerOutboxStatus(t, service, ctx, "out-blocking-notification-compensation", "published", 1)
}

func TestExportWorkerDrainsWhileNotificationSendBlocks(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-blocking-notification-export", "pending", 0)
	export, err := service.CreateReportExport(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	store := &recordingReportStore{}
	sender := newBlockingNotificationSender()
	notificationDone := make(chan workerProcessResult, 1)
	t.Cleanup(sender.release)

	go func() {
		processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
			Sender:      sender,
			MaxAttempts: 3,
			WorkerKinds: []string{outboxWorkerKindNotification},
		})
		notificationDone <- workerProcessResult{processed: processed, err: err}
	}()

	require.Eventually(t, sender.started, time.Second, time.Millisecond)
	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		ReportStore: store,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindExport},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, store.keys, 1)
	assert.Equal(t, export.ObjectKey, store.keys[0])
	assertReportExportState(t, service, ctx, export.ExportID, ReportExportStatusReady, true)
	assertReportExportOutboxStatus(t, service, ctx, export.ExportID, "published", 1)
	assertWorkerProcessStillBlocked(t, notificationDone)
	sender.release()
	result := waitWorkerProcessResult(t, notificationDone)
	require.NoError(t, result.err)
	assert.Equal(t, 1, result.processed)
	assertWorkerOutboxStatus(t, service, ctx, "out-blocking-notification-export", "published", 1)
}

func TestCompensationWorkerKindDoesNotClaimNotificationBacklog(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	insertWorkerOutbox(t, service, ctx, "out-notification-backlog-compensation", "pending", 0)
	insertWorkerOutboxPayload(t, service, ctx, "out-compensation-kind", "reservation.compensation.release_required.v2", "pending", 0, `{"reservation_id":"res_1"}`)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		MaxAttempts: 3,
		WorkerKinds: []string{outboxWorkerKindCompensation},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerOutboxStatus(t, service, ctx, "out-compensation-kind", "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-notification-backlog-compensation", "pending", 0)
}

func TestNonNotificationWorkerKindsRecoverExpiredProcessingLease(t *testing.T) {
	cases := []struct {
		name      string
		kind      string
		eventType string
		payload   string
		outboxID  string
	}{
		{
			name:      "projection",
			kind:      outboxWorkerKindProjection,
			eventType: outboxEventReportingProjectionUpdateRequiredV2,
			payload:   `{"model":"event_participation_summary"}`,
			outboxID:  "out-expired-projection-lease",
		},
		{
			name:      "compensation",
			kind:      outboxWorkerKindCompensation,
			eventType: "reservation.compensation.release_required.v2",
			payload:   `{"reservation_id":"res_lease_recovered"}`,
			outboxID:  "out-expired-compensation-lease",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, cleanup := newIntegrationService(t)
			defer cleanup()
			ctx := context.Background()
			insertWorkerOutboxPayload(t, service, ctx, tc.outboxID, tc.eventType, "processing", 1, tc.payload)
			_, err := service.db.Exec(ctx, `UPDATE outbox_events
				SET available_at = now() - interval '1 minute',
					lease_started_at = now() - interval '2 minutes'
				WHERE outbox_id = $1`, tc.outboxID)
			require.NoError(t, err)
			sender := &recordingNotificationSender{}

			processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
				Sender:      sender,
				MaxAttempts: 3,
				WorkerKinds: []string{tc.kind},
				LeaseTTL:    2 * time.Second,
			})

			require.NoError(t, err)
			assert.Equal(t, 1, processed)
			assert.Equal(t, 0, sender.calls)
			assertWorkerOutboxStatus(t, service, ctx, tc.outboxID, "published", 2)
			var leaseCleared bool
			require.NoError(t, service.db.QueryRow(ctx, `SELECT lease_started_at IS NULL
				FROM outbox_events WHERE outbox_id = $1`, tc.outboxID).Scan(&leaseCleared))
			assert.True(t, leaseCleared)
		})
	}
}

type workerProcessResult struct {
	processed int
	err       error
}

type blockingNotificationSender struct {
	startedCh chan struct{}
	releaseCh chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func newBlockingNotificationSender() *blockingNotificationSender {
	return &blockingNotificationSender{
		startedCh: make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (s *blockingNotificationSender) Send(ctx context.Context, _ DeliveryMessage) error {
	s.startOnce.Do(func() { close(s.startedCh) })
	select {
	case <-s.releaseCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *blockingNotificationSender) started() bool {
	select {
	case <-s.startedCh:
		return true
	default:
		return false
	}
}

func (s *blockingNotificationSender) release() {
	s.stopOnce.Do(func() { close(s.releaseCh) })
}

func waitWorkerProcessResult(t *testing.T, done <-chan workerProcessResult) workerProcessResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(time.Second):
		require.FailNow(t, "worker process did not finish")
		return workerProcessResult{}
	}
}

func assertWorkerProcessStillBlocked(t *testing.T, done <-chan workerProcessResult) {
	t.Helper()
	select {
	case result := <-done:
		require.Failf(t, "worker process finished before unblock", "processed=%d err=%v", result.processed, result.err)
	default:
	}
}
