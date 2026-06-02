package ticketing

import (
	"context"
	"errors"
	"time"

	"event-ticket-system/internal/ratelimit"
	"event-ticket-system/internal/reservation"
	"event-ticket-system/internal/traceid"
)

const bookingRateLimitedAction = "booking.rate_limited"

func (s *Service) rateLimitBooking(ctx context.Context, actor Actor, eventID, employeeID string) error {
	if s.bookingLimiter == nil || !s.bookingLimiter.Enabled() {
		return nil
	}
	actorHash := reservation.ActorHash(s.rateLimitSecret, employeeID)
	decision, err := s.bookingLimiter.Allow(ctx, eventID, actorHash)
	if err != nil {
		if errors.Is(err, ratelimit.ErrUnavailable) {
			return serviceUnavailable("BOOKING_RATE_LIMITER_UNAVAILABLE", "booking rate limiter unavailable; retry shortly")
		}
		return err
	}
	if decision.Allowed {
		return nil
	}
	retryAfter := retryAfterSeconds(decision.RetryAfter)
	if err := s.insertBookingRateLimitAudit(ctx, actor, eventID, decision.Scope, retryAfter); err != nil {
		return err
	}
	s.logger.Info("booking rate limited",
		"trace_id", traceid.FromContext(ctx),
		"event_id", eventID,
		"scope", decision.Scope,
		"retry_after_seconds", retryAfter,
	)
	return rateLimited("BOOKING_RATE_LIMITED", "booking rate limit exceeded; retry shortly", retryAfter)
}

func (s *Service) insertBookingRateLimitAudit(ctx context.Context, actor Actor, eventID string, scope ratelimit.Scope, retryAfter int) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"event_id":            eventID,
		"scope":               string(scope),
		"retry_after_seconds": retryAfter,
	}
	return insertAuditWithExecutor(ctx, s.db, newAuditRecord(auditID, actor, bookingRateLimitedAction, "event", eventID, metadata))
}

func retryAfterSeconds(value time.Duration) int {
	seconds := int(value.Round(time.Second) / time.Second)
	if seconds <= 0 {
		return 1
	}
	return seconds
}
