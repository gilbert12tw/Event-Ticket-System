package ticketing

import (
	"context"
	"errors"
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

type outboxAttemptLogger func(outcome string)

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

	if processed, handled, err := s.handleUnsupportedOutboxClaimTx(ctx, tx, claim, logAttempt); handled {
		return processed, err
	}

	payload, validPayload := decodeOutboxPayload(claim.payloadText, claim)
	if !validPayload {
		if err := markOutboxProcessingFailureInTx(ctx, tx, claim, retryPolicy, invalidOutboxPayloadError); err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return 0, err
		}
		if err := tx.Commit(ctx); err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return 1, err
		}
		logAttempt(outboxFailureOutcome(claim, retryPolicy))
		return 1, nil
	}
	if isReportExportRequestedEventType(claim.eventType) {
		return s.processReportExportClaim(ctx, tx, claim, options.ReportStore, retryPolicy, logAttempt)
	}

	return s.processNotificationOutboxClaim(ctx, tx, claim, payload, options.Sender, retryPolicy, logAttempt)
}

func (s *Service) handleUnsupportedOutboxClaimTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, logAttempt outboxAttemptLogger) (int, bool, error) {
	if !supportedOutboxSchemaVersion(claim.schemaVersion) {
		s.logUnknownOutboxSchemaVersion(ctx, claim)
		err := unsupportedOutboxSchemaVersionError(claim.schemaVersion)
		return deadLetterOutboxClaimTx(ctx, tx, claim, err, "", logAttempt)
	}
	if !supportedOutboxEventType(claim.eventType) {
		s.logUnknownOutboxEventType(ctx, claim)
		err := unsupportedOutboxEventTypeError(claim.eventType)
		return deadLetterOutboxClaimTx(ctx, tx, claim, err, unknownOutboxEventTypeReason, logAttempt)
	}
	return 0, false, nil
}

func deadLetterOutboxClaimTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, failure string, reason string, logAttempt outboxAttemptLogger) (int, bool, error) {
	var err error
	if reason != "" {
		err = markOutboxDeadLetterWithReasonInTx(ctx, tx, claim, failure, reason)
	} else {
		err = markOutboxDeadLetterInTx(ctx, tx, claim, failure)
	}
	if err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, true, err
	}
	if err := tx.Commit(ctx); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 1, true, err
	}
	logAttempt(outboxAttemptOutcomeDeadLetter)
	return 1, true, nil
}

func (s *Service) processReportExportClaim(ctx context.Context, tx pgx.Tx, claim outboxClaim, store ReportObjectStore, retryPolicy OutboxRetryPolicy, logAttempt outboxAttemptLogger) (int, error) {
	if err := tx.Commit(ctx); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}
	outcome, err := s.processReportExportOutbox(ctx, claim, store, retryPolicy)
	if err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 1, err
	}
	logAttempt(outcome)
	return 1, nil
}

type outboxNotificationPlan struct {
	employeeID     string
	emailStatus    string
	emailDelivery  notificationDeliveryState
	processed      int
	shouldComplete bool
}

func (s *Service) processNotificationOutboxClaim(ctx context.Context, tx pgx.Tx, claim outboxClaim, payload map[string]interface{}, sender NotificationSender, retryPolicy OutboxRetryPolicy, logAttempt outboxAttemptLogger) (int, error) {
	plan, err := s.prepareNotificationDeliveryTx(ctx, tx, claim, payload, sender != nil, retryPolicy, logAttempt)
	if err != nil || plan.shouldComplete {
		return plan.processed, err
	}
	if err := tx.Commit(ctx); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}
	return s.finishNotificationOutboxClaim(ctx, claim, payload, sender, retryPolicy, plan, logAttempt)
}

func (s *Service) prepareNotificationDeliveryTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, payload map[string]interface{}, canSendEmail bool, retryPolicy OutboxRetryPolicy, logAttempt outboxAttemptLogger) (outboxNotificationPlan, error) {
	employeeID := recipientEmployeeIDForOutbox(claim.eventType, payload)
	if employeeID == "" {
		return s.markOutboxPublishedInCurrentTx(ctx, tx, claim, logAttempt)
	}
	prefs, err := loadOutboxNotificationPreferences(ctx, tx, employeeID)
	if err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return outboxNotificationPlan{}, err
	}
	category, err := notificationCategoryForOutbox(ctx, tx, claim.eventType, payload)
	if err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return outboxNotificationPlan{}, err
	}
	categorySuppressed := notificationCategorySuppressed(prefs, claim.eventType, category)
	deliveryChannels := deliveryChannelsForOutbox(claim.eventType, payload, canSendEmail)
	if !deliveryChannels.valid {
		processed, err := markInvalidOutboxPayloadAttempt(ctx, tx, claim, retryPolicy, logAttempt)
		return outboxNotificationPlan{processed: processed, shouldComplete: true}, err
	}
	plan := outboxNotificationPlan{employeeID: employeeID}
	if deliveryChannels.inApp {
		inAppStatus, inAppReason := deliveryStatusForPreference(prefs.inAppEnabled, categorySuppressed, deliveryStatusSent)
		if _, err := ensureNotificationDelivery(ctx, tx, claim.outboxID, employeeID, "in_app", inAppStatus, inAppReason); err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return outboxNotificationPlan{}, err
		}
	}
	if deliveryChannels.email {
		desiredEmailStatus, emailReason := deliveryStatusForPreference(prefs.emailEnabled, categorySuppressed, deliveryStatusPending)
		emailDelivery, err := ensureNotificationDelivery(ctx, tx, claim.outboxID, employeeID, "email", desiredEmailStatus, emailReason)
		if err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return outboxNotificationPlan{}, err
		}
		plan.emailDelivery = emailDelivery
		plan.emailStatus = emailDelivery.status
	}
	return plan, nil
}

