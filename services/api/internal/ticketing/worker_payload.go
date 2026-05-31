package ticketing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const invalidOutboxPayloadError = "invalid outbox payload"

func decodeOutboxPayload(raw string, claim outboxClaim) (map[string]interface{}, bool) {
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload == nil {
		return nil, false
	}
	if claim.schemaVersion == 2 {
		return unwrapOutboxV2EnvelopePayload(payload, claim)
	}
	return payload, true
}

func unwrapOutboxV2EnvelopePayload(payload map[string]interface{}, claim outboxClaim) (map[string]interface{}, bool) {
	if !validOutboxV2EnvelopeMetadata(payload, claim) {
		return nil, false
	}
	value, ok := payload["payload"]
	if !ok {
		return nil, false
	}
	nested, ok := value.(map[string]interface{})
	if !ok || nested == nil {
		return nil, false
	}
	return nested, true
}

func validOutboxV2EnvelopeMetadata(payload map[string]interface{}, claim outboxClaim) bool {
	if stringFromPayload(payload, "event_id") != strings.TrimSpace(claim.outboxID) {
		return false
	}
	if stringFromPayload(payload, "event_type") != strings.TrimSpace(claim.eventType) {
		return false
	}
	if !validOutboxV2OccurredAt(stringFromPayload(payload, "occurred_at")) {
		return false
	}
	if stringFromPayload(payload, "idempotency_key") != strings.TrimSpace(claim.idempotencyKey) {
		return false
	}
	if stringFromPayload(payload, "partition_key") != strings.TrimSpace(claim.partitionKey) {
		return false
	}
	version, ok := payload["schema_version"].(float64)
	return ok && version == float64(claim.schemaVersion)
}

func validOutboxV2OccurredAt(value string) bool {
	if !strings.HasSuffix(value, "Z") {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func markOutboxProcessingFailureInTx(ctx context.Context, tx pgx.Tx, claim outboxClaim, retryPolicy OutboxRetryPolicy, lastError string) error {
	status := "pending"
	if retryPolicy.exhausted(claim.attempts) {
		status = "dead_letter"
	}
	if err := updateOutboxFailureInTx(ctx, tx, claim, retryPolicy, status, lastError); err != nil {
		return err
	}
	if status == "dead_letter" {
		return insertOutboxDeadLetterAuditInTx(ctx, tx, claim, outboxDeadLetterReasonInvalidPayload)
	}
	return insertOutboxRetryScheduledAuditInTx(ctx, tx, claim, outboxRetryReasonRetryableFailure)
}

func markInvalidOutboxPayloadAttempt(ctx context.Context, tx pgx.Tx, claim outboxClaim, retryPolicy OutboxRetryPolicy, logAttempt func(string)) (int, error) {
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
