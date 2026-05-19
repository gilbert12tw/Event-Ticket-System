package ticketing

import (
	"fmt"
	"strings"
)

func deliveryMessageForOutbox(eventType string, employeeID string, payload map[string]interface{}) DeliveryMessage {
	body := fmt.Sprintf("Your corporate event ticketing status changed: %s.", eventType)
	if context := notificationActivityContext(payload); context != "" {
		body = fmt.Sprintf("%s %s", body, context)
	}
	if context := notificationCrossCityContext(payload); context != "" {
		body = fmt.Sprintf("%s %s", body, context)
	}
	return DeliveryMessage{
		To:      deliveryAddressForEmployee(employeeID),
		Subject: fmt.Sprintf("CETS update: %s", eventType),
		Body:    body,
	}
}

func deliveryAddressForEmployee(employeeID string) string {
	employeeID = strings.TrimSpace(strings.ToLower(employeeID))
	if employeeID == "" {
		return "unknown@cets.local"
	}
	return employeeID + "@cets.local"
}

func notificationActivityContext(payload map[string]interface{}) string {
	if len(payload) == 0 {
		return ""
	}
	parts := []string{}
	if title := stringFromPayload(payload, "event_title"); title != "" {
		parts = append(parts, "Activity: "+title+".")
	}
	if startsAt := stringFromPayload(payload, "starts_at"); startsAt != "" {
		parts = append(parts, "Starts at: "+startsAt+".")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func notificationCrossCityContext(payload map[string]interface{}) string {
	if stringFromPayload(payload, "warning_code") != string(WarningCrossCity) {
		return ""
	}
	eventCity := stringFromPayload(payload, "event_city")
	if eventCity == "" {
		return ""
	}
	return fmt.Sprintf("This activity is in %s; please plan travel accordingly.", eventCity)
}
