package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultOutboxMaxAttempts      = 3
	outboxProcessingLease         = 5 * time.Minute
	notificationSuppressedByPrefs = "suppressed by notification preferences"
)

type DeliveryMessage struct {
	To      string
	Subject string
	Body    string
}

type NotificationSender interface {
	Send(ctx context.Context, message DeliveryMessage) error
}

type OutboxProcessorOptions struct {
	Sender      NotificationSender
	ReportStore ReportObjectStore
	MaxAttempts int
	BatchSize   int
}

type SMTPNotificationSender struct {
	Host       string
	Port       int
	From       string
	RedirectTo string
}

func (s SMTPNotificationSender) Send(ctx context.Context, message DeliveryMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	recipient := s.deliveryRecipient(message)
	body := s.deliveryBody(message, recipient)
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return contextError(ctx, err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return contextError(ctx, err)
	}
	defer func() { _ = client.Close() }()
	if err := client.Mail(s.From); err != nil {
		return contextError(ctx, err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return contextError(ctx, err)
	}
	writer, err := client.Data()
	if err != nil {
		return contextError(ctx, err)
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		_ = writer.Close()
		return contextError(ctx, err)
	}
	if err := writer.Close(); err != nil {
		return contextError(ctx, err)
	}
	if err := client.Quit(); err != nil {
		return contextError(ctx, err)
	}
	return nil
}

func (s SMTPNotificationSender) deliveryRecipient(message DeliveryMessage) string {
	if recipient := strings.TrimSpace(s.RedirectTo); recipient != "" {
		return recipient
	}
	return message.To
}

func (s SMTPNotificationSender) deliveryBody(message DeliveryMessage, recipient string) string {
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", s.From, recipient, message.Subject, message.Body)
}

func (s *Service) ProcessOutboxOnce(ctx context.Context, sender NotificationSender, maxAttempts int) (int, error) {
	return s.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		MaxAttempts: maxAttempts,
	})
}

