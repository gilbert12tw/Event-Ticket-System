package ticketing

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryNotificationDeliveryRequeuesFailedEmail(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	insertWorkerOutbox(t, service, ctx, "out-retry-failed", "dead_letter", 3)
	insertWorkerDelivery(t, service, ctx, "del-retry-failed", "out-retry-failed", "email", deliveryStatusDeadLetter)

	delivery, err := service.RetryNotificationDelivery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "del-retry-failed")

	require.NoError(t, err)
	assert.Equal(t, maskID("E1001"), delivery.EmployeeRef)
	assert.Equal(t, deliveryStatusPending, delivery.Status)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-retry-failed")
	assert.Equal(t, deliveryStatusPending, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-retry-failed", "pending", 3)
}

func TestRetryNotificationDeliveryRejectsOrphanEmail(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	_, err := service.db.Exec(ctx, `INSERT INTO notification_deliveries
		(delivery_id, outbox_id, employee_id, channel, status, last_error)
		VALUES ('del-orphan-email', NULL, 'E1001', 'email', $1, 'smtp unavailable')`, deliveryStatusFailed)
	require.NoError(t, err)

	_, err = service.RetryNotificationDelivery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "del-orphan-email")

	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	var status, lastError string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status, last_error FROM notification_deliveries WHERE delivery_id = 'del-orphan-email'`).
		Scan(&status, &lastError))
	assert.Equal(t, deliveryStatusFailed, status)
	assert.Equal(t, "smtp unavailable", lastError)
}

func TestNotificationDeliveriesReturnRedactedEmployeeRef(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	insertWorkerOutbox(t, service, ctx, "out-delivery-list", "published", 1)
	insertWorkerDelivery(t, service, ctx, "del-delivery-list", "out-delivery-list", "email", deliveryStatusSent)
	_, err := service.db.Exec(ctx, `UPDATE notification_deliveries SET last_error = '550 rejected e1001@cets.local for E1001' WHERE delivery_id = 'del-delivery-list'`)
	require.NoError(t, err)

	deliveries, err := service.NotificationDeliveries(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin})

	require.NoError(t, err)
	require.NotEmpty(t, deliveries)
	require.Equal(t, "del-delivery-list", deliveries[0].DeliveryID)
	assert.Equal(t, maskID("E1001"), deliveries[0].EmployeeRef)
	raw, err := json.Marshal(deliveries[0])
	require.NoError(t, err)
	encoded := string(raw)
	assert.Contains(t, encoded, `"employee_ref"`)
	assert.Contains(t, encoded, "[redacted email]")
	assert.NotContains(t, encoded, `"employee_id"`)
	assert.NotContains(t, encoded, "E1001")
	assert.NotContains(t, encoded, "e1001@cets.local")
}

func TestNotificationPreferencesDefaultAndPersistedValues(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	employee := Actor{ID: "E1001", Role: RoleEmployee}

	defaults, err := service.GetNotificationPreferences(ctx, employee)
	require.NoError(t, err)
	assert.Equal(t, "E1001", defaults.EmployeeID)
	assert.True(t, defaults.EmailEnabled)
	assert.True(t, defaults.InAppEnabled)
	assert.Empty(t, defaults.OptedOutCategories)

	updated, err := service.UpdateNotificationPreferences(ctx, employee, NotificationPreferences{
		EmailEnabled:       false,
		InAppEnabled:       true,
		OptedOutCategories: []string{" booking ", "booking", "lottery"},
	})
	require.NoError(t, err)
	assert.False(t, updated.EmailEnabled)
	assert.Equal(t, []string{"booking", "lottery"}, updated.OptedOutCategories)

	persisted, err := service.GetNotificationPreferences(ctx, employee)
	require.NoError(t, err)
	assert.Equal(t, updated.EmployeeID, persisted.EmployeeID)
	assert.Equal(t, updated.EmailEnabled, persisted.EmailEnabled)
	assert.Equal(t, updated.InAppEnabled, persisted.InAppEnabled)
	assert.Equal(t, updated.OptedOutCategories, persisted.OptedOutCategories)
	assert.False(t, persisted.UpdatedAt.IsZero())
}

func TestRetryNotificationDeliveryRejectsTerminalAndInAppDeliveries(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	cases := []struct {
		name     string
		channel  string
		status   string
		wantCode int
	}{
		{name: "sent email", channel: "email", status: deliveryStatusSent, wantCode: 409},
		{name: "suppressed email", channel: "email", status: deliveryStatusSuppressed, wantCode: 409},
		{name: "pending email", channel: "email", status: deliveryStatusPending, wantCode: 409},
		{name: "failed in-app", channel: "in_app", status: deliveryStatusFailed, wantCode: 409},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outboxID := "out-retry-" + tc.name
			deliveryID := "del-retry-" + tc.name
			insertWorkerOutbox(t, service, ctx, outboxID, "published", 1)
			insertWorkerDelivery(t, service, ctx, deliveryID, outboxID, tc.channel, tc.status)

			_, err := service.RetryNotificationDelivery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, deliveryID)

			require.Error(t, err)
			assert.Equal(t, tc.wantCode, ErrorStatus(err))
			statuses := workerDeliveryStatuses(t, service, ctx, outboxID)
			assert.Equal(t, tc.status, statuses[tc.channel])
			assertWorkerOutboxStatus(t, service, ctx, outboxID, "published", 1)
		})
	}
}
