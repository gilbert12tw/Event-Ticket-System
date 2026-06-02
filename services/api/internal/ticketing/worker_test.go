package ticketing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"event-ticket-system/internal/traceid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingNotificationSender struct {
	err      error
	calls    int
	messages []DeliveryMessage
}

func (s *recordingNotificationSender) Send(ctx context.Context, message DeliveryMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.calls++
	s.messages = append(s.messages, message)
	return s.err
}

type idempotentCancelingNotificationSender struct {
	cancel       context.CancelFunc
	calls        int
	providerSent int
	messages     []DeliveryMessage
	seen         map[string]struct{}
}

func (s *idempotentCancelingNotificationSender) Send(ctx context.Context, message DeliveryMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.calls++
	s.messages = append(s.messages, message)
	if s.seen == nil {
		s.seen = map[string]struct{}{}
	}
	if _, ok := s.seen[message.IdempotencyKey]; !ok {
		s.seen[message.IdempotencyKey] = struct{}{}
		s.providerSent++
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	return nil
}

func TestDeliveryAddressForEmployeeNormalizesLocalMailbox(t *testing.T) {
	got := deliveryAddressForEmployee(" E1001 ")
	assert.Equal(t, "e1001@cets.local", got)
}

func TestDeliveryMessageForOutboxContainsEventType(t *testing.T) {
	message := deliveryMessageForOutbox("booking.confirmed", "E1001", nil)
	assert.Equal(t, "e1001@cets.local", message.To)
	assert.Equal(t, "CETS update: booking.confirmed", message.Subject)
	assert.NotEmpty(t, message.Body, "expected message body")
}

func TestDeliveryMessageForOutboxIncludesActivityContext(t *testing.T) {
	message := deliveryMessageForOutbox("registration.cancelled", "E1001", map[string]interface{}{
		"event_title": "Factory Family Day",
		"starts_at":   "2026-05-19T10:00:00Z",
	})

	assert.Contains(t, message.Body, "Factory Family Day")
	assert.Contains(t, message.Body, "2026-05-19T10:00:00Z")
}

func TestDeliveryMessageForOutboxFallsBackWithoutActivityContext(t *testing.T) {
	message := deliveryMessageForOutbox("registration.no_show_recorded", "E1001", map[string]interface{}{
		"employee_id": "E1001",
	})

	assert.Contains(t, message.Body, "registration.no_show_recorded")
	assert.NotContains(t, message.Body, "Activity:")
	assert.NotContains(t, message.Body, "Starts at:")
}

func TestDeliveryMessageForOutboxIncludesCrossCityActivityCity(t *testing.T) {
	message := deliveryMessageForOutbox("booking.confirmed", "E1001", map[string]interface{}{
		"warning_code": string(WarningCrossCity),
		"event_city":   "Taipei",
	})

	assert.Contains(t, message.Body, "This activity is in Taipei")
}

func TestDeliveryMessageForOutboxOmitsCrossCityWordingWhenContextIncomplete(t *testing.T) {
	noWarning := deliveryMessageForOutbox("booking.confirmed", "E1001", map[string]interface{}{
		"event_city": "Taipei",
	})
	missingCity := deliveryMessageForOutbox("booking.confirmed", "E1001", map[string]interface{}{
		"warning_code": string(WarningCrossCity),
	})

	assert.NotContains(t, noWarning.Body, "This activity is in")
	assert.NotContains(t, missingCity.Body, "This activity is in")
}

func TestNotificationCategorySuppressedMatchesLabelsCaseInsensitive(t *testing.T) {
	prefs := outboxNotificationPreferences{optedOutCategories: []string{"Family"}}
	assert.True(t, notificationCategorySuppressed(prefs, "booking.confirmed", "family"), "expected category suppression")
	assert.False(t, notificationCategorySuppressed(prefs, "booking.confirmed", "sports"), "did not expect unrelated category suppression")
}

func TestProcessOutboxOnceSuppressesDisabledPreferences(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	_, err := service.UpdateNotificationPreferences(ctx, Actor{ID: "E1001", Role: RoleEmployee}, NotificationPreferences{
		EmailEnabled: false,
		InAppEnabled: false,
	})
	require.NoError(t, err)
	insertWorkerOutbox(t, service, ctx, "out-pref-suppressed", "pending", 0)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-pref-suppressed")
	assert.Equal(t, deliveryStatusSuppressed, statuses["email"])
	assert.Equal(t, deliveryStatusSuppressed, statuses["in_app"])
	assertWorkerOutboxStatus(t, service, ctx, "out-pref-suppressed", "published", 1)
}

func TestProcessOutboxOncePublishesEventsWithoutRecipient(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertWorkerOutboxPayload(t, service, ctx, "out-no-recipient", "event.updated", "pending", 0, `{"event_id":"evt_1"}`)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	assertWorkerOutboxStatus(t, service, ctx, "out-no-recipient", "published", 1)
	assertWorkerDeliveryCount(t, service, ctx, "out-no-recipient", 0)
}

func TestProcessOutboxOnceSuppressesOptedOutEventCategory(t *testing.T) {
	service, ctx := newWorkerTest(t)
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Family Day",
		Capacity: 5,
		Status:   EventStatusPublished,
		Category: "family",
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.UpdateNotificationPreferences(ctx, Actor{ID: "E1001", Role: RoleEmployee}, NotificationPreferences{
		EmailEnabled:       true,
		InAppEnabled:       true,
		OptedOutCategories: []string{"family"},
	})
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]string{"employee_id": "E1001", "event_id": event.EventID})
	require.NoError(t, err)
	insertWorkerOutboxPayload(t, service, ctx, "out-category-suppressed", "booking.confirmed", "pending", 0, string(payload))
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-category-suppressed")
	assert.Equal(t, deliveryStatusSuppressed, statuses["email"])
	assert.Equal(t, deliveryStatusSuppressed, statuses["in_app"])
	assertWorkerOutboxStatus(t, service, ctx, "out-category-suppressed", "published", 1)
}

