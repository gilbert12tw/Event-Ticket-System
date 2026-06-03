package ticketing

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type outboxAttemptLogger func(outcome string)

type outboxProcessingWork struct {
	options     OutboxProcessorOptions
	retryPolicy OutboxRetryPolicy
	claim       outboxClaim
	logAttempt  outboxAttemptLogger
}

func (s *Service) processClaimedOutbox(ctx context.Context, tx pgx.Tx, options OutboxProcessorOptions, retryPolicy OutboxRetryPolicy, claim outboxClaim, logAttempt outboxAttemptLogger) (int, error) {
	work := outboxProcessingWork{options: options, retryPolicy: retryPolicy, claim: claim, logAttempt: logAttempt}
	if !supportedOutboxSchemaVersion(claim.schemaVersion) {
		return s.rejectUnsupportedOutboxSchemaTx(ctx, tx, claim, logAttempt)
	}
	if !supportedOutboxEventType(claim.eventType) {
		return s.rejectUnsupportedOutboxEventTx(ctx, tx, claim, logAttempt)
	}
	payload, validPayload := decodeOutboxPayload(claim.payloadText, claim)
	if !validPayload {
		return markInvalidOutboxPayloadAttempt(ctx, tx, claim, retryPolicy, logAttempt)
	}
	if isReportExportRequestedEventType(claim.eventType) {
		return s.processClaimedReportExportOutbox(ctx, tx, claim, options.ReportStore, retryPolicy, logAttempt)
	}
	if claim.eventType == outboxEventReportingProjectionUpdateRequiredV2 {
		return s.processClaimedProjectionOutbox(ctx, tx, claim, logAttempt)
	}
	employeeID := recipientEmployeeIDForOutbox(claim.eventType, payload)
	if employeeID == "" {
		return markOutboxPublishedAttempt(ctx, tx, claim, logAttempt)
	}
	plan, err := s.prepareOutboxDeliveriesTx(ctx, tx, work, payload, employeeID)
	if err != nil {
		return plan.processed, err
	}
	if plan.processed > 0 {
		return plan.processed, nil
	}
	if err := tx.Commit(ctx); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}
	return s.sendPreparedOutboxEmail(ctx, work, payload, employeeID, plan)
}

func (s *Service) rejectUnsupportedOutboxSchemaTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, logAttempt outboxAttemptLogger) (int, error) {
	s.logUnknownOutboxSchemaVersion(ctx, claim)
	if err := markOutboxDeadLetterInTx(ctx, tx, claim, unsupportedOutboxSchemaVersionError(claim.schemaVersion)); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}
	return commitDeadLetterOutboxAttempt(ctx, tx, logAttempt)
}

func (s *Service) rejectUnsupportedOutboxEventTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, logAttempt outboxAttemptLogger) (int, error) {
	s.logUnknownOutboxEventType(ctx, claim)
	err := markOutboxDeadLetterWithReasonInTx(ctx, tx, claim, unsupportedOutboxEventTypeError(claim.eventType), unknownOutboxEventTypeReason)
	if err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}
	return commitDeadLetterOutboxAttempt(ctx, tx, logAttempt)
}

func commitDeadLetterOutboxAttempt(ctx context.Context, tx pgx.Tx, logAttempt outboxAttemptLogger) (int, error) {
	if err := tx.Commit(ctx); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 1, err
	}
	logAttempt(outboxAttemptOutcomeDeadLetter)
	return 1, nil
}

func markOutboxPublishedAttempt(ctx context.Context, tx pgx.Tx, claim outboxClaim, logAttempt outboxAttemptLogger) (int, error) {
	if err := markOutboxPublishedInTx(ctx, tx, claim); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 1, err
	}
	logAttempt(outboxAttemptOutcomePublished)
	return 1, nil
}

