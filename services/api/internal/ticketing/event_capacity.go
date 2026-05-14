package ticketing

import "strings"

func intPtr(value int) *int {
	return &value
}

func capacityValue(capacity *int) int {
	if capacity == nil {
		return 0
	}
	return *capacity
}

func normalizeEventCapacity(capacityType string, capacity int, allowsFamily bool) (string, *int, bool, error) {
	normalizedType := strings.TrimSpace(capacityType)
	if normalizedType == "" {
		normalizedType = CapacityTypeLimited
	}
	var normalizedCapacity *int
	var normalizedAllowsFamily bool
	switch normalizedType {
	case CapacityTypeLimited:
		if allowsFamily {
			return "", nil, false, badRequest("limited events cannot allow family attendees")
		}
		normalizedCapacity = intPtr(capacity)
		normalizedAllowsFamily = false
	case CapacityTypeUnlimited:
		if capacity > 0 {
			return "", nil, false, badRequest("capacity must be null for unlimited events")
		}
		normalizedCapacity = nil
		normalizedAllowsFamily = true
	default:
		return "", nil, false, badRequest("capacity_type must be limited or unlimited")
	}
	event := Event{CapacityType: normalizedType, Capacity: normalizedCapacity, AllowsFamily: normalizedAllowsFamily}
	if err := validateEventCapacity(event); err != nil {
		return "", nil, false, err
	}
	return normalizedType, normalizedCapacity, normalizedAllowsFamily, nil
}

func validateEventCapacity(event Event) error {
	switch event.CapacityType {
	case CapacityTypeLimited:
		if event.Capacity == nil || *event.Capacity <= 0 {
			return badRequest("capacity must be positive for limited events")
		}
		if event.AllowsFamily {
			return badRequest("limited events cannot allow family attendees")
		}
	case CapacityTypeUnlimited:
		if event.Capacity != nil {
			return badRequest("capacity must be null for unlimited events")
		}
		if !event.AllowsFamily {
			return badRequest("unlimited events must allow family attendees")
		}
	default:
		return badRequest("capacity_type must be limited or unlimited")
	}
	return nil
}

func limitedCapacity(event Event) (int, error) {
	if event.CapacityType != CapacityTypeLimited || event.Capacity == nil {
		return 0, notImplemented("unlimited event booking allocation is not implemented")
	}
	return *event.Capacity, nil
}
