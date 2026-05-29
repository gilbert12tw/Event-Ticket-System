package ticketing

import (
	"context"
	"time"

	"event-ticket-system/internal/traceid"
)

const (
	outboxAttemptOutcomePublished  = "published"
	outboxAttemptOutcomeRetry      = "retry"
	outboxAttemptOutcomeDeadLetter = "dead_letter"
	outboxAttemptOutcomeError      = "error"
)

func (s *Service) logOutboxWorkerAttempt(ctx context.Context, claim outboxClaim, startedAt time.Time, outcome string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info("outbox worker attempt",
		"trace_id", traceid.FromContext(ctx),
		"event_id", claim.outboxID,
		"event_type", safeOutboxTelemetryEventType(claim.eventType),
		"worker_kind", safeOutboxTelemetryWorkerKind(claim.eventType),
		"retry_count", claim.attempts,
		"outcome", outcome,
		"latency_ms", time.Since(startedAt).Milliseconds(),
	)
}

func (s *Service) logUnknownOutboxSchemaVersion(ctx context.Context, claim outboxClaim) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn("unknown outbox schema version",
		"trace_id", traceid.FromContext(ctx),
		"event_id", claim.outboxID,
		"event_type", safeOutboxTelemetryEventType(claim.eventType),
		"worker_kind", safeOutboxTelemetryWorkerKind(claim.eventType),
		"schema_version", claim.schemaVersion,
		"reason", unknownOutboxSchemaVersionReason,
	)
}

func (s *Service) logUnknownOutboxEventType(ctx context.Context, claim outboxClaim) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn("unknown outbox event type",
		"trace_id", traceid.FromContext(ctx),
		"event_id", claim.outboxID,
		"event_type", safeOutboxTelemetryEventType(claim.eventType),
		"worker_kind", safeOutboxTelemetryWorkerKind(claim.eventType),
		"schema_version", claim.schemaVersion,
		"reason", unknownOutboxEventTypeReason,
	)
}

func outboxFailureOutcome(claim outboxClaim, retryPolicy OutboxRetryPolicy) string {
	if retryPolicy.exhausted(claim.attempts) {
		return outboxAttemptOutcomeDeadLetter
	}
	return outboxAttemptOutcomeRetry
}