func (s *Service) processClaimedReportExportOutbox(ctx context.Context, tx pgx.Tx, claim outboxClaim, store ReportObjectStore, retryPolicy OutboxRetryPolicy, logAttempt outboxAttemptLogger) (int, error) {
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

type preparedOutboxDeliveryPlan struct {
	processed     int
	emailStatus   string
	emailDelivery notificationDeliveryState
}

func (s *Service) prepareOutboxDeliveriesTx(ctx context.Context, tx pgx.Tx, work outboxProcessingWork, payload map[string]interface{}, employeeID string) (preparedOutboxDeliveryPlan, error) {
	prefs, err := loadOutboxNotificationPreferences(ctx, tx, employeeID)
	if err != nil {
		work.logAttempt(outboxAttemptOutcomeError)
		return preparedOutboxDeliveryPlan{}, err
	}
	category, err := notificationCategoryForOutbox(ctx, tx, work.claim.eventType, payload)
	if err != nil {
		work.logAttempt(outboxAttemptOutcomeError)
		return preparedOutboxDeliveryPlan{}, err
	}
	categorySuppressed := notificationCategorySuppressed(prefs, work.claim.eventType, category)
	deliveryChannels := deliveryChannelsForOutbox(work.claim.eventType, payload, work.options.Sender != nil)
	if !deliveryChannels.valid {
		processed, err := markInvalidOutboxPayloadAttempt(ctx, tx, work.claim, work.retryPolicy, work.logAttempt)
		return preparedOutboxDeliveryPlan{processed: processed}, err
	}
	if deliveryChannels.inApp {
		inAppStatus, inAppReason := deliveryStatusForPreference(prefs.inAppEnabled, categorySuppressed, deliveryStatusSent)
		if _, err := ensureNotificationDelivery(ctx, tx, work.claim.outboxID, employeeID, "in_app", inAppStatus, inAppReason); err != nil {
			work.logAttempt(outboxAttemptOutcomeError)
			return preparedOutboxDeliveryPlan{}, err
		}
	}
	plan := preparedOutboxDeliveryPlan{}
	if deliveryChannels.email {
		desiredEmailStatus, emailReason := deliveryStatusForPreference(prefs.emailEnabled, categorySuppressed, deliveryStatusPending)
		emailDelivery, err := ensureNotificationDelivery(ctx, tx, work.claim.outboxID, employeeID, "email", desiredEmailStatus, emailReason)
		if err != nil {
			work.logAttempt(outboxAttemptOutcomeError)
			return preparedOutboxDeliveryPlan{}, err
		}
		plan.emailStatus = emailDelivery.status
		plan.emailDelivery = emailDelivery
	}
	return plan, nil
}

func (s *Service) sendPreparedOutboxEmail(ctx context.Context, work outboxProcessingWork, payload map[string]interface{}, employeeID string, plan preparedOutboxDeliveryPlan) (int, error) {
	var sendErr error
	lastError := ""
	if plan.emailStatus == deliveryStatusSending {
		if err := s.deadLetterAmbiguousEmailDelivery(ctx, work.claim, plan.emailDelivery.deliveryID, work.retryPolicy); err != nil {
			work.logAttempt(outboxAttemptOutcomeError)
			return 1, err
		}
		work.logAttempt(outboxAttemptOutcomeDeadLetter)
		return 1, nil
	}
	if work.options.Sender != nil && plan.emailStatus == deliveryStatusPending {
		lastError, sendErr = s.sendPendingOutboxEmail(ctx, work, payload, employeeID, plan.emailDelivery)
	}
	if sendErr != nil {
		if err := s.updateOutboxAfterSendFailure(ctx, work.claim, work.retryPolicy, lastError); err != nil {
			work.logAttempt(outboxAttemptOutcomeError)
			return 1, err
		}
		work.logAttempt(outboxFailureOutcome(work.claim, work.retryPolicy))
		return 1, nil
	}
	if err := s.markOutboxPublished(ctx, work.claim); err != nil {
		work.logAttempt(outboxAttemptOutcomeError)
		return 1, err
	}
	work.logAttempt(outboxAttemptOutcomePublished)
	return 1, nil
}

func (s *Service) sendPendingOutboxEmail(ctx context.Context, work outboxProcessingWork, payload map[string]interface{}, employeeID string, emailDelivery notificationDeliveryState) (string, error) {
	if err := ctx.Err(); err != nil {
		work.logAttempt(outboxAttemptOutcomeError)
		return "", err
	}
	message := deliveryMessageForOutbox(work.claim.eventType, employeeID, payload)
	message.IdempotencyKey = emailDelivery.deliveryID
	if err := s.markEmailDeliverySending(ctx, emailDelivery.deliveryID); err != nil {
		work.logAttempt(outboxAttemptOutcomeError)
		return "", err
	}
	sendErr := sendNotificationSafely(ctx, work.options.Sender, message)
	status, lastError := emailDeliveryResult(sendErr, employeeID, work.claim, work.retryPolicy)
	if err := s.updateEmailDeliveryAfterSend(ctx, work.claim.outboxID, status, lastError); err != nil {
		work.logAttempt(outboxAttemptOutcomeError)
		return lastError, err
	}
	return lastError, sendErr
}

func emailDeliveryResult(sendErr error, employeeID string, claim outboxClaim, retryPolicy OutboxRetryPolicy) (string, string) {
	if sendErr == nil {
		return deliveryStatusSent, ""
	}
	status := deliveryStatusFailed
	if retryPolicy.exhausted(claim.attempts) {
		status = deliveryStatusDeadLetter
	}
	return status, redactNotificationDeliveryError(sendErr.Error(), employeeID)
}
