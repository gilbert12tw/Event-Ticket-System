package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryNotificationDeliveryRejectsNonNotificationOutbox(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	insertWorkerOutboxPayload(t, service, ctx, "out-retry-export", outboxEventReportExportRequestedV2, "dead_letter", 3, `{"export_id":"exp_retry_export"}`)
	insertWorkerDelivery(t, service, ctx, "del-retry-export", "out-retry-export", "email", deliveryStatusFailed)

	_, err := service.RetryNotificationDelivery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "del-retry-export")

	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	statuses := workerDeliveryStatuses(t, service, ctx, "out-retry-export")
	assert.Equal(t, deliveryStatusFailed, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-retry-export", "dead_letter", 3)
}

func TestRetryNotificationDeliveryRejectsUnknownOutboxEventType(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJF1001xyz.XyZ_12345678"
	insertWorkerOutboxPayload(t, service, ctx, "out-retry-unsafe", unsafeEventType, "dead_letter", 3, `{"notification_id":"ntf_retry_unsafe"}`)
	insertWorkerDelivery(t, service, ctx, "del-retry-unsafe", "out-retry-unsafe", "email", deliveryStatusDeadLetter)

	_, err := service.RetryNotificationDelivery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "del-retry-unsafe")

	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	statuses := workerDeliveryStatuses(t, service, ctx, "out-retry-unsafe")
	assert.Equal(t, deliveryStatusDeadLetter, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-retry-unsafe", "dead_letter", 3)
}

func TestNotificationDeliveryOpsFeedDoesNotMarkNonNotificationOutboxRetryEligible(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 28, 9, 45, 0, 0, time.UTC)
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID:   "del-ops-export-retry",
		OutboxID:     "out-ops-export-retry",
		EventType:    outboxEventReportExportRequestedV2,
		Status:       deliveryStatusDeadLetter,
		RetryCount:   5,
		DeadLetterAt: base.Add(time.Minute),
		UpdatedAt:    base,
		LastError:    "object store unavailable for e1001@cets.local and E1001",
	})

	page, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{
		Status: deliveryStatusDeadLetter,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	row := page.Deliveries[0]
	assert.Equal(t, "del-ops-export-retry", row.DeliveryID)
	assert.Equal(t, outboxWorkerKindExport, row.WorkerKind)
	assert.False(t, row.RetryEligible)
	assert.True(t, row.DeadLetterEligible)
}
