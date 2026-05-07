package ticketing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
	if got != "e1001@cets.local" {
		t.Fatalf("address = %q", got)
	}
}

func TestDeliveryMessageForOutboxContainsEventType(t *testing.T) {
	message := deliveryMessageForOutbox("booking.confirmed", "E1001")
	if message.To != "e1001@cets.local" {
		t.Fatalf("to = %q", message.To)
	}
	if message.Subject != "CETS update: booking.confirmed" {
		t.Fatalf("subject = %q", message.Subject)
	}
	if message.Body == "" {
		t.Fatal("expected message body")
	}
}

func TestSMTPNotificationSenderReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := SMTPNotificationSender{Host: "127.0.0.1", Port: 1, From: "noreply@cets.local"}.
		Send(ctx, DeliveryMessage{To: "e1001@cets.local", Subject: "test", Body: "body"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSMTPNotificationSenderRedirectsRecipientInEnvelopeAndBody(t *testing.T) {
	sender := SMTPNotificationSender{
		From:       "noreply@cets.local",
		RedirectTo: "notifications@cets.local",
	}
	message := DeliveryMessage{To: "e1001@cets.local", Subject: "test", Body: "body"}

	recipient := sender.deliveryRecipient(message)
	body := sender.deliveryBody(message, recipient)

	if recipient != "notifications@cets.local" {
		t.Fatalf("recipient = %q, want redirect", recipient)
	}
	if strings.Contains(body, "e1001@cets.local") {
		t.Fatalf("body leaked employee recipient: %q", body)
	}
	if !strings.Contains(body, "To: notifications@cets.local") {
		t.Fatalf("body did not contain redirected recipient: %q", body)
	}
}

func TestNotificationCategorySuppressedMatchesLabelsCaseInsensitive(t *testing.T) {
	prefs := outboxNotificationPreferences{optedOutCategories: []string{"Family"}}
	if !notificationCategorySuppressed(prefs, "booking.confirmed", "family") {
		t.Fatal("expected category suppression")
	}
	if notificationCategorySuppressed(prefs, "booking.confirmed", "sports") {
		t.Fatal("did not expect unrelated category suppression")
	}
}

func TestProcessOutboxOnceSuppressesDisabledPreferences(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seedWorkerEmployee(t, service, ctx)
	if _, err := service.UpdateNotificationPreferences(ctx, Actor{ID: "E1001", Role: RoleEmployee}, NotificationPreferences{
		EmailEnabled: false,
		InAppEnabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	insertWorkerOutbox(t, service, ctx, "out-pref-suppressed", "pending", 0)
	sender := &recordingNotificationSender{}

	processed, err := service.ProcessOutboxOnce(ctx, sender, 3)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
	statuses := workerDeliveryStatuses(t, service, ctx, "out-pref-suppressed")
	if statuses["email"] != deliveryStatusSuppressed {
		t.Fatalf("email status = %q, want suppressed", statuses["email"])
	}
	if statuses["in_app"] != deliveryStatusSuppressed {
		t.Fatalf("in_app status = %q, want suppressed", statuses["in_app"])
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if processed != 3 {
		t.Fatalf("processed = %d, want 3", processed)
	}
	if sender.calls != 3 {
		t.Fatalf("sender calls = %d, want 3", sender.calls)
	}
	assertWorkerOutboxStatus(t, service, ctx, "out-batch-1", "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-batch-2", "published", 1)
	assertWorkerOutboxStatus(t, service, ctx, "out-batch-3", "published", 1)
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
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
	statuses := workerDeliveryStatuses(t, service, ctx, "out-stale-processing")
	if statuses["email"] != deliveryStatusSent {
		t.Fatalf("email status = %q, want sent", statuses["email"])
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if sender.calls != 0 {
		t.Fatalf("sender calls = %d, want 0", sender.calls)
	}
	statuses := workerDeliveryStatuses(t, service, ctx, "out-recovered-sent")
	if statuses["email"] != deliveryStatusSent {
		t.Fatalf("email status = %q, want sent", statuses["email"])
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", sender.calls)
	}
	statuses := workerDeliveryStatuses(t, service, ctx, "out-email-failed")
	if statuses["email"] != deliveryStatusFailed {
		t.Fatalf("email status = %q, want failed", statuses["email"])
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	statuses := workerDeliveryStatuses(t, service, ctx, "out-email-dead-letter")
	if statuses["email"] != deliveryStatusDeadLetter {
		t.Fatalf("email status = %q, want dead_letter", statuses["email"])
	}
	assertWorkerOutboxStatus(t, service, ctx, "out-email-dead-letter", "dead_letter", 3)
}

func seedWorkerEmployee(t *testing.T, service *Service, ctx context.Context) {
	t.Helper()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
}

func insertWorkerOutbox(t *testing.T, service *Service, ctx context.Context, outboxID string, status string, attempts int) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, publish_status, attempts, available_at)
		VALUES ($1,$2,'booking.confirmed',$3::jsonb,$4,$5,now() - interval '1 minute')`,
		outboxID, outboxID+"-aggregate", `{"employee_id":"E1001"}`, status, attempts)
	if err != nil {
		t.Fatal(err)
	}
}

func insertWorkerDelivery(t *testing.T, service *Service, ctx context.Context, deliveryID string, outboxID string, channel string, status string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO notification_deliveries
		(delivery_id, outbox_id, employee_id, channel, status)
		VALUES ($1,$2,'E1001',$3,$4)`, deliveryID, outboxID, channel, status)
	if err != nil {
		t.Fatal(err)
	}
}

func workerDeliveryStatuses(t *testing.T, service *Service, ctx context.Context, outboxID string) map[string]string {
	t.Helper()
	rows, err := service.db.Query(ctx, `SELECT channel, status FROM notification_deliveries WHERE outbox_id = $1`, outboxID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	statuses := map[string]string{}
	for rows.Next() {
		var channel, status string
		if err := rows.Scan(&channel, &status); err != nil {
			t.Fatal(err)
		}
		statuses[channel] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return statuses
}

func assertWorkerOutboxStatus(t *testing.T, service *Service, ctx context.Context, outboxID string, wantStatus string, wantAttempts int) {
	t.Helper()
	var gotStatus string
	var gotAttempts int
	if err := service.db.QueryRow(ctx, `SELECT publish_status, attempts FROM outbox_events WHERE outbox_id = $1`, outboxID).
		Scan(&gotStatus, &gotAttempts); err != nil {
		t.Fatal(err)
	}
	if gotStatus != wantStatus || gotAttempts != wantAttempts {
		t.Fatalf("outbox = (%s, %d), want (%s, %d)", gotStatus, gotAttempts, wantStatus, wantAttempts)
	}
}