func (s *Service) ProcessOutboxOnceWithOptions(ctx context.Context, options OutboxProcessorOptions) (int, error) {
	maxAttempts := options.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultOutboxMaxAttempts
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	processed := 0
	for processed < batchSize {
		count, err := s.processOneOutbox(ctx, options, maxAttempts)
		processed += count
		if err != nil || count == 0 {
			return processed, err
		}
		if err := ctx.Err(); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func (s *Service) processOneOutbox(ctx context.Context, options OutboxProcessorOptions, maxAttempts int) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer rollback(ctx, tx)

	claim, err := claimOutboxEvent(ctx, tx)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(claim.payloadText), &payload); err != nil {
		return 0, err
	}
	if claim.eventType == outboxEventReportExportRequested {
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		if err := s.processReportExportOutbox(ctx, claim, options.ReportStore, maxAttempts); err != nil {
			return 1, err
		}
		return 1, nil
	}
	employeeID := stringFromPayload(payload, "employee_id")
	if employeeID == "" {
		if err := markOutboxPublishedInTx(ctx, tx, claim.outboxID); err != nil {
			return 0, err
		}
		return 1, tx.Commit(ctx)
	}

	prefs, err := loadOutboxNotificationPreferences(ctx, tx, employeeID)
	if err != nil {
		return 0, err
	}
	category, err := notificationCategoryForOutbox(ctx, tx, claim.eventType, payload)
	if err != nil {
		return 0, err
	}
	categorySuppressed := notificationCategorySuppressed(prefs, claim.eventType, category)

	inAppStatus, inAppReason := deliveryStatusForPreference(prefs.inAppEnabled, categorySuppressed, deliveryStatusSent)
	if _, err := ensureNotificationDelivery(ctx, tx, claim.outboxID, employeeID, "in_app", inAppStatus, inAppReason); err != nil {
		return 0, err
	}
	emailStatus := ""
	if options.Sender != nil {
		desiredEmailStatus, emailReason := deliveryStatusForPreference(prefs.emailEnabled, categorySuppressed, deliveryStatusPending)
		emailStatus, err = ensureNotificationDelivery(ctx, tx, claim.outboxID, employeeID, "email", desiredEmailStatus, emailReason)
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	var sendErr error
	if options.Sender != nil && emailStatus == deliveryStatusPending {
		if err := ctx.Err(); err != nil {
			return 1, err
		}
		message := deliveryMessageForOutbox(claim.eventType, employeeID, payload)
		sendErr = options.Sender.Send(ctx, message)
		status := deliveryStatusSent
		lastError := ""
		if sendErr != nil {
			status = deliveryStatusFailed
			if claim.attempts >= maxAttempts {
				status = deliveryStatusDeadLetter
			}
			lastError = sendErr.Error()
		}
		if err := s.updateEmailDeliveryAfterSend(ctx, claim.outboxID, status, lastError); err != nil {
			return 1, err
		}
	}
	if sendErr != nil {
		return 1, s.updateOutboxAfterSendFailure(ctx, claim.outboxID, claim.attempts, maxAttempts, sendErr)
	}
	return 1, s.markOutboxPublished(ctx, claim.outboxID)
}

type outboxClaim struct {
	outboxID    string
	aggregateID string
	eventType   string
	payloadText string
	attempts    int
}

type outboxNotificationPreferences struct {
	emailEnabled       bool
	inAppEnabled       bool
	optedOutCategories []string
}

const (
	deliveryStatusPending    = "pending"
	deliveryStatusSent       = "sent"
	deliveryStatusFailed     = "failed"
	deliveryStatusSuppressed = "suppressed"
	deliveryStatusDeadLetter = "dead_letter"
)

func claimOutboxEvent(ctx context.Context, tx pgx.Tx) (outboxClaim, error) {
	var claim outboxClaim
	err := tx.QueryRow(ctx, `WITH next_outbox AS (
			SELECT outbox_id
			FROM outbox_events
			WHERE available_at <= now()
				AND publish_status IN ('pending', 'processing')
			ORDER BY CASE WHEN event_type = 'report.export.requested' THEN 0 ELSE 1 END,
				created_at ASC,
				outbox_id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events outbox
		SET publish_status = 'processing',
			attempts = attempts + 1,
			last_error = '',
			available_at = now() + ($1::double precision * interval '1 second')
		FROM next_outbox
		WHERE outbox.outbox_id = next_outbox.outbox_id
		RETURNING outbox.outbox_id, outbox.aggregate_id, outbox.event_type, outbox.payload::text, outbox.attempts`,
		outboxProcessingLease.Seconds()).
		Scan(&claim.outboxID, &claim.aggregateID, &claim.eventType, &claim.payloadText, &claim.attempts)
	return claim, err
}

func loadOutboxNotificationPreferences(ctx context.Context, tx pgx.Tx, employeeID string) (outboxNotificationPreferences, error) {
	prefs := outboxNotificationPreferences{emailEnabled: true, inAppEnabled: true}
	var categories string
	err := tx.QueryRow(ctx, `SELECT email_enabled, in_app_enabled, opted_out_categories
		FROM notification_preferences WHERE employee_id = $1`, employeeID).
		Scan(&prefs.emailEnabled, &prefs.inAppEnabled, &categories)
	if errors.Is(err, pgx.ErrNoRows) {
		return prefs, nil
	}
	if err != nil {
		return outboxNotificationPreferences{}, err
	}
	prefs.optedOutCategories = splitTags(categories)
	return prefs, nil
}

func notificationCategoryForOutbox(ctx context.Context, tx pgx.Tx, eventType string, payload map[string]interface{}) (string, error) {
	eventID := stringFromPayload(payload, "event_id")
	if eventID == "" {
		return eventType, nil
	}
	var category string
	err := tx.QueryRow(ctx, `SELECT category FROM events WHERE event_id = $1`, eventID).Scan(&category)
	if errors.Is(err, pgx.ErrNoRows) {
		return eventType, nil
	}
	if err != nil {
		return "", err
	}
	category = strings.TrimSpace(category)
	if category == "" {
		return eventType, nil
	}
	return category, nil
}

func notificationCategorySuppressed(prefs outboxNotificationPreferences, labels ...string) bool {
	for _, optedOut := range prefs.optedOutCategories {
		optedOut = strings.ToLower(strings.TrimSpace(optedOut))
		for _, label := range labels {
			if optedOut != "" && optedOut == strings.ToLower(strings.TrimSpace(label)) {
				return true
			}
		}
	}
	return false
}

func deliveryStatusForPreference(enabled bool, categorySuppressed bool, enabledStatus string) (string, string) {
	if !enabled || categorySuppressed {
		return deliveryStatusSuppressed, notificationSuppressedByPrefs
	}
	return enabledStatus, ""
}

func ensureNotificationDelivery(ctx context.Context, tx pgx.Tx, outboxID string, employeeID string, channel string, status string, lastError string) (string, error) {
	deliveryID, err := newID("del")
	if err != nil {
		return "", err
	}
	var currentStatus string
	err = tx.QueryRow(ctx, `INSERT INTO notification_deliveries
		(delivery_id, outbox_id, employee_id, channel, status, attempts, last_error)
		VALUES ($1,$2,$3,$4,$5,0,$6)
		ON CONFLICT (outbox_id, channel) WHERE outbox_id IS NOT NULL DO UPDATE SET
			employee_id = EXCLUDED.employee_id,
			status = CASE
				WHEN notification_deliveries.status IN ('sent', 'suppressed') THEN notification_deliveries.status
				ELSE EXCLUDED.status
			END,
			last_error = CASE
				WHEN notification_deliveries.status IN ('sent', 'suppressed') THEN notification_deliveries.last_error
				ELSE EXCLUDED.last_error
			END,
			updated_at = CASE
				WHEN notification_deliveries.status IN ('sent', 'suppressed') THEN notification_deliveries.updated_at
				ELSE now()
			END
		RETURNING status`, deliveryID, outboxID, employeeID, channel, status, lastError).
		Scan(&currentStatus)
	return currentStatus, err
}

func markOutboxPublishedInTx(ctx context.Context, tx pgx.Tx, outboxID string) error {
	_, err := tx.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'published',
			last_error = '',
			published_at = now()
		WHERE outbox_id = $1`, outboxID)
	return err
}

func (s *Service) markOutboxPublished(ctx context.Context, outboxID string) error {
	_, err := s.db.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'published',
			last_error = '',
			published_at = now()
		WHERE outbox_id = $1`, outboxID)
	return err
}

