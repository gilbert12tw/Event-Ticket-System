package ticketing

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationDeliveryOpsFeedRedactsUnsafeEventType(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 28, 10, 30, 0, 0, time.UTC)
	unsafeEventType := "notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJF1001xyz.XyZ_12345678"
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID:   "del-ops-unsafe-event-type",
		OutboxID:     "out-ops-unsafe-event-type",
		Status:       deliveryStatusDeadLetter,
		RetryCount:   3,
		DeadLetterAt: base.Add(time.Minute),
		UpdatedAt:    base,
		LastError:    "smtp failed for e1001@cets.local and E1001",
	})
	_, err := service.db.Exec(ctx, `UPDATE outbox_events SET event_type = $1 WHERE outbox_id = $2`, unsafeEventType, "out-ops-unsafe-event-type")
	require.NoError(t, err)

	page, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{
		Status: deliveryStatusDeadLetter,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	row := page.Deliveries[0]
	assert.Equal(t, "unknown", row.EventType)
	assert.Equal(t, outboxWorkerKindUnknown, row.WorkerKind)
	assert.False(t, row.RetryEligible)
	assert.True(t, row.DeadLetterEligible)
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	encoded := string(raw)
	assert.Contains(t, encoded, `"event_type":"unknown"`)
	assert.NotContains(t, encoded, "E1001")
	assert.NotContains(t, strings.ToLower(encoded), "e1001@cets.local")
	assert.NotContains(t, encoded, "eyJhbGci")
	assert.NotContains(t, encoded, unsafeEventType)
}

func TestNotificationDeliveryOpsFeedRedactsEmailBodyFromLastError(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 28, 10, 45, 0, 0, time.UTC)
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID:   "del-ops-email-body-error",
		OutboxID:     "out-ops-email-body-error",
		Status:       deliveryStatusDeadLetter,
		RetryCount:   3,
		DeadLetterAt: base.Add(time.Minute),
		UpdatedAt:    base,
		LastError:    "smtp rejected message_body=Private venue body for E1001; retry later",
	})

	page, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{
		Status: deliveryStatusDeadLetter,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	raw, err := json.Marshal(page.Deliveries[0])
	require.NoError(t, err)
	encoded := string(raw)
	assert.Contains(t, encoded, "[redacted email body]")
	assert.NotContains(t, encoded, "Private venue body")
	assert.NotContains(t, encoded, "E1001")
}
