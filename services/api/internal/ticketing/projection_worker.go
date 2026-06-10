package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

const projectionProjectionName = "event_summary"

// processClaimedProjectionOutbox is the top-level handler for a claimed
// reporting.projection.update_required.v2 outbox event.
//
// Flow:
//  1. Decode inner payload → ProjectionEvent
//  2. Read current aggregate row
//  3. Compute new counts
//  4. Upsert reporting_event_summary (idempotent)
//  5. Advance reporting_projection_offsets watermark
//  6. Record lag metric
//  7. Mark outbox published
func (s *Service) processClaimedProjectionOutbox(
	ctx context.Context,
	tx pgx.Tx,
	claim outboxClaim,
	logAttempt outboxAttemptLogger,
) (int, error) {
	proj, ok := decodeProjectionEvent(claim)
	if !ok {
		return s.skipProjectionEvent(ctx, tx, claim, logAttempt, "decode_failed")
	}

	proj, done, result, err := s.resolveProjectionInnerType(ctx, tx, claim, logAttempt, proj)
	if done {
		return result, err
	}

	switch proj.InnerType {
	case projectionInnerTypeCheckinCompleted:
		// checkin has no aggregate effect — advance offset and mark published.
		return s.applyProjectionCheckin(ctx, tx, claim, logAttempt, proj)
	case projectionInnerTypeBookingConfirmed,
		projectionInnerTypeBookingCancelled,
		projectionInnerTypeBookingWaitlisted,
		projectionInnerTypeBookingWaitlistCancel:
		return s.applyProjectionBooking(ctx, tx, claim, logAttempt, proj)
	default:
		return s.skipProjectionEvent(ctx, tx, claim, logAttempt, "unknown_inner_type:"+proj.InnerType)
	}
}

// resolveProjectionInnerType fills in proj.InnerType for schema_version=2
// envelopes by looking up the trigger event's type. It returns done=true when
// the caller should stop (the event was skipped gracefully or a hard error
// occurred), in which case (result, err) are the values to return.
func (s *Service) resolveProjectionInnerType(
	ctx context.Context,
	tx pgx.Tx,
	claim outboxClaim,
	logAttempt outboxAttemptLogger,
	proj ProjectionEvent,
) (ProjectionEvent, bool, int, error) {
	if proj.InnerType != "" || proj.TriggerEventID == "" {
		return proj, false, 0, nil
	}

	var rawEventType string
	err := tx.QueryRow(ctx, `SELECT event_type FROM outbox_events WHERE outbox_id = $1`, proj.TriggerEventID).Scan(&rawEventType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Trigger row already published or GC'd — skip gracefully.
			result, skipErr := s.skipProjectionEvent(ctx, tx, claim, logAttempt, "trigger_event_gc")
			return proj, true, result, skipErr
		}
		logAttempt(outboxAttemptOutcomeError)
		return proj, true, 0, err
	}

	proj.InnerType = normalizeProjectionTriggerType(rawEventType)
	if proj.InnerType == "" {
		result, skipErr := s.skipProjectionEvent(ctx, tx, claim, logAttempt, "decode_failed")
		return proj, true, result, skipErr
	}
	return proj, false, 0, nil
}

// applyProjectionCheckin advances the offset for a checkin event, which has no
// aggregate effect, then marks the outbox row published.
func (s *Service) applyProjectionCheckin(
	ctx context.Context,
	tx pgx.Tx,
	claim outboxClaim,
	logAttempt outboxAttemptLogger,
	proj ProjectionEvent,
) (int, error) {
	if err := advanceProjectionOffset(ctx, tx, projectionProjectionName, proj.OutboxID); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}
	s.observeProjectionProcessed(claim)
	return markOutboxPublishedAttempt(ctx, tx, claim, logAttempt)
}

// applyProjectionBooking applies a booking event to the aggregate row, advances
// the offset, and marks the outbox row published.
func (s *Service) applyProjectionBooking(
	ctx context.Context,
	tx pgx.Tx,
	claim outboxClaim,
	logAttempt outboxAttemptLogger,
	proj ProjectionEvent,
) (int, error) {
	current, err := getEventSummaryRow(ctx, tx, proj.EventID)
	if err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}

	newCounts := computeNewCounts(current, proj)
	// proj.OutboxID is the time-ordered composite offset (see projectionOffsetKey),
	// NOT the raw random outbox_id — it is what the upsert's lexicographic guard
	// compares against, so older events never overwrite newer aggregates.
	if err := upsertEventSummary(ctx, tx, proj.EventID, newCounts, proj.OutboxID); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}

	if err := advanceProjectionOffset(ctx, tx, projectionProjectionName, proj.OutboxID); err != nil {
		logAttempt(outboxAttemptOutcomeError)
		return 0, err
	}

	s.observeProjectionProcessed(claim)
	return markOutboxPublishedAttempt(ctx, tx, claim, logAttempt)
}

// skipProjectionEvent records a skip (with reason), counts the event as
// processed, and marks the outbox row published so it is not retried.
func (s *Service) skipProjectionEvent(
	ctx context.Context,
	tx pgx.Tx,
	claim outboxClaim,
	logAttempt outboxAttemptLogger,
	reason string,
) (int, error) {
	s.logProjectionSkip(ctx, claim, reason)
	s.incrementProjectionProcessed()
	return markOutboxPublishedAttempt(ctx, tx, claim, logAttempt)
}

