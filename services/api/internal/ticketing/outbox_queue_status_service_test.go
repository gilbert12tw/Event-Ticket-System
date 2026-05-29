package ticketing

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxQueueStatusAggregatesWorkerKinds(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	base := time.Now().UTC().Add(-10 * time.Minute)
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:  "out-queue-notification-pending",
		EventType: "notification.requested.v2",
		Status:    "pending",
		CreatedAt: base,
	})
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:  "out-queue-notification-processing",
		EventType: "booking.confirmed",
		Status:    "processing",
		CreatedAt: base.Add(time.Minute),
	})
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:  "out-queue-notification-dead-letter",
		EventType: "registration.cancelled",
		Status:    "dead_letter",
		CreatedAt: base.Add(2 * time.Minute),
	})
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:  "out-queue-export-pending",
		EventType: outboxEventReportExportRequested,
		Status:    "pending",
		CreatedAt: base.Add(3 * time.Minute),
	})
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:  "out-queue-export-v2-pending",
		EventType: "report.export.requested.v2",
		Status:    "pending",
		CreatedAt: base.Add(4 * time.Minute),
	})
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:    "out-queue-projection-published",
		EventType:   outboxEventReportingProjectionUpdateRequiredV2,
		Status:      "published",
		CreatedAt:   base.Add(5 * time.Minute),
		PublishedAt: base.Add(6 * time.Minute),
	})
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:  "out-queue-reservation-compensation-pending",
		EventType: "reservation.compensation.release_required.v2",
		Status:    "pending",
		CreatedAt: base.Add(7 * time.Minute),
	})
	insertQueueStatusOutbox(t, service, ctx, queueStatusFixture{
		OutboxID:  "out-queue-unknown-pending",
		EventType: "notification.requested.v2.e1001@cets.local",
		Status:    "pending",
		CreatedAt: base.Add(8 * time.Minute),
	})

	status, err := service.OutboxQueueStatus(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin})

	require.NoError(t, err)
	require.Len(t, status.Queues, 5)
	rows := queueStatusByName(status)
	notification := rows[outboxWorkerKindNotification]
	assert.Equal(t, 1, notification.Pending)
	assert.Equal(t, 1, notification.InFlight)
	assert.Equal(t, 1, notification.DeadLetter)
	assert.Greater(t, notification.P95AgeSeconds, 0)
	assert.Nil(t, notification.LastProcessedAt)
	export := rows[outboxWorkerKindExport]
	assert.Equal(t, 2, export.Pending)
	assert.Equal(t, 0, export.InFlight)
	projection := rows[outboxWorkerKindProjection]
	assert.Equal(t, 0, projection.Pending)
	require.NotNil(t, projection.LastProcessedAt)
	assert.Equal(t, base.Add(6*time.Minute).Unix(), projection.LastProcessedAt.Unix())
	compensation := rows[outboxWorkerKindCompensation]
	assert.Equal(t, 1, compensation.Pending)
	assert.Greater(t, compensation.P95AgeSeconds, 0)
	unknown := rows[outboxWorkerKindUnknown]
	assert.Equal(t, 1, unknown.Pending)
	assert.Equal(t, 0, unknown.InFlight)
	assert.Equal(t, 0, unknown.DeadLetter)
	assert.Greater(t, unknown.P95AgeSeconds, 0)
	raw, err := json.Marshal(status)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "payload")
}

func TestOutboxQueueStatusDoesNotExposePayloadOrEnvelopeIdentifiers(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, idempotency_key, partition_key, last_error, created_at)
		VALUES (
			'out-queue-sensitive',
			'evt_sensitive_employee',
			'notification.requested.v2',
			'{"employee_id":"E1001","recipient":"e1001@cets.local","provider_token":"eyJhbGciSecret","body":"private body"}'::jsonb,
			'pending',
			'notification.requested.v2:E1001:e1001@cets.local',
			'E1001|evt_sensitive_employee',
			'smtp failed for e1001@cets.local',
			now()
		)`)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, created_at)
		VALUES (
			'out-queue-unsafe-event-type',
			'evt_sensitive_event_type',
			'notification.requested.v2.e1001@cets.local.eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJxdWV1ZSJ9.signature',
			'{}'::jsonb,
			'pending',
			now()
		)`)
	require.NoError(t, err)

	status, err := service.OutboxQueueStatus(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin})

	require.NoError(t, err)
	raw, err := json.Marshal(status)
	require.NoError(t, err)
	encoded := string(raw)
	assert.Contains(t, encoded, `"name":"notification"`)
	assert.Contains(t, encoded, `"name":"unknown"`)
	assert.NotContains(t, encoded, "out-queue-sensitive")
	assert.NotContains(t, encoded, "out-queue-unsafe-event-type")
	assert.NotContains(t, encoded, "evt_sensitive_employee")
	assert.NotContains(t, encoded, "evt_sensitive_event_type")
	assert.NotContains(t, encoded, "E1001")
	assert.NotContains(t, encoded, "e1001@cets.local")
	assert.NotContains(t, encoded, "eyJhbGciSecret")
	assert.NotContains(t, encoded, "private body")
	assert.NotContains(t, encoded, "idempotency")
	assert.NotContains(t, encoded, "partition")
	assert.NotContains(t, encoded, "smtp failed")
}

func TestOutboxQueueStatusRequiresHRAdminOrSystemAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	for _, actor := range []Actor{
		{ID: "E1001", Role: RoleEmployee},
		{ID: "admin-1", Role: RoleActivityAdmin},
	} {
		t.Run("rejects "+actor.Role, func(t *testing.T) {
			_, err := service.OutboxQueueStatus(ctx, actor)

			require.Error(t, err)
			assert.Equal(t, 403, ErrorStatus(err))
		})
	}

	for _, actor := range []Actor{
		{ID: "hr-1", Role: RoleHRAdmin},
		{ID: "system-1", Role: RoleSystemAdmin},
	} {
		t.Run("allows "+actor.Role, func(t *testing.T) {
			status, err := service.OutboxQueueStatus(ctx, actor)

			require.NoError(t, err)
			require.Len(t, status.Queues, 5)
		})
	}
}

type queueStatusFixture struct {
	OutboxID    string
	EventType   string
	Status      string
	CreatedAt   time.Time
	PublishedAt time.Time
}

func insertQueueStatusOutbox(t *testing.T, service *Service, ctx context.Context, fixture queueStatusFixture) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, created_at, published_at)
		VALUES ($1,$2,$3,'{}'::jsonb,$4,$5,$6)`,
		fixture.OutboxID, fixture.OutboxID+"-aggregate", fixture.EventType, fixture.Status,
		fixture.CreatedAt, nullableQueueStatusTime(fixture.PublishedAt))
	require.NoError(t, err)
}

func nullableQueueStatusTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value, Valid: true}
}

func queueStatusByName(status OutboxQueueStatus) map[string]OutboxQueueStatusRow {
	rows := map[string]OutboxQueueStatusRow{}
	for _, row := range status.Queues {
		rows[row.Name] = row
	}
	return rows
}
