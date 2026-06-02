package ticketing

import (
	"context"
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
	notificationSuppressedByPrefs = "suppressed by notification preferences"
)

type DeliveryMessage struct {
	To             string
	Subject        string
	Body           string
	IdempotencyKey string
}

type NotificationSender interface {
	Send(ctx context.Context, message DeliveryMessage) error
}

type OutboxProcessorOptions struct {
	Sender      NotificationSender
	ReportStore ReportObjectStore
	MaxAttempts int
	BatchSize   int
	WorkerKinds []string
	LeaseTTL    time.Duration
	RetryPolicy *OutboxRetryPolicy
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
	_ = client.Quit()
	return nil
}

func (s SMTPNotificationSender) deliveryRecipient(message DeliveryMessage) string {
	if recipient := strings.TrimSpace(s.RedirectTo); recipient != "" {
		return recipient
	}
	return message.To
}

func (s SMTPNotificationSender) deliveryBody(message DeliveryMessage, recipient string) string {
	headers := []string{
		fmt.Sprintf("From: %s", s.From),
		fmt.Sprintf("To: %s", recipient),
		fmt.Sprintf("Subject: %s", message.Subject),
	}
	if key := strings.TrimSpace(message.IdempotencyKey); key != "" {
		headers = append(headers, fmt.Sprintf("Message-ID: <%s@cets.local>", key))
		headers = append(headers, fmt.Sprintf("X-Idempotency-Key: %s", key))
	}
	return fmt.Sprintf("%s\r\n\r\n%s\r\n", strings.Join(headers, "\r\n"), message.Body)
}

func (s *Service) ProcessOutboxOnce(ctx context.Context, sender NotificationSender, maxAttempts int) (int, error) {
	return s.ProcessOutboxOnceWithOptions(ctx, OutboxProcessorOptions{
		Sender:      sender,
		MaxAttempts: maxAttempts,
	})
}

func (s *Service) ProcessOutboxOnceWithOptions(ctx context.Context, options OutboxProcessorOptions) (int, error) {
	retryPolicy := outboxRetryPolicyFromOptions(options)
	leaseTTL := outboxLeaseTTLFromOptions(options)
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	processed := 0
	for processed < batchSize {
		count, err := s.processOneOutbox(ctx, options, retryPolicy, leaseTTL)
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

func (s *Service) processOneOutbox(ctx context.Context, options OutboxProcessorOptions, retryPolicy OutboxRetryPolicy, leaseTTL time.Duration) (processed int, err error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer rollback(ctx, tx)

	claim, err := claimOutboxEvent(ctx, tx, options.WorkerKinds, leaseTTL)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer func() {
		s.releaseOutboxLeaseAfterContextCancel(ctx, claim, err)
	}()
	startedAt := time.Now()
	logAttempt := func(outcome string) {
		s.logOutboxWorkerAttempt(ctx, claim, startedAt, outcome)
	}

	return s.processClaimedOutbox(ctx, tx, options, retryPolicy, claim, logAttempt)
}

type outboxClaim struct {
	outboxID       string
	aggregateID    string
	eventType      string
	payloadText    string
	attempts       int
	schemaVersion  int
	idempotencyKey string
	partitionKey   string
	leaseStartedAt time.Time
}

type outboxNotificationPreferences struct {
	emailEnabled       bool
	inAppEnabled       bool
	optedOutCategories []string
}

const (
	deliveryStatusPending    = "pending"
	deliveryStatusSending    = "sending"
	deliveryStatusSent       = "sent"
	deliveryStatusFailed     = "failed"
	deliveryStatusSuppressed = "suppressed"
	deliveryStatusDeadLetter = "dead_letter"
)

type notificationDeliveryState struct {
	deliveryID string
	status     string
}

func claimOutboxEvent(ctx context.Context, tx pgx.Tx, workerKinds []string, leaseTTL time.Duration) (outboxClaim, error) {
	var claim outboxClaim
	err := tx.QueryRow(ctx, `WITH next_outbox AS (
			SELECT outbox_id
			FROM outbox_events
			WHERE available_at <= now()
				AND publish_status IN ('pending', 'processing')
				AND (
					cardinality($2::text[]) = 0
					OR CASE
						WHEN event_type IN ('report.export.requested', 'report.export.requested.v2') THEN 'export'
						WHEN event_type = 'reporting.projection.update_required.v2' THEN 'projection'
						WHEN event_type LIKE 'reservation.compensation.%' THEN 'compensation'
						ELSE 'notification'
					END = ANY($2::text[])
				)
			ORDER BY CASE WHEN event_type IN ('report.export.requested', 'report.export.requested.v2') THEN 0 ELSE 1 END,
				created_at ASC,
				outbox_id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events outbox
		SET publish_status = 'processing',
			attempts = attempts + 1,
			last_error = '',
			available_at = now() + ($1::double precision * interval '1 second'),
			lease_started_at = now()
		FROM next_outbox
		WHERE outbox.outbox_id = next_outbox.outbox_id
		RETURNING outbox.outbox_id, outbox.aggregate_id, outbox.event_type, outbox.payload::text,
			outbox.attempts, outbox.schema_version, COALESCE(outbox.idempotency_key, ''),
			COALESCE(outbox.partition_key, ''), outbox.lease_started_at`,
		leaseTTL.Seconds(), outboxWorkerKindFilter(workerKinds)).
		Scan(
			&claim.outboxID,
			&claim.aggregateID,
			&claim.eventType,
			&claim.payloadText,
			&claim.attempts,
			&claim.schemaVersion,
			&claim.idempotencyKey,
			&claim.partitionKey,
			&claim.leaseStartedAt,
		)
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