// incrementProjectionProcessed counts one processed projection event.
func (s *Service) incrementProjectionProcessed() {
	if s != nil && s.metrics != nil {
		s.metrics.IncrementProjectionProcessed()
	}
}

// observeProjectionProcessed records the projection lag for claim and counts
// the event as processed.
func (s *Service) observeProjectionProcessed(claim outboxClaim) {
	if s != nil && s.metrics != nil {
		s.metrics.ObserveProjectionLag(time.Since(claim.createdAt))
		s.metrics.IncrementProjectionProcessed()
	}
}

// computeNewCounts derives new aggregate counts from the current row and a
// projection event. Counts never go below 0.
func computeNewCounts(current eventSummaryRow, proj ProjectionEvent) eventSummaryRow {
	next := eventSummaryRow{
		ConfirmedCount:  current.ConfirmedCount,
		CancelledCount:  current.CancelledCount,
		WaitlistCount:   current.WaitlistCount,
		EmployeeCount:   current.EmployeeCount,
		FamilyCount:     current.FamilyCount,
		TicketCount:     current.TicketCount,
		CheckinCount:    current.CheckinCount,
		LastEventOffset: proj.OutboxID,
	}
	// Deep-copy breakdown so we do not mutate the input.
	next.DepartmentBreakdown = make(map[string]int, len(current.DepartmentBreakdown))
	for k, v := range current.DepartmentBreakdown {
		next.DepartmentBreakdown[k] = v
	}
	dept := strings.TrimSpace(proj.Department)
	switch proj.InnerType {
	case projectionInnerTypeBookingConfirmed:
		next.ConfirmedCount++
		if dept != "" {
			next.DepartmentBreakdown[dept]++
		}
	case projectionInnerTypeBookingCancelled:
		next.ConfirmedCount = max(0, next.ConfirmedCount-1)
		next.CancelledCount++
		if dept != "" && next.DepartmentBreakdown[dept] > 0 {
			next.DepartmentBreakdown[dept] = max(0, next.DepartmentBreakdown[dept]-1)
		}
	case projectionInnerTypeBookingWaitlisted:
		next.WaitlistCount++
	case projectionInnerTypeBookingWaitlistCancel:
		next.WaitlistCount = max(0, next.WaitlistCount-1)
	}
	return next
}

// decodeProjectionEvent extracts the relevant fields from a projection outbox
// claim. Supports both:
//   - schema_version=2 envelope: outer JSON has {"payload": {"aggregate_id": ..., "trigger_event_id": ...}}
//   - schema_version=1 / test-seeded payloads: flat JSON with aggregate_id + inner_event_type
//
// Returns false on decode failure — callers skip without dead-lettering.
func decodeProjectionEvent(claim outboxClaim) (ProjectionEvent, bool) {
	if strings.TrimSpace(claim.eventType) != outboxEventReportingProjectionUpdateRequiredV2 {
		return ProjectionEvent{}, false
	}
	// v2 envelope: {"event_id":..., "payload": {"aggregate_id":..., "trigger_event_id":..., ...}}
	if claim.schemaVersion == 2 {
		var v2 struct {
			Payload struct {
				AggregateID    string `json:"aggregate_id"`
				TriggerEventID string `json:"trigger_event_id"`
				Department     string `json:"department"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(claim.payloadText), &v2); err == nil {
			eventID := strings.TrimSpace(v2.Payload.AggregateID)
			triggerEventID := strings.TrimSpace(v2.Payload.TriggerEventID)
			if eventID != "" && triggerEventID != "" {
				return ProjectionEvent{
					EventID:        eventID,
					OutboxID:       projectionOffsetKey(claim.createdAt, claim.outboxID),
					TriggerEventID: triggerEventID,
					Department:     strings.TrimSpace(v2.Payload.Department),
				}, true
			}
		}
	}
	// v1 / test-seeded payloads: flat JSON with aggregate_id + inner_event_type
	var v1 struct {
		AggregateID string `json:"aggregate_id"`
		InnerType   string `json:"inner_event_type"`
		Department  string `json:"department"`
	}
	if err := json.Unmarshal([]byte(claim.payloadText), &v1); err != nil {
		return ProjectionEvent{}, false
	}
	eventID := strings.TrimSpace(v1.AggregateID)
	innerType := normalizeProjectionTriggerType(v1.InnerType)
	if eventID == "" || innerType == "" {
		return ProjectionEvent{}, false
	}
	return ProjectionEvent{
		EventID:    eventID,
		OutboxID:   projectionOffsetKey(claim.createdAt, claim.outboxID),
		InnerType:  innerType,
		Department: strings.TrimSpace(v1.Department),
	}, true
}

// normalizeProjectionTriggerType strips .v<N> version suffixes from trigger
// event types so they match the canonical inner-type constants.
// For example, it converts a string ending in ".v2" to omit the suffix.
func normalizeProjectionTriggerType(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.LastIndex(raw, ".v"); idx > 0 {
		candidate := raw[idx+2:] // digits after ".v"
		allDigits := len(candidate) > 0
		for _, c := range candidate {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return raw[:idx]
		}
	}
	return raw
}

func (s *Service) logProjectionSkip(ctx context.Context, claim outboxClaim, reason string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn("projection worker skipping event",
		"trace_id", traceid.FromContext(ctx),
		"outbox_id", claim.outboxID,
		"event_type", safeOutboxTelemetryEventType(claim.eventType),
		"reason", reason,
		"worker_kind", outboxWorkerKindProjection,
	)
}
