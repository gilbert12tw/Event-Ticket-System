package ticketing

import "strings"

type EventListQuery struct {
	CapacityType string
	City         string
	Status       string
}

func normalizeEventListQuery(query EventListQuery) (EventListQuery, error) {
	query.CapacityType = strings.TrimSpace(query.CapacityType)
	query.City = strings.TrimSpace(query.City)
	query.Status = strings.TrimSpace(query.Status)
	if query.Status == "" {
		query.Status = EventStatusPublished
	}
	if query.CapacityType != "" && query.CapacityType != CapacityTypeLimited && query.CapacityType != CapacityTypeUnlimited {
		return EventListQuery{}, badRequest("capacity_type must be limited or unlimited")
	}
	if !eventStatusAllowed(query.Status) {
		return EventListQuery{}, badRequest("status is invalid")
	}
	if query.Status != EventStatusPublished {
		return EventListQuery{}, badRequest("status must be published")
	}
	return query, nil
}

func eventStatusAllowed(status string) bool {
	switch status {
	case EventStatusDraft, EventStatusPublished, EventStatusClosed, EventStatusCancelled, EventStatusArchived:
		return true
	default:
		return false
	}
}
