package ticketing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	unknownOutboxSchemaVersionReason = "unknown_schema_version"
	unknownOutboxEventTypeReason     = "unknown_event_type"
)

func supportedOutboxSchemaVersion(version int) bool {
	return version == 1 || version == 2
}

func supportedOutboxEventType(eventType string) bool {
	return knownReplayEventType(eventType) && !unsafeOutboxTelemetryEventType(eventType)
}

func unsupportedOutboxSchemaVersionError(version int) string {
	return fmt.Sprintf("reason=%s unsupported outbox schema_version=%d", unknownOutboxSchemaVersionReason, version)
}

func unsupportedOutboxEventTypeError(eventType string) string {
	return fmt.Sprintf("reason=%s unsupported outbox event_type=%s", unknownOutboxEventTypeReason, safeOutboxTelemetryEventType(eventType))
}

func markOutboxDeadLetterInTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, lastError string) error {
	return markOutboxDeadLetterWithReasonInTx(ctx, tx, claim, lastError, unknownOutboxSchemaVersionReason)
}

func markOutboxDeadLetterWithReasonInTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, lastError string, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'dead_letter',
			last_error = $1,
			retry_count = $2,
			dead_letter_at = now(),
			lease_started_at = NULL,
			available_at = now()
		WHERE outbox_id = $3
			AND publish_status = 'processing'
			AND lease_started_at = $4`, lastError, claim.attempts, claim.outboxID, claim.leaseStartedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errOutboxLeaseLost
	}
	return insertOutboxDeadLetterAuditInTx(ctx, tx, claim, reason)
}