func (s *Service) markOutboxPublishedInCurrentTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, logAttempt outboxAttemptLogger) (outboxNotificationPlan, error) {
	if err := markOutboxPublishedInTx(ctx, tx, claim); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return outboxNotificationPlan{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return outboxNotificationPlan{processed: 1, shouldComplete: true}, err
	}
	logAttempt(outboxAttemptOutcomePublished)
	return outboxNotificationPlan{processed: 1, shouldComplete: true}, nil
}

func (s *Service) finishNotificationOutboxClaim(ctx context.Context, claim outboxClaim, payload map[string]interface{}, sender NotificationSender, retryPolicy OutboxRetryPolicy, plan outboxNotificationPlan, logAttempt outboxAttemptLogger) (int, error) {
	if plan.emailStatus == deliveryStatusSending {
		return s.finishAmbiguousEmailDelivery(ctx, claim, plan.emailDelivery.deliveryID, retryPolicy, logAttempt)
	}
	sendErr, lastError, err := s.sendPendingEmailDelivery(ctx, claim, payload, sender, retryPolicy, plan, logAttempt)
	if err != nil {
		return 1, err
	}
	return s.publishOrRetryNotificationOutbox(ctx, claim, retryPolicy, sendErr, lastError, logAttempt)
}

func (s *Service) finishAmbiguousEmailDelivery(ctx context.Context, claim outboxClaim, deliveryID string, retryPolicy OutboxRetryPolicy, logAttempt outboxAttemptLogger) (int, error) {
	if err := s.deadLetterAmbiguousEmailDelivery(ctx, claim, deliveryID, retryPolicy); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 1, err
	}
	logAttempt(outboxAttemptOutcomeDeadLetter)
	return 1, nil
}

func (s *Service) sendPendingEmailDelivery(ctx context.Context, claim outboxClaim, payload map[string]interface{}, sender NotificationSender, retryPolicy OutboxRetryPolicy, plan outboxNotificationPlan, logAttempt outboxAttemptLogger) (error, string, error) {
	var sendErr error
	lastError := ""
	if sender != nil && plan.emailStatus == deliveryStatusPending {
		if err := ctx.Err(); err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return nil, "", err
		}
		message := deliveryMessageForOutbox(claim.eventType, plan.employeeID, payload)
		message.IdempotencyKey = plan.emailDelivery.deliveryID
		if err := s.markEmailDeliverySending(ctx, plan.emailDelivery.deliveryID); err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return nil, "", err
		}
		sendErr = sendNotificationSafely(ctx, sender, message)
		status := deliveryStatusSent
		if sendErr != nil {
			status = deliveryStatusFailed
			if retryPolicy.exhausted(claim.attempts) {
				status = deliveryStatusDeadLetter
			}
			lastError = redactNotificationDeliveryError(sendErr.Error(), plan.employeeID)
		}
		if err := s.updateEmailDeliveryAfterSend(ctx, claim.outboxID, status, lastError); err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return nil, "", err
		}
	}
	return sendErr, lastError, nil
}

func (s *Service) publishOrRetryNotificationOutbox(ctx context.Context, claim outboxClaim, retryPolicy OutboxRetryPolicy, sendErr error, lastError string, logAttempt outboxAttemptLogger) (int, error) {
	if sendErr != nil {
		if err := s.updateOutboxAfterSendFailure(ctx, claim, retryPolicy, lastError); err != nil {
			logAttempt(outboxAttemptOutcomeError)
			return 1, err
		}
		logAttempt(outboxFailureOutcome(claim, retryPolicy))
		return 1, nil
	}
	if err := s.markOutboxPublished(ctx, claim); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 1, err
	}
	logAttempt(outboxAttemptOutcomePublished)
	return 1, nil
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
