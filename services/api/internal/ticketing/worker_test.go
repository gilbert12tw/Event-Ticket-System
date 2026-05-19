package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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

func TestSMTPNotificationSenderReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := SMTPNotificationSender{Host: "127.0.0.1", Port: 1, From: "noreply@cets.local"}.
		Send(ctx, DeliveryMessage{To: "e1001@cets.local", Subject: "test", Body: "body"})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSMTPNotificationSenderRedirectsRecipientInEnvelopeAndBody(t *testing.T) {
	sender := SMTPNotificationSender{
		From:       "noreply@cets.local",
		RedirectTo: "notifications@cets.local",
	}
	message := DeliveryMessage{To: "e1001@cets.local", Subject: "test", Body: "body"}

	recipient := sender.deliveryRecipient(message)
	body := sender.deliveryBody(message, recipient)

	assert.Equal(t, "notifications@cets.local", recipient)
	assert.NotContains(t, body, "e1001@cets.local", "body leaked employee recipient")
	assert.Contains(t, body, "To: notifications@cets.local")
}

func TestNotificationCategorySuppressedMatchesLabelsCaseInsensitive(t *testing.T) {
	prefs := outboxNotificationPreferences{optedOutCategories: []string{"Family"}}
	assert.True(t, notificationCategorySuppressed(prefs, "booking.confirmed", "family"), "expected category suppression")
	assert.False(t, notificationCategorySuppressed(prefs, "booking.confirmed", "sports"), "did not expect unrelated category suppression")
}

func TestProcessOutboxOnceSuppressesDisabledPreferences(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
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

func TestProcessOutboxOnceProcessesConfiguredBatch(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
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
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
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
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
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
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
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

func TestProcessOutboxOnceMarksFailedEmailForRetry(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
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

func TestProcessOutboxOnceMarksDeadLetterAtMaxAttempts(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
	insertWorkerOutbox(t, service, ctx, "out-email-dead-letter", "pending", 2)
	sender := &recordingNotificationSender{err: errors.New("smtp unavailable")}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	statuses := workerDeliveryStatuses(t, service, ctx, "out-email-dead-letter")
	assert.Equal(t, deliveryStatusDeadLetter, statuses["email"])
	assertWorkerOutboxStatus(t, service, ctx, "out-email-dead-letter", "dead_letter", 3)
}

func TestBookingOutboxPayloadIncludesOnlyRedactedCrossCityContext(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:     "Cross City Notification",
		Location:  "Taipei HQ",
		EventCity: "Taipei",
		Capacity:  2,
		Status:    EventStatusPublished,
		Rule:      RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Hsinchu",
		Grade:            6,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "cross-city-notification"})
	require.NoError(t, err)

	payloadText := workerOutboxPayload(t, service, ctx, booking.Registration.RegistrationID, "booking.confirmed")
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &payload))
	assert.Equal(t, string(WarningCrossCity), payload["warning_code"])
	assert.Equal(t, "Taipei", payload["event_city"])
	assert.NotContains(t, payload, "employee_city")
	assert.NotContains(t, payloadText, "Hsinchu")
	assert.NotContains(t, payloadText, "Ariel Chen")
	assert.NotContains(t, payloadText, "@")
	assert.NotContains(t, payloadText, "signed_token")
	assert.NotContains(t, payloadText, "qr_payload")

	sender := &recordingNotificationSender{}
	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, sender.messages, 1)
	assert.Contains(t, sender.messages[0].Body, "This activity is in Taipei")
	assert.NotContains(t, sender.messages[0].Body, "Hsinchu")
}

func TestBookingOutboxPayloadOmitsCrossCityContextForSameCity(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:     "Same City Notification",
		Location:  "Taipei HQ",
		EventCity: "Taipei",
		Capacity:  2,
		Status:    EventStatusPublished,
		Rule:      RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Taipei",
		Grade:            6,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "same-city-notification"})
	require.NoError(t, err)

	payloadText := workerOutboxPayload(t, service, ctx, booking.Registration.RegistrationID, "booking.confirmed")
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &payload))
	assert.NotContains(t, payload, "warning_code")
	assert.NotContains(t, payload, "event_city")

	sender := &recordingNotificationSender{}
	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, sender.messages, 1)
	assert.NotContains(t, sender.messages[0].Body, "This activity is in")
}

func TestBookingOutboxPayloadOmitsCrossCityContextForWaitlistedBooking(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:     "Waitlist Cross City Notification",
		Location:  "Taipei HQ",
		EventCity: "Taipei",
		Capacity:  1,
		Status:    EventStatusPublished,
		Rule:      RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Hsinchu",
		Grade:            6,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "waitlist-cross-city-confirmed"})
	require.NoError(t, err)
	waitlisted, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee, Claims: &ProviderClaims{
		Department:       "Engineering",
		Site:             "Taipei HQ",
		City:             "Hsinchu",
		Grade:            5,
		EmploymentStatus: "active",
	}}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "waitlist-cross-city-waitlisted"})
	require.NoError(t, err)
	require.Equal(t, RegistrationWaitlisted, waitlisted.Registration.Status)

	payloadText := workerOutboxPayload(t, service, ctx, waitlisted.Registration.RegistrationID, "booking.waitlisted")
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &payload))
	assert.NotContains(t, payload, "warning_code")
	assert.NotContains(t, payload, "event_city")
	assert.NotContains(t, payloadText, "Hsinchu")
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
	return payload
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
