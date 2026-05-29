package ticketing

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/traceid"

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
	assertWorkerOutboxStatus(t, service, ctx, "out-retry-failed", "pending", 0)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'notification.delivery.retry'
			AND entity_type = 'notification_delivery'
			AND entity_id = $1`, "del-retry-failed")
	assert.Equal(t, "out-retry-failed", audit["outbox_id"])
	assert.Equal(t, "email", audit["channel"])
	assert.Equal(t, deliveryStatusDeadLetter, audit["previous_status"])
	assert.Equal(t, deliveryStatusPending, audit["next_status"])
	assert.Equal(t, true, audit["retry_budget_reset"])
	assert.Equal(t, true, audit["dead_letter_cleared"])
	assertNoSensitiveJSONValues(t, audit, "E1001", "e1001@cets.local", "smtp unavailable")
}

func TestRetryNotificationDeliveryLogsSafeRetryOutcome(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx := traceid.WithContext(context.Background(), "trace-notification-retry-1")
	require.NoError(t, service.SeedDemoData(ctx))
	insertWorkerOutboxPayload(t, service, ctx, "out-retry-log", "notification.requested.v2", "dead_letter", 3, `{
		"employee_id":"E1001",
		"recipient":"e1001@cets.local",
		"event_title":"Confidential Retry"
	}`)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET retry_count = 3,
			dead_letter_at = now(),
			last_error = 'smtp unavailable for e1001@cets.local and E1001'
		WHERE outbox_id = $1`, "out-retry-log")
	require.NoError(t, err)
	insertWorkerDelivery(t, service, ctx, "del-retry-log", "out-retry-log", "email", deliveryStatusDeadLetter)
	_, err = service.db.Exec(ctx, `UPDATE notification_deliveries
		SET attempts = 3,
			last_error = 'smtp unavailable for e1001@cets.local and E1001'
		WHERE delivery_id = $1`, "del-retry-log")
	require.NoError(t, err)

	_, err = service.RetryNotificationDelivery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "del-retry-log")

	require.NoError(t, err)
	output := logs.String()
	assert.Contains(t, output, `"msg":"notification delivery retry queued"`)
	assert.Contains(t, output, `"trace_id":"trace-notification-retry-1"`)
	assert.Contains(t, output, `"action":"notification.delivery.retry"`)
	assert.Contains(t, output, `"actor_role":"activity_admin"`)
	assert.Contains(t, output, `"delivery_id":"del-retry-log"`)
	assert.Contains(t, output, `"outbox_id":"out-retry-log"`)
	assert.Contains(t, output, `"worker_kind":"notification"`)
	assert.Contains(t, output, `"channel":"email"`)
	assert.Contains(t, output, `"previous_status":"dead_letter"`)
	assert.Contains(t, output, `"next_status":"pending"`)
	assert.Contains(t, output, `"retry_budget_reset":true`)
	assert.Contains(t, output, `"dead_letter_cleared":true`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "smtp unavailable")
	assert.NotContains(t, output, "Confidential Retry")
	assert.NotContains(t, output, "payload")
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

