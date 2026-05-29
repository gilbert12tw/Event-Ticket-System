package ticketing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSeededWorkerTest(t *testing.T) (*Service, context.Context) {
	t.Helper()
	service, ctx := newWorkerTest(t)
	seedWorkerEmployee(t, service, ctx)
	return service, ctx
}

func newWorkerTest(t *testing.T) (*Service, context.Context) {
	t.Helper()
	service, cleanup := newIntegrationService(t)
	t.Cleanup(cleanup)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return service, ctx
}

func seedWorkerEmployee(t *testing.T, service *Service, ctx context.Context) {
	t.Helper()
	require.NoError(t, service.SeedDemoData(ctx))
}

func insertWorkerOutbox(t *testing.T, service *Service, ctx context.Context, outboxID string, status string, attempts int) {
	t.Helper()
	insertWorkerOutboxPayload(t, service, ctx, outboxID, "booking.confirmed", status, attempts, `{"employee_id":"E1001"}`)
}

func insertWorkerOutboxPayload(t *testing.T, service *Service, ctx context.Context, outboxID string, eventType string, status string, attempts int, payload string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, attempts, available_at)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6,now() - interval '1 minute')`,
		outboxID, outboxID+"-aggregate", eventType, payload, status, attempts)
	require.NoError(t, err)
}

func insertWorkerDelivery(t *testing.T, service *Service, ctx context.Context, deliveryID string, outboxID string, channel string, status string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO notification_deliveries
		(delivery_id, outbox_id, employee_id, channel, status)
		VALUES ($1,$2,'E1001',$3,$4)`, deliveryID, outboxID, channel, status)
	require.NoError(t, err)
}

func workerDeliveryStatuses(t *testing.T, service *Service, ctx context.Context, outboxID string) map[string]string {
	t.Helper()
	rows, err := service.db.Query(ctx, `SELECT channel, status FROM notification_deliveries WHERE outbox_id = $1`, outboxID)
	require.NoError(t, err)
	defer rows.Close()
	statuses := map[string]string{}
	for rows.Next() {
		var channel, status string
		require.NoError(t, rows.Scan(&channel, &status))
		statuses[channel] = status
	}
	require.NoError(t, rows.Err())
	return statuses
}

func workerOutboxPayload(t *testing.T, service *Service, ctx context.Context, aggregateID string, eventType string) string {
	t.Helper()
	var payload string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT payload::text FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`, aggregateID, eventType).
		Scan(&payload))
	envelope := map[string]interface{}{}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return payload
	}
	nested, ok := envelope["payload"].(map[string]interface{})
	if !ok {
		return payload
	}
	body, err := json.Marshal(nested)
	require.NoError(t, err)
	return string(body)
}

func assertWorkerOutboxStatus(t *testing.T, service *Service, ctx context.Context, outboxID string, wantStatus string, wantAttempts int) {
	t.Helper()
	var gotStatus string
	var gotAttempts int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, attempts FROM outbox_events WHERE outbox_id = $1`, outboxID).
		Scan(&gotStatus, &gotAttempts))
	assert.Equal(t, wantStatus, gotStatus)
	assert.Equal(t, wantAttempts, gotAttempts)
}

func assertWorkerDeliveryCount(t *testing.T, service *Service, ctx context.Context, outboxID string, want int) {
	t.Helper()
	var got int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM notification_deliveries WHERE outbox_id = $1`, outboxID).Scan(&got))
	assert.Equal(t, want, got)
}