func (s *Service) updateEmailDeliveryAfterSend(ctx context.Context, outboxID string, status string, lastError string) error {
	_, err := s.db.Exec(ctx, `UPDATE notification_deliveries
		SET status = $1,
			attempts = attempts + 1,
			last_error = $2,
			updated_at = now()
		WHERE outbox_id = $3 AND channel = 'email' AND status = 'pending'`, status, lastError, outboxID)
	return err
}

func (s *Service) updateOutboxAfterSendFailure(ctx context.Context, outboxID string, attempts int, maxAttempts int, sendErr error) error {
	status := "pending"
	if attempts >= maxAttempts {
		status = "dead_letter"
	}
	_, err := s.db.Exec(ctx, `UPDATE outbox_events
		SET publish_status = $1,
			last_error = $2,
			available_at = now() + ($3::double precision * interval '1 second')
		WHERE outbox_id = $4`, status, sendErr.Error(), outboxRetryBackoff(attempts).Seconds(), outboxID)
	return err
}

func outboxRetryBackoff(attempts int) time.Duration {
	if attempts <= 0 {
		attempts = 1
	}
	return time.Duration(attempts) * time.Minute
}

func contextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func stringFromPayload(payload map[string]interface{}, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func deliveryMessageForOutbox(eventType string, employeeID string, payload map[string]interface{}) DeliveryMessage {
	context := notificationActivityContext(payload)
	body := fmt.Sprintf("Your corporate event ticketing status changed: %s.", eventType)
	if context != "" {
		body = fmt.Sprintf("%s %s", body, context)
	}
	return DeliveryMessage{
		To:      deliveryAddressForEmployee(employeeID),
		Subject: fmt.Sprintf("CETS update: %s", eventType),
		Body:    body,
	}
}

func deliveryAddressForEmployee(employeeID string) string {
	employeeID = strings.TrimSpace(strings.ToLower(employeeID))
	if employeeID == "" {
		return "unknown@cets.local"
	}
	return employeeID + "@cets.local"
}

func notificationActivityContext(payload map[string]interface{}) string {
	if len(payload) == 0 {
		return ""
	}
	parts := []string{}
	if title := stringFromPayload(payload, "event_title"); title != "" {
		parts = append(parts, "Activity: "+title+".")
	}
	if startsAt := stringFromPayload(payload, "starts_at"); startsAt != "" {
		parts = append(parts, "Starts at: "+startsAt+".")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}