func TestRetryNotificationDeliveryClearsOutboxDeadLetterState(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	insertWorkerOutbox(t, service, ctx, "out-retry-dead-letter-clear", "dead_letter", 3)
	_, err := service.db.Exec(ctx, `UPDATE outbox_events
		SET retry_count = 3,
			dead_letter_at = now(),
			last_error = 'smtp unavailable'
		WHERE outbox_id = $1`, "out-retry-dead-letter-clear")
	require.NoError(t, err)
	insertWorkerDelivery(t, service, ctx, "del-retry-dead-letter-clear", "out-retry-dead-letter-clear", "email", deliveryStatusDeadLetter)

	delivery, err := service.RetryNotificationDelivery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "del-retry-dead-letter-clear")

	require.NoError(t, err)
	assert.Equal(t, deliveryStatusPending, delivery.Status)
	var status string
	var lastError string
	var deadLetterAt sql.NullTime
	var attempts int
	var retryCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT publish_status, last_error, dead_letter_at, attempts, retry_count
		FROM outbox_events WHERE outbox_id = $1`, "out-retry-dead-letter-clear").
		Scan(&status, &lastError, &deadLetterAt, &attempts, &retryCount))
	assert.Equal(t, "pending", status)
	assert.Empty(t, lastError)
	assert.False(t, deadLetterAt.Valid)
	assert.Equal(t, 0, attempts)
	assert.Equal(t, 0, retryCount)

	page, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{
		Status: deliveryStatusPending,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	assert.Equal(t, "del-retry-dead-letter-clear", page.Deliveries[0].DeliveryID)
	assert.Equal(t, 0, page.Deliveries[0].RetryCount)
	assert.Nil(t, page.Deliveries[0].DeadLetterAt)
	assert.False(t, page.Deliveries[0].RetryEligible)
	assert.False(t, page.Deliveries[0].DeadLetterEligible)
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

func TestNotificationDeliveryOpsFeedFiltersPaginatesAndRedacts(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	providerToken := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJvcHMifQ.signature"
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID:   "del-ops-newest",
		OutboxID:     "out-ops-newest",
		Status:       deliveryStatusDeadLetter,
		RetryCount:   4,
		DeadLetterAt: base.Add(4 * time.Minute),
		UpdatedAt:    base.Add(5 * time.Minute),
		LastError:    "smtp failed for e1001@cets.local and E1001 with token " + providerToken,
	})
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID: "del-ops-middle",
		OutboxID:   "out-ops-middle",
		Status:     deliveryStatusDeadLetter,
		RetryCount: 3,
		UpdatedAt:  base.Add(3 * time.Minute),
		LastError:  "smtp still down for E1001",
	})
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID: "del-ops-sent",
		OutboxID:   "out-ops-sent",
		Status:     deliveryStatusSent,
		UpdatedAt:  base.Add(2 * time.Minute),
	})

	page, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{
		Status: deliveryStatusDeadLetter,
		Limit:  1,
	})

	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	first := page.Deliveries[0]
	assert.Equal(t, "del-ops-newest", first.DeliveryID)
	assert.Equal(t, "notification", first.WorkerKind)
	assert.Equal(t, "notification.requested.v2", first.EventType)
	assert.Equal(t, 4, first.RetryCount)
	assert.True(t, first.RetryEligible)
	assert.True(t, first.DeadLetterEligible)
	assert.NotNil(t, first.DeadLetterAt)
	assert.Equal(t, maskID("E1001"), first.RecipientRedacted)
	raw, err := json.Marshal(first)
	require.NoError(t, err)
	encoded := string(raw)
	assert.Contains(t, encoded, "[redacted email]")
	assert.Contains(t, encoded, "[redacted token]")
	assert.NotContains(t, encoded, "E1001")
	assert.NotContains(t, encoded, "e1001@cets.local")
	assert.NotContains(t, encoded, providerToken)
	require.NotEmpty(t, page.NextCursor)

	cursor, cursorID, err := ParseNotificationDeliveryCursorStrict(page.NextCursor)
	require.NoError(t, err)
	secondPage, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{
		Status:   deliveryStatusDeadLetter,
		Limit:    1,
		Cursor:   cursor,
		CursorID: cursorID,
	})

	require.NoError(t, err)
	require.Len(t, secondPage.Deliveries, 1)
	assert.Equal(t, "del-ops-middle", secondPage.Deliveries[0].DeliveryID)
	assert.Empty(t, secondPage.NextCursor)
}

func TestNotificationDeliveryOpsFeedIsReadOnly(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 28, 9, 30, 0, 0, time.UTC)
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID:   "del-ops-read-only",
		OutboxID:     "out-ops-read-only",
		Status:       deliveryStatusDeadLetter,
		RetryCount:   5,
		DeadLetterAt: base.Add(time.Minute),
		UpdatedAt:    base,
		LastError:    "smtp failed for e1001@cets.local and E1001",
	})
	var auditBefore int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&auditBefore))

	page, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{
		Status: deliveryStatusDeadLetter,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	assert.Equal(t, "del-ops-read-only", page.Deliveries[0].DeliveryID)
	var deliveryStatus, deliveryError, outboxStatus, outboxError string
	var attempts, retryCount, auditAfter int
	var deadLetterAt sql.NullTime
	require.NoError(t, service.db.QueryRow(ctx, `SELECT nd.status, nd.attempts, nd.last_error,
			oe.publish_status, oe.retry_count, oe.last_error, oe.dead_letter_at
		FROM notification_deliveries nd
		JOIN outbox_events oe ON oe.outbox_id = nd.outbox_id
		WHERE nd.delivery_id = $1`, "del-ops-read-only").
		Scan(&deliveryStatus, &attempts, &deliveryError, &outboxStatus, &retryCount, &outboxError, &deadLetterAt))
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&auditAfter))
	assert.Equal(t, deliveryStatusDeadLetter, deliveryStatus)
	assert.Equal(t, 5, attempts)
	assert.Equal(t, "smtp failed for e1001@cets.local and E1001", deliveryError)
	assert.Equal(t, "dead_letter", outboxStatus)
	assert.Equal(t, 5, retryCount)
	assert.Empty(t, outboxError)
	assert.True(t, deadLetterAt.Valid)
	assert.Equal(t, auditBefore, auditAfter)
}

func TestNotificationDeliveryOpsFeedRejectsNonHRAdminActors(t *testing.T) {
	service := NewService(nil, NewSigner("test-secret"), nil)

	for _, actor := range []Actor{
		{ID: "E1001", Role: RoleEmployee},
		{ID: "admin-1", Role: RoleActivityAdmin},
	} {
		t.Run(actor.Role, func(t *testing.T) {
			page, err := service.NotificationDeliveryOpsFeed(context.Background(), actor)

			require.Error(t, err)
			assert.Equal(t, 403, ErrorStatus(err))
			assert.Empty(t, page.Deliveries)
			assert.Empty(t, page.NextCursor)
		})
	}
}

func TestNotificationDeliveryOpsFeedAllowsSystemAdminActor(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	base := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	insertOpsNotificationDelivery(t, service, ctx, opsDeliveryFixture{
		DeliveryID:   "del-ops-system-admin",
		OutboxID:     "out-ops-system-admin",
		Status:       deliveryStatusDeadLetter,
		RetryCount:   2,
		DeadLetterAt: base.Add(time.Minute),
		UpdatedAt:    base,
		LastError:    "smtp failed for e1001@cets.local and E1001",
	})

	page, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "system-1", Role: RoleSystemAdmin}, NotificationDeliveryOpsQuery{
		Status: deliveryStatusDeadLetter,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, page.Deliveries, 1)
	row := page.Deliveries[0]
	assert.Equal(t, "del-ops-system-admin", row.DeliveryID)
	assert.Equal(t, maskID("E1001"), row.RecipientRedacted)
	assert.True(t, row.RetryEligible)
	assert.True(t, row.DeadLetterEligible)
}

func TestNotificationDeliveryOpsFeedValidatesQuery(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	_, err := service.NotificationDeliveryOpsFeed(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, NotificationDeliveryOpsQuery{Status: "unknown"})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
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

type opsDeliveryFixture struct {
	DeliveryID   string
	OutboxID     string
	EventType    string
	Status       string
	RetryCount   int
	DeadLetterAt time.Time
	UpdatedAt    time.Time
	LastError    string
}

func insertOpsNotificationDelivery(t *testing.T, service *Service, ctx context.Context, fixture opsDeliveryFixture) {
	t.Helper()
	outboxStatus := "published"
	if fixture.Status == deliveryStatusDeadLetter {
		outboxStatus = "dead_letter"
	}
	if strings.TrimSpace(fixture.EventType) == "" {
		fixture.EventType = "notification.requested.v2"
	}
	_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, retry_count, dead_letter_at, available_at, created_at)
		VALUES ($1,$2,$3,'{}'::jsonb,$4,$5,$6,now(),$7)`,
		fixture.OutboxID, fixture.OutboxID+"-aggregate", fixture.EventType, outboxStatus, fixture.RetryCount, nullableTime(fixture.DeadLetterAt), fixture.UpdatedAt)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `INSERT INTO notification_deliveries
		(delivery_id, outbox_id, employee_id, channel, status, attempts, last_error, created_at, updated_at)
		VALUES ($1,$2,'E1001','email',$3,$4,$5,$6,$6)`,
		fixture.DeliveryID, fixture.OutboxID, fixture.Status, fixture.RetryCount, fixture.LastError, fixture.UpdatedAt)
	require.NoError(t, err)
}

func nullableTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value, Valid: true}
}
