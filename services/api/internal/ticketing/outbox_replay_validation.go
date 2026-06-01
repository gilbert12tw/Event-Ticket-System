package ticketing

import (
	"event-ticket-system/internal/eventcontract"
	"fmt"
	"sort"
	"strings"
)

const outboxWorkerKindReservationCompensation = "reservation_compensation"

var registeredReplayEventTypes = buildRegisteredReplayEventTypes()

func ValidateReplayOutboxRequest(req ReplayOutboxRequest) (ReplayOutboxRequest, error) {
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	if !supportedReplayWorkerKind(req.Kind) {
		return ReplayOutboxRequest{}, badRequest("worker kind is invalid")
	}
	if req.From.IsZero() {
		return ReplayOutboxRequest{}, badRequest("replay from time is required")
	}
	if req.To.IsZero() {
		return ReplayOutboxRequest{}, badRequest("replay to time is required")
	}
	req.From = req.From.UTC()
	req.To = req.To.UTC()
	if !req.To.After(req.From) {
		return ReplayOutboxRequest{}, badRequest("replay to time must be after from time")
	}
	eventTypes, err := normalizeReplayEventTypes(req.Kind, req.EventTypes)
	if err != nil {
		return ReplayOutboxRequest{}, err
	}
	req.EventTypes = eventTypes
	return req, nil
}

func supportedReplayWorkerKind(kind string) bool {
	switch kind {
	case outboxWorkerKindNotification,
		outboxWorkerKindProjection,
		outboxWorkerKindCompensation,
		outboxWorkerKindExport,
		outboxWorkerKindReservationCompensation:
		return true
	default:
		return false
	}
}

func normalizeReplayEventTypes(kind string, eventTypes []string) ([]string, error) {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			return nil, badRequest("replay event type is invalid")
		}
		if !knownReplayEventType(eventType) {
			return nil, badRequest("replay event type is invalid")
		}
		if !replayKindMatchesEventType(kind, eventType) {
			return nil, badRequest(fmt.Sprintf("event type %q does not belong to worker kind %q", eventType, kind))
		}
		if _, ok := seen[eventType]; ok {
			continue
		}
		seen[eventType] = struct{}{}
		normalized = append(normalized, eventType)
	}
	return normalized, nil
}

func buildRegisteredReplayEventTypes() map[string]struct{} {
	registered := map[string]struct{}{
		"booking.confirmed":                 {},
		"booking.received":                  {},
		"booking.waitlisted":                {},
		"eligibility.impact_review.created": {},
		"event.updated":                     {},
		"hr_sync.completed":                 {},
		"lottery.completed":                 {},
		"registration.cancelled":            {},
		"registration.no_show_recorded":     {},
		outboxEventReportExportRequested:    {},
		"ticket.expired":                    {},
		"ticket.issued":                     {},
		"ticket.redeemed":                   {},
		"ticket.revoked":                    {},
		"waitlist.promoted":                 {},
	}
	for _, eventType := range eventcontract.Registry {
		registered[eventType] = struct{}{}
	}
	return registered
}

func knownReplayEventType(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	_, ok := registeredReplayEventTypes[eventType]
	return ok
}

func replayKindMatchesEventType(kind string, eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	if kind == outboxWorkerKindReservationCompensation {
		return strings.HasPrefix(eventType, "reservation.compensation.")
	}
	return outboxWorkerKindForEventType(eventType) == replaySQLWorkerKind(kind)
}

func replayEventTypesForKind(kind string) []string {
	eventTypes := make([]string, 0, len(registeredReplayEventTypes))
	for eventType := range registeredReplayEventTypes {
		if replayKindMatchesEventType(kind, eventType) {
			eventTypes = append(eventTypes, eventType)
		}
	}
	sort.Strings(eventTypes)
	return eventTypes
}

func replaySQLWorkerKind(kind string) string {
	if kind == outboxWorkerKindReservationCompensation {
		return outboxWorkerKindCompensation
	}
	return kind
}
