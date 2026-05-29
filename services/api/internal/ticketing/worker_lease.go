package ticketing

import (
	"context"
	"errors"
	"strings"
	"time"

	"event-ticket-system/internal/traceid"
)

const defaultOutboxLeaseTTL = time.Minute
const outboxLeaseReleaseTimeout = 5 * time.Second

var errOutboxLeaseLost = errors.New("outbox lease ownership lost")

func outboxLeaseTTLFromOptions(options OutboxProcessorOptions) time.Duration {
	if options.LeaseTTL <= 0 {
		return defaultOutboxLeaseTTL
	}
	return options.LeaseTTL
}

func (s *Service) releaseOutboxLeaseAfterContextCancel(ctx context.Context, claim outboxClaim, attemptErr error) {
	if attemptErr == nil || ctx.Err() == nil || claim.outboxID == "" {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), outboxLeaseReleaseTimeout)
	defer cancel()
	if err := s.releaseOutboxLease(releaseCtx, claim); err != nil {
		s.logOutboxLeaseReleaseFailure(ctx, claim, err)
	}
}

func (s *Service) releaseOutboxLease(ctx context.Context, claim outboxClaim) error {
	_, err := s.db.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'pending',
			lease_started_at = NULL,
			available_at = now()
		WHERE outbox_id = $1
			AND publish_status = 'processing'
			AND lease_started_at = $2`, claim.outboxID, claim.leaseStartedAt)
	return err
}

func (s *Service) logOutboxLeaseReleaseFailure(ctx context.Context, claim outboxClaim, failure error) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn("outbox lease release failed",
		"trace_id", traceid.FromContext(ctx),
		"event_id", claim.outboxID,
		"event_type", safeOutboxTelemetryEventType(claim.eventType),
		"worker_kind", safeOutboxTelemetryWorkerKind(claim.eventType),
		"error", safeOutboxWorkerError(failure),
	)
}

func safeOutboxWorkerError(failure error) string {
	if failure == nil {
		return ""
	}
	message := strings.TrimSpace(failure.Error())
	if message == "" {
		return "outbox worker error"
	}
	message = notificationEmailBodyPattern.ReplaceAllString(message, "$1=[redacted email body]")
	message = notificationEmailPattern.ReplaceAllString(message, "[redacted email]")
	message = outboxTelemetryTokenPattern.ReplaceAllString(message, "[redacted token]")
	message = outboxTelemetryEmployeePattern.ReplaceAllString(message, "[redacted employee]")
	return message
}