func TestProcessOutboxOnceProcessesConfiguredBatch(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-batch-1", "pending", 0)
	insertWorkerOutbox(t, service, ctx, "out-batch-2", "pending", 0)
	insertWorkerOutbox(t, service, ctx, "out-batch-3", "pending", 0)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		MaxAttempts: 3,
		BatchSize:   3,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, processed)
	assert.Equal(t, 3, sender.calls)
	assertWorkerOutboxStatus(t, service, ctx, "out-batch-1", "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-batch-2", "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-batch-3", "published", 1)
}

func TestProcessOutboxOnceSendsGovernanceEventContext(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutboxPayload(t, service, ctx, "out-governance-context", "registration.cancelled", "pending", 0, `{
		"employee_id":"E1001",
		"event_id":"evt-context",
		"event_title":"Cancellation Drill",
		"starts_at":"2026-05-19T10:00:00Z"
	}`)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, sender.messages, 1)
	assert.Contains(t, sender.messages[0].Body, "Cancellation Drill")
	assert.Contains(t, sender.messages[0].Body, "2026-05-19T10:00:00Z")
	assertWorkerOutboxStatus(t, service, ctx, "out-governance-context", "published", 1)
}

func TestProcessOutboxOnceClaimsStaleProcessingOutbox(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-stale-processing", "processing", 1)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-stale-processing")
	assert.Equal(t, deliveryStatusSent, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-stale-processing", "published", 2)
}

