package ticketing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	outboxDeadLetterAction               = "outbox.dead_letter"
	outboxDeadLetterEntityType           = "outbox_event"
	outboxDeadLetterReasonAmbiguousEmail = "ambiguous_email_delivery"
	outboxDeadLetterReasonInvalidPayload = "invalid_payload"
	outboxDeadLetterReasonRetryExhausted = "retry_exhausted"
	outboxRetryScheduledAction           = "outbox.retry_scheduled"
	outboxRetryReasonRetryableFailure    = "retryable_failure"
)

const ambiguousEmailDeliveryError = "ambiguous email delivery state after worker crash before provider acknowledgement"

func (s *Service) updateOutboxAfterSendFailure(ctx context.Context, claim outboxClaim, retryPolicy OutboxRetryPolicy, lastError string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	status := "pending"
	if retryPolicy.exhausted(claim.attempts) {
		status = "dead_letter"
	}
	if err := updateOutboxFailureInTx(ctx, tx, claim, retryPolicy, status, lastError); err != nil {
		return err
	}
	if status == "dead_letter" {
		if err := insertOutboxDeadLetterAuditInTx(ctx, tx, claim, outboxDeadLetterReasonRetryExhausted); err != nil {
			return err
		}
	} else if err := insertOutboxRetryScheduledAuditInTx(ctx, tx, claim, outboxRetryReasonRetryableFailure); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) deadLetterAmbiguousEmailDelivery(ctx context.Context, claim outboxClaim, deliveryID string, retryPolicy OutboxRetryPolicy) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	tag, err := tx.Exec(ctx, `UPDATE notification_deliveries
		SET status = 'dead_letter',
			last_error = $1,
			updated_at = now()
		WHERE delivery_id = $2 AND status = 'sending'`, ambiguousEmailDeliveryError, deliveryID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ambiguous email delivery was not dead-lettered")
	}
	if err := updateOutboxFailureInTx(ctx, tx, claim, retryPolicy, "dead_letter", ambiguousEmailDeliveryError); err != nil {
		return err
	}
	if err := insertOutboxDeadLetterAuditInTx(ctx, tx, claim, outboxDeadLetterReasonAmbiguousEmail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func updateOutboxFailureInTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, retryPolicy OutboxRetryPolicy, status string, lastError string) error {
	tag, err := tx.Exec(ctx, `UPDATE outbox_events
		SET publish_status = $1,
			last_error = $2,
			retry_count = $3,
			dead_letter_at = CASE WHEN $1 = 'dead_letter' THEN now() ELSE NULL END,
			lease_started_at = NULL,
			available_at = now() + ($4::double precision * interval '1 second')
		WHERE outbox_id = $5
			AND publish_status = 'processing'
			AND lease_started_at = $6`,
		status, lastError, claim.attempts, retryPolicy.backoffForOutbox(claim.outboxID, claim.attempts).Seconds(), claim.outboxID, claim.leaseStartedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errOutboxLeaseLost
	}
	return nil
}

func insertOutboxDeadLetterAuditInTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, reason string) error {
	return insertOutboxFailureAuditInTx(ctx, tx, outboxDeadLetterAction, claim, reason)
}

func insertOutboxRetryScheduledAuditInTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, reason string) error {
	return insertOutboxFailureAuditInTx(ctx, tx, outboxRetryScheduledAction, claim, reason)
}

func insertOutboxFailureAuditInTx(ctx context.Context, tx pgx.Tx, action string, claim outboxClaim, reason string) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	return insertAudit(ctx, tx, newAuditRecord(auditID, outboxWorkerAuditActor(), action, outboxDeadLetterEntityType, claim.outboxID, map[string]interface{}{
		"event_type":     safeOutboxTelemetryEventType(claim.eventType),
		"worker_kind":    safeOutboxTelemetryWorkerKind(claim.eventType),
		"retry_count":    claim.attempts,
		"schema_version": claim.schemaVersion,
		"reason":         reason,
	}))
}

func outboxWorkerAuditActor() Actor {
	return Actor{ID: "outbox-worker", Role: RoleSystemAdmin}
}
