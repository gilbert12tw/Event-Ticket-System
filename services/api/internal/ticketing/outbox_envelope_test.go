package ticketing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsertOutboxWritesEnvelopeV2Metadata(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer rollback(ctx, tx)

	require.NoError(t, insertOutbox(ctx, tx, "notification.requested.v2", "agg-envelope-v2", map[string]interface{}{
		"notification_id":       "ntf-envelope-v2",
		"recipient_employee_id": "E1001",
	}))
	require.NoError(t, tx.Commit(ctx))

	var (
		outboxID       string
		schemaVersion  int
		idempotencyKey string
		partitionKey   string
		payloadText    string
	)
	require.NoError(t, service.db.QueryRow(ctx, `SELECT outbox_id, schema_version, COALESCE(idempotency_key, ''), COALESCE(partition_key, ''), payload::text
		FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2`, "agg-envelope-v2", "notification.requested.v2").
		Scan(&outboxID, &schemaVersion, &idempotencyKey, &partitionKey, &payloadText))
	assert.Equal(t, 2, schemaVersion)
	assert.Equal(t, "E1001", partitionKey)
	assert.Equal(t, "notification.requested:ntf-envelope-v2", idempotencyKey)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &envelope))
	assert.Equal(t, outboxID, envelope["event_id"])
	assert.Equal(t, "notification.requested.v2", envelope["event_type"])
	assert.Equal(t, float64(2), envelope["schema_version"])
	assert.Equal(t, idempotencyKey, envelope["idempotency_key"])
	assert.Equal(t, partitionKey, envelope["partition_key"])
	occurredAt, ok := envelope["occurred_at"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, occurredAt)
	require.NoError(t, err)
	nested, ok := envelope["payload"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ntf-envelope-v2", nested["notification_id"])
	assert.Equal(t, "E1001", nested["recipient_employee_id"])
}

func TestOutboxEnvelopeV2IdempotencyKeyUniquePerEventType(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	insert := func(outboxID string, eventType string, idempotencyKey string) error {
		_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
			(outbox_id, aggregate_id, event_type, payload, schema_version, idempotency_key, partition_key)
			VALUES ($1,$2,$3,'{}'::jsonb,2,$4,$5)`,
			outboxID, outboxID+"-aggregate", eventType, idempotencyKey, outboxID+"-partition")
		return err
	}

	require.NoError(t, insert("out-idem-original", "notification.requested.v2", "shared-idem-key"))
	err := insert("out-idem-duplicate", "notification.requested.v2", "shared-idem-key")
	require.Error(t, err)
	assert.True(t, isUniqueViolation(err))
	require.NoError(t, insert("out-idem-other-type", "report.export.requested.v2", "shared-idem-key"))

	var count int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE idempotency_key = $1`, "shared-idem-key").Scan(&count))
	assert.Equal(t, 2, count)
}