func TestProcessOutboxOnceDoesNotResendAlreadySentEmail(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-recovered-sent", "processing", 1)
	insertWorkerDelivery(t, service, ctx, "del-existing-email", "out-recovered-sent", "email", deliveryStatusSent)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-recovered-sent")
	assert.Equal(t, deliveryStatusSent, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-recovered-sent", "published", 2)
}

func TestProcessOutboxOnceDeadLettersAmbiguousEmailWithoutResend(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-ambiguous-email", "processing", 1)
	insertWorkerDelivery(t, service, ctx, "del-ambiguous-email", "out-ambiguous-email", "email", deliveryStatusSending)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 0, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-ambiguous-email")
	assert.Equal(t, deliveryStatusDeadLetter, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-ambiguous-email", "dead_letter", 2)
	var lastError string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT last_error FROM outbox_events WHERE outbox_id = $1`, "out-ambiguous-email").Scan(&lastError))
	assert.Contains(t, lastError, "ambiguous")
}

func TestProcessOutboxOnceMarksFailedEmailForRetry(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-email-failed", "pending", 0)
	sender := &recordingNotificationSender{err: errors.New("smtp unavailable")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, sender.calls)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-email-failed")
	assert.Equal(t, deliveryStatusFailed, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-email-failed", "pending", 1)
}

func TestProcessOutboxOnceDeadLettersUnacknowledgedEmailAfterSendCancellation(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	seedWorkerEmployee(t, service, ctx)
	insertWorkerOutbox(t, service, ctx, "out-email-idempotent", "pending", 0)
	sender := &idempotentCancelingNotificationSender{cancel: cancel}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.Error(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, sender.calls)
	assert.Equal(t, 1, sender.providerSent)
	require.Len(t, sender.messages, 1)
	assert.NotEmpty(t, sender.messages[0].IdempotencyKey)

	runCtx := context.Background()
	assertWorkerOutboxStatus(t, service, runCtx, "out-email-idempotent", "pending", 1)

	processed, err = service.ProcessOutboxOnce(runCtx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, sender.calls)
	assert.Equal(t, 1, sender.providerSent)
	statuses := workerDeliveryStatuses(t, service, runCtx, "out-email-idempotent")
	assert.Equal(t, deliveryStatusDeadLetter, statuses["email"])
	assertWorkerOutboxStatus(t, service, runCtx, "out-email-idempotent", "dead_letter", 2)
}

func TestProcessOutboxOnceRedactsFailedDeliveryError(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-email-redacted", "pending", 0)
	sender := &recordingNotificationSender{err: errors.New("550 rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	var deliveryError string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT last_error FROM notification_deliveries WHERE outbox_id = $1 AND channel = 'email'`, "out-email-redacted").Scan(&deliveryError))
	assertSanitizedNotificationError(t, deliveryError)
	var outboxError string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT last_error FROM outbox_events WHERE outbox_id = $1`, "out-email-redacted").Scan(&outboxError))
	assertSanitizedNotificationError(t, outboxError)

	deliveries, err := service.NotificationDeliveries(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin})
	require.NoError(t, err)
	require.NotEmpty(t, deliveries)
	raw, err := json.Marshal(deliveries[0])
	require.NoError(t, err)
	assertSanitizedNotificationError(t, string(raw))
}

func TestProcessOutboxOnceMarksDeadLetterAtMaxAttempts(t *testing.T) {
	service, ctx := newSeededWorkerTest(t)
	insertWorkerOutbox(t, service, ctx, "out-email-dead-letter", "pending", 2)
	sender := &recordingNotificationSender{err: errors.New("smtp unavailable")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-email-dead-letter")
	assert.Equal(t, deliveryStatusDeadLetter, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-email-dead-letter", "dead_letter", 3)
}

func TestProcessOutboxOnceLogsTraceSafeWorkerAttempt(t *testing.T) {
	var logs bytes.Buffer
	service, cleanup := newIntegrationServiceWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer cleanup()
	ctx, cancel := context.WithTimeout(traceid.WithContext(context.Background(), "trace-worker-log-1"), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
	insertWorkerOutboxPayload(t, service, ctx, "out-worker-log", "booking.confirmed", "pending", 0, `{
		"employee_id":"E1001",
		"event_id":"evt_worker_log",
		"event_title":"Confidential Launch",
		"recipient":"e1001@cets.local"
	}`)
	sender := &recordingNotificationSender{err: errors.New("smtp rejected e1001@cets.local for E1001")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	output := logs.String()
	assert.Contains(t, output, `"msg":"outbox worker attempt"`)
	assert.Contains(t, output, `"trace_id":"trace-worker-log-1"`)
	assert.Contains(t, output, `"event_id":"out-worker-log"`)
	assert.Contains(t, output, `"event_type":"booking.confirmed"`)
	assert.Contains(t, output, `"worker_kind":"notification"`)
	assert.Contains(t, output, `"retry_count":1`)
	assert.Contains(t, output, `"outcome":"retry"`)
	assert.Contains(t, output, `"latency_ms":`)
	assert.NotContains(t, output, "E1001")
	assert.NotContains(t, strings.ToLower(output), "e1001@cets.local")
	assert.NotContains(t, output, "Confidential Launch")
	assert.NotContains(t, output, "payload")
}

func assertSanitizedNotificationError(t *testing.T, value string) {
	t.Helper()
	assert.Contains(t, value, "550 rejected")
	assert.Contains(t, value, "[redacted email]")
	assert.NotContains(t, value, "E1001")
	assert.NotContains(t, strings.ToLower(value), "e1001@cets.local")
}
